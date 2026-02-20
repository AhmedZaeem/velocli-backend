package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
	"os"

	"golang.org/x/crypto/chacha20poly1305"
)

const KeySize = 32
const NonceSize = 12

var headerV2 = []byte("VELOENC2")

func LoadKeyFromEnvOrFile(envKey string, filePath string) ([]byte, error) {
	if v := os.Getenv(envKey); v != "" {
		b, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return nil, err
		}
		if len(b) != KeySize {
			return nil, errors.New("invalid key length")
		}
		return b, nil
	}

	if b, err := os.ReadFile(filePath); err == nil {
		key, err := base64.StdEncoding.DecodeString(string(bytesTrimSpace(b)))
		if err != nil {
			return nil, err
		}
		if len(key) != KeySize {
			return nil, errors.New("invalid key length")
		}
		return key, nil
	}

	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}

	enc := base64.StdEncoding.EncodeToString(key)
	if err := os.MkdirAll(dir(filePath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filePath, []byte(enc), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func Encrypt(key []byte, plaintext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := aead.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(headerV2)+len(nonce)+len(ciphertext))
	out = append(out, headerV2...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

func Decrypt(key []byte, data []byte) ([]byte, error) {
	if len(data) >= len(headerV2)+NonceSize && subtle.ConstantTimeCompare(data[:len(headerV2)], headerV2) == 1 {
		aead, err := chacha20poly1305.New(key)
		if err != nil {
			return nil, err
		}
		nonce := data[len(headerV2) : len(headerV2)+NonceSize]
		ciphertext := data[len(headerV2)+NonceSize:]
		return aead.Open(nil, nonce, ciphertext, nil)
	}

	if len(data) < NonceSize {
		return nil, errors.New("ciphertext too short")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := data[:NonceSize]
	ciphertext := data[NonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func bytesTrimSpace(b []byte) []byte {
	start := 0
	for start < len(b) && (b[start] == ' ' || b[start] == '\n' || b[start] == '\r' || b[start] == '\t') {
		start++
	}
	end := len(b)
	for end > start && (b[end-1] == ' ' || b[end-1] == '\n' || b[end-1] == '\r' || b[end-1] == '\t') {
		end--
	}
	return b[start:end]
}

func dir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			if i == 0 {
				return "/"
			}
			return p[:i]
		}
	}
	return "."
}
