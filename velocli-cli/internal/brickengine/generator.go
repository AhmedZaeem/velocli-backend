package brickengine

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/afero"
	"github.com/velocli/velocli/velocli-shared/secure"
)

type Generator struct {
	fs        afero.Fs
	masterKey []byte
}

func NewGenerator(fs afero.Fs, masterKey []byte) (*Generator, error) {
	if fs == nil {
		return nil, errors.New("fs is required")
	}
	if len(masterKey) != 32 {
		return nil, errors.New("invalid master key length")
	}
	keyCopy := make([]byte, len(masterKey))
	copy(keyCopy, masterKey)
	return &Generator{fs: fs, masterKey: keyCopy}, nil
}

func (g *Generator) Generate(ctx context.Context, encryptedZip []byte, destDir string, vars any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	destDir = filepath.Clean(destDir)
	if destDir == "." || destDir == "" {
		return errors.New("destDir is required")
	}

	zipBytes, err := secure.Decrypt(g.masterKey, encryptedZip)
	if err != nil {
		return err
	}

	if err := unzipToFs(ctx, g.fs, destDir, zipBytes); err != nil {
		return err
	}

	return applyTemplates(ctx, g.fs, destDir, vars)
}

func unzipToFs(ctx context.Context, fs afero.Fs, destDir string, zipBytes []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return err
	}

	for _, f := range zr.File {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		rel, err := sanitizeZipPath(f.Name)
		if err != nil {
			return err
		}
		if rel == "" {
			continue
		}

		target := filepath.Join(destDir, filepath.FromSlash(rel))
		if f.FileInfo().IsDir() {
			if err := fs.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}

		if err := fs.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := fs.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			_ = rc.Close()
			return err
		}

		_, copyErr := io.Copy(out, rc)
		_ = out.Close()
		_ = rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}

	return nil
}

func sanitizeZipPath(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}

	cleaned := path.Clean("/" + name)
	if strings.HasPrefix(cleaned, "/..") {
		return "", errors.New("invalid zip path")
	}
	rel := strings.TrimPrefix(cleaned, "/")
	if rel == "." {
		return "", nil
	}
	if strings.Contains(rel, ":") {
		return "", errors.New("invalid zip path")
	}
	return rel, nil
}

func applyTemplates(ctx context.Context, fs afero.Fs, root string, vars any) error {
	return afero.Walk(fs, root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(p), ".dart") {
			return nil
		}

		b, err := afero.ReadFile(fs, p)
		if err != nil {
			return err
		}

		tmpl, err := template.New(filepath.Base(p)).Option("missingkey=error").Parse(string(b))
		if err != nil {
			return err
		}
		var out bytes.Buffer
		if err := tmpl.Execute(&out, vars); err != nil {
			return err
		}

		return afero.WriteFile(fs, p, out.Bytes(), info.Mode())
	})
}
