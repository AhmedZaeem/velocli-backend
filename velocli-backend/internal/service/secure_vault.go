package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/velocli/velocli/velocli-backend/internal/domain"
	"github.com/velocli/velocli/velocli-backend/internal/repository"
	"github.com/velocli/velocli/velocli-shared/secure"
)

type SecureVault struct {
	masterKey []byte
	bricks    *repository.BricksRepository
}

func NewSecureVault(masterKey []byte, bricks *repository.BricksRepository) (*SecureVault, error) {
	if len(masterKey) != 32 {
		return nil, errors.New("invalid master key length")
	}
	if bricks == nil {
		return nil, errors.New("bricks repo required")
	}
	keyCopy := make([]byte, len(masterKey))
	copy(keyCopy, masterKey)
	return &SecureVault{masterKey: keyCopy, bricks: bricks}, nil
}

func (v *SecureVault) StoreFolder(ctx context.Context, name string, version string, folderPath string, variables []string) (string, error) {
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	folderPath = strings.TrimSpace(folderPath)
	if name == "" || version == "" || folderPath == "" {
		return "", errors.New("name, version, and folderPath are required")
	}

	payload, err := zipFolder(folderPath)
	if err != nil {
		return "", err
	}

	encrypted, err := secure.Encrypt(v.masterKey, payload)
	if err != nil {
		return "", err
	}

	sort.Strings(variables)
	variables = uniqueStrings(variables)

	id, err := v.bricks.Insert(ctx, domain.Brick{
		Name:             name,
		Version:          version,
		EncryptedPayload: encrypted,
		Variables:        variables,
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

func zipFolder(root string) ([]byte, error) {
	root = filepath.Clean(root)

	var files []string
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		files = append(files, rel)
		return nil
	}); err != nil {
		return nil, err
	}

	sort.Strings(files)

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for _, rel := range files {
		abs := filepath.Join(root, rel)
		info, err := os.Stat(abs)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		h.Name = filepath.ToSlash(rel)
		h.Method = zip.Deflate

		w, err := zw.CreateHeader(h)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}

		f, err := os.Open(abs)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		_, copyErr := io.Copy(w, f)
		_ = f.Close()
		if copyErr != nil {
			_ = zw.Close()
			return nil, copyErr
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	var prev string
	for i, s := range in {
		if i == 0 || s != prev {
			out = append(out, s)
		}
		prev = s
	}
	return out
}
