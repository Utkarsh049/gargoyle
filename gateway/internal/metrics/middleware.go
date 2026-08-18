package metrics

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Middleware instruments HTTP requests, measuring duration and recording
// outcome counters to Prometheus metrics (see PROJECT.md §2 and §4).
func Middleware(m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			duration := time.Since(start).Seconds()
			outcome := outcomeFromStatus(ww.Status())

			m.RequestsTotal.WithLabelValues(outcome).Inc()
			m.RequestDuration.WithLabelValues(outcome).Observe(duration)
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
