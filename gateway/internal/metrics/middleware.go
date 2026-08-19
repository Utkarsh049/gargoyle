package metrics

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Middleware instruments HTTP requests, measuring duration and recording
// outcome counters to Prometheus metrics.
func Middleware(m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			r = r.WithContext(NewDecisionContext(r.Context()))

			defer func() {
				duration := time.Since(start).Seconds()
				outcome := GetDecision(r.Context())
				if outcome == "" {
					status := ww.Status()
					if status == 0 {
						// If the handler panicked or never wrote a status, classify as 500 / error
						status = http.StatusInternalServerError
					}
					outcome = outcomeFromStatus(status)
				}

				m.RequestsTotal.WithLabelValues(outcome).Inc()
				m.RequestDuration.WithLabelValues(outcome).Observe(duration)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

func outcomeFromStatus(status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return OutcomeUnauthenticated
	case status == http.StatusTooManyRequests:
		return OutcomeRateLimited
	case status == http.StatusForbidden:
		return OutcomeBlockedAbuse
	case status >= 500:
		return OutcomeError
	default:
		return OutcomeAllowed
	}
}
