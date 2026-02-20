package commands

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/velocli/velocli/velocli-cli/internal/cryptox"
	"gopkg.in/yaml.v3"
)

func applySelectedBlocks(apiURL string, blocks []Block, selectedIDs []string, projectDir string) error {
	if len(selectedIDs) == 0 {
		return nil
	}

	key, err := cryptox.LoadKeyFromEnv("VELOCLI_DATA_KEY")
	if err != nil {
		return err
	}

	cacheDir, err := userCacheDir()
	if err != nil {
		return err
	}
	cacheDir = filepath.Join(cacheDir, "velocli", "blocks")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}

	_ = purgeOldCache(cacheDir, 7*24*time.Hour)

	blockByID := map[string]Block{}
	for _, b := range blocks {
		blockByID[b.ID] = b
	}

	for _, id := range selectedIDs {
		blk, ok := blockByID[id]
		if !ok {
			return fmt.Errorf("unknown block: %s", id)
		}

		enc, err := cacheGetOrFetchEncrypted(apiURL, cacheDir, blk.ID)
		if err != nil {
			return err
		}

		plainZip, err := cryptox.Decrypt(key, enc)
		if err != nil {
			return err
		}

		if err := applyZipBlock(projectDir, blk, plainZip); err != nil {
			return err
		}

		if err := updatePubspecDeps(projectDir, blk.Deps); err != nil {
			return err
		}

		if err := applyMainMutation(projectDir, blk); err != nil {
			return err
		}
	}

	return nil
}

func applyMainTemplate(apiURL string, t MainTemplate, projectDir string) error {
	if strings.TrimSpace(t.BlobID) == "" && strings.TrimSpace(t.Content) == "" {
		return nil
	}

	key, err := cryptox.LoadKeyFromEnv("VELOCLI_DATA_KEY")
	if err != nil {
		return err
	}

	cacheDir, err := userCacheDir()
	if err != nil {
		return err
	}
	cacheDir = filepath.Join(cacheDir, "velocli", "templates")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}

	_ = purgeOldCache(cacheDir, 7*24*time.Hour)

	if strings.TrimSpace(t.BlobID) != "" {
		enc, err := cacheGetOrFetchEncryptedTemplate(apiURL, cacheDir, t.ID)
		if err != nil {
			return err
		}
		plainZip, err := cryptox.Decrypt(key, enc)
		if err != nil {
			return err
		}
		if err := applyZipBlock(projectDir, Block{BasePath: "lib/"}, plainZip); err != nil {
			return err
		}
	}

	if strings.TrimSpace(t.Content) != "" {
		target := filepath.Join(projectDir, "lib", "main.dart")
		if err := os.WriteFile(target, []byte(t.Content), 0o644); err != nil {
			return err
		}
	}

	if len(t.Deps) > 0 {
		if err := updatePubspecDeps(projectDir, t.Deps); err != nil {
			return err
		}
	}

	return nil
}

func applyMainMutation(projectDir string, blk Block) error {
	mode := strings.ToLower(strings.TrimSpace(blk.MainMode))
	if mode == "" || mode == "none" {
		return nil
	}
	target := strings.TrimSpace(blk.MainTarget)
	if target == "" {
		target = "lib/main.dart"
	}
	target = filepath.Join(projectDir, filepath.FromSlash(strings.TrimPrefix(target, "/")))

	// If target is a directory, append main.dart
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		target = filepath.Join(target, "main.dart")
	}

	switch mode {
	case "replace":
		if blk.MainContent == "" {
			return nil
		}
		return os.WriteFile(target, []byte(blk.MainContent), 0o644)
	case "prepend":
		if blk.MainContent == "" {
			return nil
		}
		existing, _ := os.ReadFile(target)
		head := blk.MainContent
		if head != "" && !strings.HasSuffix(head, "\n") {
			head += "\n"
		}
		out := head + string(existing)
		return os.WriteFile(target, []byte(out), 0o644)
	case "append":
		if blk.MainContent == "" {
			return nil
		}
		existing, _ := os.ReadFile(target)
		out := string(existing)
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += blk.MainContent
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		return os.WriteFile(target, []byte(out), 0o644)
	case "inject":
		if strings.TrimSpace(blk.MainContent) == "" {
			return nil
		}
		existing, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		out, changed, err := injectIntoDartMain(string(existing), blk.MainContent)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		return os.WriteFile(target, []byte(out), 0o644)
	default:
		return nil
	}
}

var reDartMain = regexp.MustCompile(`(?m)^\s*(?:Future<\s*void\s*>\s+|void\s+)?main\s*\([^)]*\)\s*(?:async\s*)?\{`)

func injectIntoDartMain(src string, snippet string) (string, bool, error) {
	snippet = strings.TrimSpace(snippet)
	if snippet == "" {
		return src, false, nil
	}
	if strings.Contains(src, snippet) {
		return src, false, nil
	}

	loc := reDartMain.FindStringIndex(src)
	if loc == nil {
		return src, false, fmt.Errorf("main() not found for injection")
	}
	openBrace := strings.Index(src[loc[0]:loc[1]], "{")
	if openBrace < 0 {
		return src, false, fmt.Errorf("main() body not found for injection")
	}
	openIdx := loc[0] + openBrace

	closeIdx, ok := findMatchingBrace(src, openIdx)
	if !ok {
		return src, false, fmt.Errorf("main() body not balanced for injection")
	}

	body := src[openIdx+1 : closeIdx]
	if strings.Contains(body, snippet) {
		return src, false, nil
	}

	insertAt := -1
	runAppPos := strings.Index(body, "runApp(")
	if runAppPos >= 0 {
		insertAt = runAppPos
	} else {
		insertAt = len(body)
	}

	indent := detectIndentForInsert(body, insertAt)
	ins := indentMultiline(snippet, indent)
	if ins != "" && !strings.HasSuffix(ins, "\n") {
		ins += "\n"
	}

	if insertAt == len(body) {
		if strings.TrimSpace(body) != "" && !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		body = body + ins
	} else {
		if insertAt > 0 && !strings.HasSuffix(body[:insertAt], "\n") {
			ins = "\n" + ins
		}
		body = body[:insertAt] + ins + body[insertAt:]
	}

	out := src[:openIdx+1] + body + src[closeIdx:]
	return out, true, nil
}

