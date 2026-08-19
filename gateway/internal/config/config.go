// Package config loads and validates Gargoyle's runtime configuration from
// environment variables. It intentionally has zero dependencies outside the
// standard library so that it can be imported by every other package
// without pulling in anything heavy.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config holds all settings needed to run the Gargoyle gateway.
type Config struct {
	// ListenAddr is the address the HTTP server binds to, e.g. ":8080".
	ListenAddr string

	// DatabaseURL is the Postgres connection string used for the client
	// registry and per-client request logs.
	DatabaseURL string

	// RedisURL is the connection string for Redis used by the rate limiter
	// and abuse detection.
	RedisURL string

	// RateLimitWindow is the sliding window duration over which client rate
	// limits are enforced (defaults to 1 minute, must be >= 1ms).
	RateLimitWindow time.Duration

	// PreAuthRateLimit bounds the number of requests per IP within the
	// sliding window allowed before authentication (protection against
	// unauthenticated key-stuffing/DoS attacks, defaults to 60 req/window). Set to 0 to disable.
	PreAuthRateLimit int

	// AbuseBlockThreshold is the minimum risk score (0.0 to 1.0) required to
	// block a request with 403 Forbidden (defaults to 0.8).
	AbuseBlockThreshold float64

	// AbuseSweepThreshold is the maximum number of distinct paths allowed from
	// the same client/IP within AbuseSweepWindow before triggering sweep detection (defaults to 10).
	AbuseSweepThreshold int

	// AbuseSweepWindow is the sliding window for tracking distinct endpoints (defaults to 10s).
	AbuseSweepWindow time.Duration

	// ClientCacheTTL bounds how long a resolved client (API key -> target
	// URL, rate limit, plan tier) is cached in memory before the next
	// lookup re-reads it from Postgres.
	ClientCacheTTL time.Duration

	// ReadHeaderTimeout bounds how long the server waits to read request
	// headers, mitigating slow-header (Slowloris-style) attacks.
	ReadHeaderTimeout time.Duration
	// ReadTimeout bounds how long the server waits to read the full request.
	ReadTimeout time.Duration
	// WriteTimeout bounds how long the server allows for writing a response.
	WriteTimeout time.Duration
	// IdleTimeout bounds how long an idle keep-alive connection is kept open.
	IdleTimeout time.Duration
	// ShutdownTimeout bounds how long graceful shutdown waits for in-flight
	// requests to finish before forcing an exit.
	ShutdownTimeout time.Duration
}

var loadOnce sync.Once

// Load reads configuration from environment variables and local .env files,
// applying defaults for anything not set.
func Load() (*Config, error) {
	loadOnce.Do(loadDotEnv)
	clientCacheTTL, err := getDuration("GARGOYLE_CLIENT_CACHE_TTL", 30*time.Second)
	if err != nil {
		return nil, err
	}

	rateLimitWindow, err := getDuration("GARGOYLE_RATE_LIMIT_WINDOW", 1*time.Minute)
	if err != nil {
		return nil, err
	}
	if rateLimitWindow < time.Millisecond {
		return nil, fmt.Errorf("config: GARGOYLE_RATE_LIMIT_WINDOW must be at least 1ms, got %v", rateLimitWindow)
	}

	preAuthRateLimit, err := getInt("GARGOYLE_PRE_AUTH_RATE_LIMIT", 60)
	if err != nil {
		return nil, err
	}
	if preAuthRateLimit < 0 {
		return nil, fmt.Errorf("config: GARGOYLE_PRE_AUTH_RATE_LIMIT cannot be negative, got %d", preAuthRateLimit)
	}

	abuseBlockThreshold, err := getFloat("GARGOYLE_ABUSE_BLOCK_THRESHOLD", 0.8)
	if err != nil {
		return nil, err
	}
	if abuseBlockThreshold < 0.0 || abuseBlockThreshold > 1.0 {
		return nil, fmt.Errorf("config: GARGOYLE_ABUSE_BLOCK_THRESHOLD must be between 0.0 and 1.0, got %v", abuseBlockThreshold)
	}

	abuseSweepThreshold, err := getInt("GARGOYLE_ABUSE_SWEEP_THRESHOLD", 10)
	if err != nil {
		return nil, err
	}
	if abuseSweepThreshold < 0 {
		return nil, fmt.Errorf("config: GARGOYLE_ABUSE_SWEEP_THRESHOLD cannot be negative, got %d", abuseSweepThreshold)
	}

	abuseSweepWindow, err := getDuration("GARGOYLE_ABUSE_SWEEP_WINDOW", 10*time.Second)
	if err != nil {
		return nil, err
	}
	if abuseSweepWindow < time.Millisecond {
		return nil, fmt.Errorf("config: GARGOYLE_ABUSE_SWEEP_WINDOW must be at least 1ms, got %v", abuseSweepWindow)
	}

	readHeaderTimeout, err := getDuration("GARGOYLE_READ_HEADER_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, err
	}
	readTimeout, err := getDuration("GARGOYLE_READ_TIMEOUT", 15*time.Second)
	if err != nil {
		return nil, err
	}
	writeTimeout, err := getDuration("GARGOYLE_WRITE_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, err
	}
	idleTimeout, err := getDuration("GARGOYLE_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return nil, err
	}
	shutdownTimeout, err := getDuration("GARGOYLE_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		ListenAddr:          getEnv("GARGOYLE_LISTEN_ADDR", ":8080"),
		DatabaseURL:         getEnv("GARGOYLE_DATABASE_URL", "postgres://gargoyle:gargoyle@localhost:5432/gargoyle?sslmode=disable"),
		RedisURL:            getEnv("GARGOYLE_REDIS_URL", "redis://localhost:6379/0"),
		RateLimitWindow:     rateLimitWindow,
		PreAuthRateLimit:    preAuthRateLimit,
		AbuseBlockThreshold: abuseBlockThreshold,
		AbuseSweepThreshold: abuseSweepThreshold,
		AbuseSweepWindow:    abuseSweepWindow,
		ClientCacheTTL:      clientCacheTTL,
		ReadHeaderTimeout:   readHeaderTimeout,
		ReadTimeout:         readTimeout,
		WriteTimeout:        writeTimeout,
		IdleTimeout:         idleTimeout,
		ShutdownTimeout:     shutdownTimeout,
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("config: parsing %s %q as integer: %w", key, v, err)
	}
	return n, nil
}

func getFloat(key string, fallback float64) (float64, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("config: parsing %s %q as float: %w", key, v, err)
	}
	return f, nil
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("config: parsing %s %q: %w", key, v, err)
	}
	return d, nil
}

func loadDotEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}

	for i := 0; i < 5; i++ {
		target := filepath.Join(dir, ".env")
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			_ = parseEnvFile(target)
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
}

func parseEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, val, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}

		if _, exists := os.LookupEnv(key); !exists && val != "" {
			_ = os.Setenv(key, val)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("config: reading %s: %w", path, err)
	}
	return nil
}

