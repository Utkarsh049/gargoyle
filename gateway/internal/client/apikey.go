package client

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// apiKeyPrefix marks a string as a Gargoyle API key at a glance (mirrors
// the convention used by Stripe, GitHub, etc.), which makes keys easy to
// recognize in logs, config files, and support tickets.
const apiKeyPrefix = "gk_live_"

// apiKeyRandomBytes is the amount of randomness backing each key: 24 bytes
// is 192 bits of entropy, comfortably beyond what's brute-forceable.
const apiKeyRandomBytes = 24

// GenerateAPIKey creates a new cryptographically random API key. The
// plaintext value is meant to be shown to the client exactly once — only
// its hash (see HashAPIKey) is ever persisted.
func GenerateAPIKey() (string, error) {
	buf := make([]byte, apiKeyRandomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("client: generating api key: %w", err)
	}
	return apiKeyPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashAPIKey returns a deterministic digest of a plaintext API key,
// suitable for storage and lookup.
//
// A plain SHA-256 digest (no salt, no slow KDF like bcrypt) is deliberate
// here, not an oversight: unlike a password, an API key is a high-entropy
// random token the client never has to remember, so there's no weak input
// space for an attacker to dictionary-attack — the only feasible attack is
// brute-forcing the hash itself, which 192 bits of entropy already makes
// infeasible. A fast, deterministic hash also keeps the lookup path cheap.
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
