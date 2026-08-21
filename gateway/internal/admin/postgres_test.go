package admin

import (
	"testing"
)

func TestGenerateAPIKey(t *testing.T) {
	rawKey, keyHash := generateAPIKey()

	if len(rawKey) != 40 || rawKey[:8] != "gk_live_" {
		t.Fatalf("unexpected API key format: %s", rawKey)
	}

	if len(keyHash) != 64 {
		t.Fatalf("unexpected key hash length: %d", len(keyHash))
	}
}
