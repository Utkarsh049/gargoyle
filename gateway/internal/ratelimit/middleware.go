package ratelimit

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"gargoyle/internal/client"
	"gargoyle/internal/logstore"
	"gargoyle/internal/metrics"
)

type rateLimitErrorResponse struct {
	Error      string `json:"error"`
	RetryAfter int    `json:"retry_after"`
}

// Middleware enforces rate limits per tenant based on client configuration.
func Middleware(limiter Limiter, logStore logstore.Store, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, ok := client.FromContext(r.Context())
			if !ok {
				logger.ErrorContext(r.Context(), "ratelimit: no client in context")
				next.ServeHTTP(w, r)
				return
			}

			if c.RateLimit <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			res, err := limiter.Allow(r.Context(), c.ID, c.RateLimit)
			if err != nil {
				// Fail open on Redis errors
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
				metrics.SetDecision(r.Context(), metrics.OutcomeRateLimited)
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

				if logStore != nil {
					go func(clientID, ip, path string) {
						ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
						defer cancel()

						if err := logStore.Write(ctx, logstore.Entry{
							ClientID:   clientID,
							Timestamp:  time.Now().UTC(),
							IP:         ip,
							Path:       path,
							Outcome:    metrics.OutcomeRateLimited,
							AbuseScore: 0.0,
							Reason:     "rate limit exceeded",
						}); err != nil {
							logger.ErrorContext(ctx, "logstore: failed to record rate-limited request",
								"client_id", clientID,
								"error", err,
							)
						}
					}(c.ID, extractClientIP(r.RemoteAddr), r.URL.Path)
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// PreAuthMiddleware enforces IP-based rate limiting before authentication.
func PreAuthMiddleware(limiter Limiter, limit int, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limit <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			ip := extractClientIP(r.RemoteAddr)
			key := "ip:" + ip

			res, err := limiter.Allow(r.Context(), key, limit)
			if err != nil {
				// Fail-open on Redis error
				logger.ErrorContext(r.Context(), "ratelimit: pre-auth check failed, failing open",
					"ip", ip,
					"error", err,
				)
				next.ServeHTTP(w, r)
				return
			}

			resetSec := int(math.Ceil(res.ResetAfter.Seconds()))
			if resetSec < 1 {
				resetSec = 1
			}

			if !res.Allowed {
				metrics.SetDecision(r.Context(), metrics.OutcomeRateLimited)
				w.Header().Set("Retry-After", strconv.Itoa(resetSec))
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", strconv.Itoa(resetSec))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)

				_ = json.NewEncoder(w).Encode(rateLimitErrorResponse{
					Error:      "rate limit exceeded",
					RetryAfter: resetSec,
				})

				logger.WarnContext(r.Context(), "ratelimit: pre-auth request throttled",
					"ip", ip,
					"limit", res.Limit,
					"retry_after_seconds", resetSec,
				)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func extractClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
