package brickengine

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"github.com/spf13/afero"
	"github.com/velocli/velocli/velocli-shared/secure"
)

func TestGenerator_DecryptUnzipAndTemplate(t *testing.T) {
	masterKey := bytes.Repeat([]byte{0x11}, 32)

	zipBytes := buildZip(t, map[string]string{
		"lib/main.dart": `void main() { print("{{.AppName}} @ {{.Organization}}"); }`,
		"README.md":     `# {{.AppName}}`,
	})

	encrypted, err := secure.Encrypt(masterKey, zipBytes)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	fs := afero.NewMemMapFs()
	gen, err := NewGenerator(fs, masterKey)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}

	vars := map[string]any{
		"AppName":       "HelloWorld",
		"Organization":  "Acme",
	}

	if err := gen.Generate(context.Background(), encrypted, "project", vars); err != nil {
		t.Fatalf("generate: %v", err)
	}

	got, err := afero.ReadFile(fs, "project/lib/main.dart")
	if err != nil {
		t.Fatalf("read generated dart: %v", err)
	}
	want := `void main() { print("HelloWorld @ Acme"); }`
	if string(bytes.TrimSpace(got)) != want {
		t.Fatalf("unexpected dart content:\nwant: %q\ngot:  %q", want, string(bytes.TrimSpace(got)))
	}

	readme, err := afero.ReadFile(fs, "project/README.md")
	if err != nil {
		t.Fatalf("read readme: %v", err)
	}
	if string(bytes.TrimSpace(readme)) != `# {{.AppName}}` {
		t.Fatalf("non-dart files should not be templated, got: %q", string(readme))
	}
}

func TestGenerator_MissingTemplateKeyReturnsError(t *testing.T) {
	masterKey := bytes.Repeat([]byte{0x22}, 32)

	zipBytes := buildZip(t, map[string]string{
		"lib/main.dart": `{{.Missing}}`,
	})

	encrypted, err := secure.Encrypt(masterKey, zipBytes)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	fs := afero.NewMemMapFs()
	gen, err := NewGenerator(fs, masterKey)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}

	if err := gen.Generate(context.Background(), encrypted, "project", map[string]any{"AppName": "X"}); err == nil {
		t.Fatalf("expected error for missing template key")
	}
}

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %q: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

