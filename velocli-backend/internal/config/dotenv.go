package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func loadDotEnvFor(appEnv string) error {
	appEnv = strings.ToLower(strings.TrimSpace(appEnv))
	if appEnv == "" {
		appEnv = "dev"
	}
	filename := ".env." + appEnv

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	candidates := []string{
		filepath.Join(cwd, filename),
	}
	if filepath.Base(cwd) != "velocli-backend" {
		candidates = append(candidates, filepath.Join(cwd, "velocli-backend", filename))
	}

	for _, p := range candidates {
		if err := loadDotEnvFile(p); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		return nil
	}
	return nil
}

func loadDotEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "export ") {
			line = strings.TrimSpace(line[len("export "):])
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" {
			continue
		}
		val = stripOptionalQuotes(val)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, val)
	}
	return sc.Err()
}

func stripOptionalQuotes(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

