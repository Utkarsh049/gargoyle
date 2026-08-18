package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsMiddlewareOutcomes(t *testing.T) {
	tests := []struct {
		name            string
		status          int
		expectedOutcome string
	}{
		{"200 OK", http.StatusOK, OutcomeAllowed},
		{"201 Created", http.StatusCreated, OutcomeAllowed},
		{"401 Unauthorized", http.StatusUnauthorized, OutcomeUnauthenticated},
		{"429 Too Many Requests", http.StatusTooManyRequests, OutcomeRateLimited},
		{"403 Forbidden Abuse", http.StatusForbidden, OutcomeBlockedAbuse},
		{"500 Internal Server Error", http.StatusInternalServerError, OutcomeError},
		{"502 Bad Gateway", http.StatusBadGateway, OutcomeError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			m := New(reg)

			handler := Middleware(m)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tc.status {
				t.Fatalf("expected status %d, got %d", tc.status, rec.Code)
			}

			count := testutil.ToFloat64(m.RequestsTotal.WithLabelValues(tc.expectedOutcome))
			if count != 1 {
				t.Fatalf("expected outcome %q counter to be 1, got %v", tc.expectedOutcome, count)
			}
		})
	}
}

func TestActiveClientsGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	m.ActiveClients.Set(42)
	val := testutil.ToFloat64(m.ActiveClients)
	if val != 42 {
		t.Fatalf("expected ActiveClients 42, got %v", val)
	}
}

func TestPrometheusScrapeEndpoint(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	// Simulate some traffic
	m.RequestsTotal.WithLabelValues(OutcomeAllowed).Inc()
	m.RequestsTotal.WithLabelValues(OutcomeRateLimited).Inc()
	m.ActiveClients.Set(5)

	scraper := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	scraper.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 from /metrics, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `gargoyle_requests_total{outcome="allowed"} 1`) {
		t.Fatalf("expected gargoyle_requests_total allowed in scrape output, got:\n%s", body)
	}
	if !strings.Contains(body, `gargoyle_requests_total{outcome="rate_limited"} 1`) {
		t.Fatalf("expected gargoyle_requests_total rate_limited in scrape output, got:\n%s", body)
	}
	if !strings.Contains(body, `gargoyle_active_clients 5`) {
		t.Fatalf("expected gargoyle_active_clients 5 in scrape output, got:\n%s", body)
	}
}
