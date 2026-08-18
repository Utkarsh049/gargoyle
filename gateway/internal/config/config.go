// Package config loads and validates Gargoyle's runtime configuration from
// environment variables. It intentionally has zero dependencies outside the
// standard library so that it can be imported by every other package
// without pulling in anything heavy.
package config

import (
	"fmt"
	"net/url"
	"os"
	"time"
)

// Config holds all settings needed to run the Gargoyle gateway.
//
// Phase 1 only needs a listen address and a single hardcoded upstream
// target. Later phases (client registry, Redis, Postgres, etc.) will add
// fields here rather than introducing a second, competing config source.
type Config struct {
	// ListenAddr is the address the HTTP server binds to, e.g. ":8080".
	ListenAddr string

	// TargetURL is the upstream backend all traffic is forwarded to.
	// This is a temporary, hardcoded stand-in for the per-client
	// target_url lookup that arrives in Phase 2.
	TargetURL *url.URL

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
// configuration is invalid (e.g. an unparsable target URL).
func Load() (*Config, error) {
	targetURLStr := getEnv("GARGOYLE_TARGET_URL", "http://localhost:9000")
	targetURL, err := url.Parse(targetURLStr)
	if err != nil {
		return nil, fmt.Errorf("config: parsing GARGOYLE_TARGET_URL %q: %w", targetURLStr, err)
	}
	if targetURL.Scheme == "" || targetURL.Host == "" {
		return nil, fmt.Errorf("config: GARGOYLE_TARGET_URL %q must be an absolute URL (scheme + host)", targetURLStr)
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
		TargetURL:         targetURL,
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
