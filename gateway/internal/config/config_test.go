package config

import (
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
