package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"os"

	"golang.org/x/crypto/chacha20poly1305"
)

const KeySize = 32
const NonceSize = 12

var headerV2 = []byte("VELOENC2")

func LoadKeyFromEnv(envKey string) ([]byte, error) {
	v := os.Getenv(envKey)
	if v == "" {
		return nil, errors.New("missing encryption key")
	}
	b, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, err
	}
	if len(b) != KeySize {
		return nil, errors.New("invalid key length")
	}
	return b, nil
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
