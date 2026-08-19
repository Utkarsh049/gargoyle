package abuse

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"gargoyle/internal/client"
	"gargoyle/internal/logstore"
	"gargoyle/internal/metrics"
)

type abuseErrorResponse struct {
	Error  string  `json:"error"`
	Reason string  `json:"reason,omitempty"`
	Score  float64 `json:"score,omitempty"`
}

// Middleware runs heuristic and ML abuse detection rules on incoming
// authenticated requests after rate limiting.
//
// Requests with an abuse score meeting or exceeding the block threshold are
// rejected immediately with 403 Forbidden, recorded to Prometheus metrics,
// and asynchronously logged to Postgres (request_logs table).
func Middleware(engine *Engine, logStore logstore.Store, promMetrics *metrics.Metrics, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if engine == nil {
				next.ServeHTTP(w, r)
				return
			}

			c, ok := client.FromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			ip := extractClientIP(r.RemoteAddr)
			reqCtx := &RequestContext{
				ClientID:  c.ID,
				IP:        ip,
				Path:      r.URL.Path,
				Method:    r.Method,
				Header:    r.Header,
				UserAgent: r.UserAgent(),
				Timestamp: time.Now(),
			}

			decision, err := engine.Evaluate(r.Context(), reqCtx)
			if err != nil {
				// Fail open on unexpected engine error
				logger.ErrorContext(r.Context(), "abuse: evaluation error, failing open", "error", err)
				next.ServeHTTP(w, r)
				return
			}

			if promMetrics != nil {
				promMetrics.AbuseScoreDistribution.Observe(decision.Score)
			}

			if decision.Action == ActionBlock {
				metrics.SetDecision(r.Context(), metrics.OutcomeBlockedAbuse)

				logger.WarnContext(r.Context(), "abuse: request blocked",
					"client_id", c.ID,
					"client_name", c.Name,
					"ip", ip,
					"path", r.URL.Path,
					"rule", decision.Rule,
					"score", decision.Score,
					"reason", decision.Reason,
				)

				if logStore != nil {
					go func(clientID, clientIP, path, reason string, score float64) {
						ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
						defer cancel()

						if err := logStore.Write(ctx, logstore.Entry{
							ClientID:   clientID,
							Timestamp:  time.Now().UTC(),
							IP:         clientIP,
							Path:       path,
							Outcome:    metrics.OutcomeBlockedAbuse,
							AbuseScore: score,
							Reason:     reason,
						}); err != nil {
							logger.ErrorContext(ctx, "logstore: failed to record blocked abuse request",
								"client_id", clientID,
								"error", err,
							)
						}
					}(c.ID, ip, r.URL.Path, decision.Reason, decision.Score)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(abuseErrorResponse{
					Error:  "request blocked due to abuse detection",
					Reason: decision.Reason,
					Score:  decision.Score,
				})
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
