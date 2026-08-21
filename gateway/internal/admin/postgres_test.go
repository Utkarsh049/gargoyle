package admin

import (
	"testing"

	"gargoyle/internal/client"
)

func TestClientAPIKeyGeneration(t *testing.T) {
	rawKey, err := client.GenerateAPIKey()
	if err != nil {
		t.Fatalf("failed to generate API key: %v", err)
	}

	if len(rawKey) < 32 || rawKey[:8] != "gk_live_" {
		t.Fatalf("unexpected API key format: %s", rawKey)
	}

	keyHash := client.HashAPIKey(rawKey)
	if len(keyHash) != 64 {
		t.Fatalf("unexpected key hash length: %d", len(keyHash))
	}
}
