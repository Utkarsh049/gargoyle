package client

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// APIKeyHeader is the header clients send their Gargoyle API key in (see
// PROJECT.md §7 — the header-based identification pattern).
const APIKeyHeader = "X-Gargoyle-Key"

// Middleware extracts the caller's API key, resolves it to a Client via
// registry, and attaches the Client to the request context via
// NewContext. Requests with a missing or unrecognized key are rejected
// with 401 before reaching anything downstream — every handler behind this
// middleware can assume client.FromContext will succeed.
func Middleware(registry *Registry, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get(APIKeyHeader)
			if apiKey == "" {
				writeAuthError(w, "missing API key")
				return
			}

			c, err := registry.Lookup(r.Context(), apiKey)
			if err != nil {
				if !errors.Is(err, ErrNotFound) {
					logger.ErrorContext(r.Context(), "client: lookup failed", "error", err)
				}
				writeAuthError(w, "invalid API key")
				return
			}

			next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), c)))
		})
	}
}

func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
