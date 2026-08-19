package client

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"gargoyle/internal/metrics"
)

// APIKeyHeader is the header containing the client's API key.
const APIKeyHeader = "X-Gargoyle-Key"

// Middleware authenticates requests by API key and attaches the resolved Client to context.
func Middleware(registry *Registry, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get(APIKeyHeader)
			if apiKey == "" {
				metrics.SetDecision(r.Context(), metrics.OutcomeUnauthenticated)
				writeAuthError(w, "missing API key")
				return
			}

			c, err := registry.Lookup(r.Context(), apiKey)
			if err != nil {
				if !errors.Is(err, ErrNotFound) {
					logger.ErrorContext(r.Context(), "client: lookup failed", "error", err)
				}
				metrics.SetDecision(r.Context(), metrics.OutcomeUnauthenticated)
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