func findMatchingBrace(src string, openIdx int) (int, bool) {
	if openIdx < 0 || openIdx >= len(src) || src[openIdx] != '{' {
		return -1, false
	}
	depth := 0
	for i := openIdx; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return -1, false
}

func detectIndentForInsert(body string, at int) string {
	if at < 0 {
		at = 0
	}
	if at > len(body) {
		at = len(body)
	}
	lineStart := strings.LastIndex(body[:at], "\n")
	if lineStart < 0 {
		lineStart = 0
	} else {
		lineStart++
	}
	i := lineStart
	for i < len(body) {
		ch := body[i]
		if ch != ' ' && ch != '\t' {
			break
		}
		i++
	}
	indent := body[lineStart:i]
	if indent == "" {
		return "  "
	}
	return indent
}

func indentMultiline(s string, indent string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = indent + strings.TrimRight(lines[i], " \t")
	}
	return strings.Join(lines, "\n")
}

func applyZipBlock(projectDir string, blk Block, zipBytes []byte) error {
	if len(zipBytes) == 0 {
		return nil
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return err
	}

	base := strings.TrimSpace(blk.BasePath)
	if base == "" {
		base = "/"
	}
	base = strings.TrimPrefix(base, "./")

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}

		zipPath := sanitizeZipPath(f.Name)
		if strings.HasPrefix(zipPath, "__MACOSX/") || zipPath == "__MACOSX" {
			continue
		}
		if zipPath == "" {
			continue
		}

		rel := zipPath
		if strings.HasPrefix(base, "/") {
			rel = path.Clean(path.Join(strings.TrimPrefix(base, "/"), zipPath))
		} else {
			rel = path.Clean(path.Join(base, zipPath))
		}

		dstPath := filepath.Join(projectDir, filepath.FromSlash(rel))
		if !strings.HasPrefix(dstPath, filepath.Clean(projectDir)+string(os.PathSeparator)) {
			continue
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return err
		}

		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			return err
		}
	}

	return nil
}

func sanitizeZipPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.TrimPrefix(p, "/")
	p = path.Clean(p)
	if p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return ""
	}
	return p
}

func updatePubspecDeps(projectDir string, deps map[string]string) error {
	if len(deps) == 0 {
		return nil
	}

	pubspecPath := filepath.Join(projectDir, "pubspec.yaml")
	b, err := os.ReadFile(pubspecPath)
	if err != nil {
		return err
	}

	var root map[string]any
	if err := yaml.Unmarshal(b, &root); err != nil {
		return err
	}

	rawDeps, ok := root["dependencies"]
	if !ok {
		rawDeps = map[string]any{}
		root["dependencies"] = rawDeps
	}

	depMap, ok := rawDeps.(map[string]any)
	if !ok {
		return fmt.Errorf("pubspec dependencies is not a map")
	}

	for k, v := range deps {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		depMap[k] = v
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(pubspecPath, out, 0o644)
}

func cacheGetOrFetchEncrypted(apiURL string, cacheDir string, blockID string) ([]byte, error) {
	cachePath := filepath.Join(cacheDir, blockID+".bin")
	if b, err := os.ReadFile(cachePath); err == nil {
		_ = os.Chtimes(cachePath, time.Now(), time.Now())
		return b, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := strings.TrimRight(apiURL, "/") + "/api/v1/blocks/" + blockID + "/download"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-VeloCLI-Version", Version)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
		return nil, fmt.Errorf("download failed: %s (%s)", res.Status, strings.TrimSpace(string(body)))
	}

	b, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, err
	}

	tmp := cachePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, cachePath); err != nil {
		return nil, err
	}
	return b, nil
}

func cacheGetOrFetchEncryptedTemplate(apiURL string, cacheDir string, templateID string) ([]byte, error) {
	cachePath := filepath.Join(cacheDir, templateID+".bin")
	if b, err := os.ReadFile(cachePath); err == nil {
		_ = os.Chtimes(cachePath, time.Now(), time.Now())
		return b, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := strings.TrimRight(apiURL, "/") + "/api/v1/templates/" + templateID + "/download"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-VeloCLI-Version", Version)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
		return nil, fmt.Errorf("download failed: %s (%s)", res.Status, strings.TrimSpace(string(body)))
	}

	b, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, err
	}

	tmp := cachePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, cachePath); err != nil {
		return nil, err
	}
	return b, nil
}

func purgeOldCache(cacheDir string, ttl time.Duration) error {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-ttl)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(cacheDir, e.Name()))
		}
	}
	return nil
}

func userCacheDir() (string, error) {
	if d, err := os.UserCacheDir(); err == nil && d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache"), nil
}
