package client

import "testing"

func TestGenerateAPIKey(t *testing.T) {
	a, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	b, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	if a == b {
		t.Fatalf("expected two distinct keys, got the same value twice: %q", a)
	}
	for _, k := range []string{a, b} {
		if len(k) <= len(apiKeyPrefix) {
			t.Fatalf("key %q is too short to contain real entropy", k)
		}
		if k[:len(apiKeyPrefix)] != apiKeyPrefix {
			t.Fatalf("key %q does not start with prefix %q", k, apiKeyPrefix)
		}
	}
}

func TestHashAPIKeyIsDeterministicAndDistinct(t *testing.T) {
	const keyA = "gk_live_same-key-value"
	const keyB = "gk_live_different-key-value"

	hash1 := HashAPIKey(keyA)
	hash2 := HashAPIKey(keyA)
	if hash1 != hash2 {
		t.Fatal("expected HashAPIKey to be deterministic for the same input")
	}
	if hash1 == HashAPIKey(keyB) {
		t.Fatal("expected different keys to hash to different values")
	}
	if hash1 == keyA {
		t.Fatal("expected the hash to differ from the plaintext key")
	}
}
