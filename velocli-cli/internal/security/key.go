package security

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
)

func LoadOrCreateKey(filePath string) ([]byte, error) {
	if filePath == "" {
		return nil, errors.New("key file path required")
	}

	if b, err := os.ReadFile(filePath); err == nil {
		if len(b) != 32 {
			return nil, errors.New("invalid key file")
		}
		return b, nil
	}

	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}

	if err := os.WriteFile(filePath, key, 0600); err != nil {
		return nil, err
	}
	return key, nil
}
