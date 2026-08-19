package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigLoadDefaults(t *testing.T) {
	// Safely clear relevant env vars with auto-restoration
	t.Setenv("GARGOYLE_LISTEN_ADDR", "")
	t.Setenv("GARGOYLE_RATE_LIMIT_WINDOW", "")
	t.Setenv("GARGOYLE_PRE_AUTH_RATE_LIMIT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.ListenAddr != ":8080" {
		t.Fatalf("expected ListenAddr :8080, got %q", cfg.ListenAddr)
	}
	if cfg.RateLimitWindow != time.Minute {
		t.Fatalf("expected RateLimitWindow 1m, got %v", cfg.RateLimitWindow)
	}
	if cfg.PreAuthRateLimit != 60 {
		t.Fatalf("expected PreAuthRateLimit 60, got %d", cfg.PreAuthRateLimit)
	}
	if cfg.AbuseBlockThreshold != 0.8 {
		t.Fatalf("expected AbuseBlockThreshold 0.8, got %v", cfg.AbuseBlockThreshold)
	}
	if cfg.AbuseSweepThreshold != 10 {
		t.Fatalf("expected AbuseSweepThreshold 10, got %d", cfg.AbuseSweepThreshold)
	}
	if cfg.AbuseSweepWindow != 10*time.Second {
		t.Fatalf("expected AbuseSweepWindow 10s, got %v", cfg.AbuseSweepWindow)
	}
}

func TestConfigLoadInvalidWindow(t *testing.T) {
	t.Setenv("GARGOYLE_RATE_LIMIT_WINDOW", "0s")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when GARGOYLE_RATE_LIMIT_WINDOW is 0s, got nil")
	}

	t.Setenv("GARGOYLE_RATE_LIMIT_WINDOW", "500us")
	_, err = Load()
	if err == nil {
		t.Fatal("expected error when GARGOYLE_RATE_LIMIT_WINDOW < 1ms, got nil")
	}
}

func TestConfigLoadInvalidPreAuthRateLimit(t *testing.T) {
	t.Setenv("GARGOYLE_PRE_AUTH_RATE_LIMIT", "-5")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when GARGOYLE_PRE_AUTH_RATE_LIMIT is negative, got nil")
	}

	for _, invalid := range []string{"60junk", "60.5", "abc"} {
		t.Setenv("GARGOYLE_PRE_AUTH_RATE_LIMIT", invalid)
		_, err = Load()
		if err == nil {
			t.Fatalf("expected error when GARGOYLE_PRE_AUTH_RATE_LIMIT is %q, got nil", invalid)
		}
	}
}

func TestConfigLoadAbuseSettings(t *testing.T) {
	t.Setenv("GARGOYLE_ABUSE_BLOCK_THRESHOLD", "1.5")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when GARGOYLE_ABUSE_BLOCK_THRESHOLD > 1.0, got nil")
	}

	t.Setenv("GARGOYLE_ABUSE_BLOCK_THRESHOLD", "-0.1")
	_, err = Load()
	if err == nil {
		t.Fatal("expected error when GARGOYLE_ABUSE_BLOCK_THRESHOLD < 0.0, got nil")
	}

	t.Setenv("GARGOYLE_ABUSE_BLOCK_THRESHOLD", "0.75")
	t.Setenv("GARGOYLE_ABUSE_SWEEP_THRESHOLD", "25")
	t.Setenv("GARGOYLE_ABUSE_SWEEP_WINDOW", "15s")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error loading valid abuse config: %v", err)
	}
	if cfg.AbuseBlockThreshold != 0.75 {
		t.Fatalf("expected AbuseBlockThreshold 0.75, got %v", cfg.AbuseBlockThreshold)
	}
	if cfg.AbuseSweepThreshold != 25 {
		t.Fatalf("expected AbuseSweepThreshold 25, got %d", cfg.AbuseSweepThreshold)
	}
	if cfg.AbuseSweepWindow != 15*time.Second {
		t.Fatalf("expected AbuseSweepWindow 15s, got %v", cfg.AbuseSweepWindow)
	}
}

func TestParseEnvFile(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")

	content := `
# Comment line
TEST_GARGOYLE_VAR1=hello
TEST_GARGOYLE_VAR2="quoted_val"
TEST_GARGOYLE_VAR3='single_quoted'
INVALID_LINE_NO_EQUALS
`
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test .env: %v", err)
	}

	if err := parseEnvFile(envPath); err != nil {
		t.Fatalf("parseEnvFile failed: %v", err)
	}

	if val := os.Getenv("TEST_GARGOYLE_VAR1"); val != "hello" {
		t.Fatalf("expected hello, got %q", val)
	}
	if val := os.Getenv("TEST_GARGOYLE_VAR2"); val != "quoted_val" {
		t.Fatalf("expected quoted_val, got %q", val)
	}
	if val := os.Getenv("TEST_GARGOYLE_VAR3"); val != "single_quoted" {
		t.Fatalf("expected single_quoted, got %q", val)
	}

	// Non-existent file should return error
	if err := parseEnvFile(filepath.Join(tempDir, "nonexistent")); err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}
