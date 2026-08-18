// Package config loads and validates Gargoyle's runtime configuration from
// environment variables. It intentionally has zero dependencies outside the
// standard library so that it can be imported by every other package
// without pulling in anything heavy.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all settings needed to run the Gargoyle gateway.
//
// Later phases (Redis, abuse detection thresholds, etc.) will add fields
// here rather than introducing a second, competing config source.
type Config struct {
	// ListenAddr is the address the HTTP server binds to, e.g. ":8080".
	ListenAddr string

	// DatabaseURL is the Postgres connection string used for the client
	// registry (Phase 2) and, from Phase 5 onward, per-client request logs.
	DatabaseURL string

	// RedisURL is the connection string for Redis used by the rate limiter
	// (Phase 3) and abuse detection (Phase 6).
	RedisURL string

	// RateLimitWindow is the sliding window duration over which client rate
	// limits are enforced (defaults to 1 minute, must be >= 1ms).
	RateLimitWindow time.Duration

	// PreAuthRateLimit bounds the number of requests per IP within the
	// sliding window allowed before authentication (Phase 3 protection against
	// unauthenticated key-stuffing/DoS attacks, defaults to 60 req/window). Set to 0 to disable.
	PreAuthRateLimit int

	// ClientCacheTTL bounds how long a resolved client (API key -> target
	// URL, rate limit, plan tier) is cached in memory before the next
	// lookup re-reads it from Postgres. See PROJECT.md §8.
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

// Load reads configuration from environment variables, applying sane
// defaults for anything not set, and returns an error if the resulting
// configuration is invalid.
func Load() (*Config, error) {
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
		ListenAddr:        getEnv("GARGOYLE_LISTEN_ADDR", ":8080"),
		DatabaseURL:       getEnv("GARGOYLE_DATABASE_URL", "postgres://gargoyle:gargoyle@localhost:5432/gargoyle?sslmode=disable"),
		RedisURL:          getEnv("GARGOYLE_REDIS_URL", "redis://localhost:6379/0"),
		RateLimitWindow:   rateLimitWindow,
		PreAuthRateLimit:  preAuthRateLimit,
		ClientCacheTTL:    clientCacheTTL,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ShutdownTimeout:   shutdownTimeout,
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
