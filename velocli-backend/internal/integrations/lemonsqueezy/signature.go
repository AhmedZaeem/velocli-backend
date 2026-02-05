package lemonsqueezy

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

func VerifySignature(rawBody []byte, secret []byte, signature string) bool {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(signature), "sha256=") {
		signature = strings.TrimSpace(signature[len("sha256="):])
	}

	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(rawBody)
	expected := []byte(hex.EncodeToString(mac.Sum(nil)))
	received := []byte(signature)

	if len(expected) != len(received) {
		return false
	}
	return subtle.ConstantTimeCompare(expected, received) == 1
}

