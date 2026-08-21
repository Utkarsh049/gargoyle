package admin

import (
	"log/slog"
	"net/http"
	"strings"
)

// AuthMiddleware enforces admin key authorization on admin endpoints when an admin key is configured.
func AuthMiddleware(adminKey string, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if adminKey == "" {
				// Open access in development mode
				next.ServeHTTP(w, r)
				return
			}

			// Check X-Admin-Key header
			key := r.Header.Get("X-Admin-Key")

			// Check Authorization: Bearer <key>
			if key == "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					key = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			// Check query parameter ?admin_key=...
			if key == "" {
				key = r.URL.Query().Get("admin_key")
			}

			if key != adminKey {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized: invalid or missing admin key"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
