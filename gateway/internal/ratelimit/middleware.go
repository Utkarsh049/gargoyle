package ratelimit

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"gargoyle/internal/client"
)

type rateLimitErrorResponse struct {
	Error      string `json:"error"`
	RetryAfter int    `json:"retry_after"`
}

// Middleware enforces rate limits per client based on each client's configured
// limit from Postgres (see PROJECT.md §5).
//
// It runs after client.Middleware in the router stack so client.FromContext is
// guaranteed to succeed. If Redis is down or returns an error, it logs the failure
// and fails open so downstream services remain accessible.
func Middleware(limiter Limiter, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, ok := client.FromContext(r.Context())
			if !ok {
				logger.ErrorContext(r.Context(), "ratelimit: no client in context")
				next.ServeHTTP(w, r)
				return
			}

			// If client has no rate limit configured or <= 0, allow unlimited
			if c.RateLimit <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			res, err := limiter.Allow(r.Context(), c.ID, c.RateLimit)
			if err != nil {
				// Fail-open strategy: log Redis failure and let request through
				logger.ErrorContext(r.Context(), "ratelimit: check failed, failing open",
					"client_id", c.ID,
					"client_name", c.Name,
					"error", err,
				)
				next.ServeHTTP(w, r)
				return
			}

			resetSec := int(math.Ceil(res.ResetAfter.Seconds()))
			if resetSec < 1 {
				resetSec = 1
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.Itoa(resetSec))

			if !res.Allowed {
				w.Header().Set("Retry-After", strconv.Itoa(resetSec))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)

				_ = json.NewEncoder(w).Encode(rateLimitErrorResponse{
					Error:      "rate limit exceeded",
					RetryAfter: resetSec,
				})

				logger.WarnContext(r.Context(), "ratelimit: client throttled",
					"client_id", c.ID,
					"client_name", c.Name,
					"limit", res.Limit,
					"retry_after_seconds", resetSec,
				)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
