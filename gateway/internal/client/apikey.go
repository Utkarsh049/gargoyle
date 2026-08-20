package client

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const apiKeyPrefix = "gk_live_"

const apiKeyRandomBytes = 24

// GenerateAPIKey creates a cryptographically random API key.
func GenerateAPIKey() (string, error) {
	buf := make([]byte, apiKeyRandomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("client: generating api key: %w", err)
	}
	return apiKeyPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashAPIKey returns a SHA-256 digest of a plaintext API key for storage and lookup.
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
