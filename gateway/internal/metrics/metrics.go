// Package metrics defines Gargoyle's aggregate Prometheus metrics and
// HTTP measurement middleware.
//
// Metrics are strictly aggregate (labelled by outcome, not client ID)
// to prevent time-series cardinality blowup. High-cardinality per-client
// logs are persisted to Postgres instead.
package metrics

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Possible outcome label values for gargoyle_requests_total and gargoyle_request_duration_seconds.
const (
	OutcomeAllowed         = "allowed"
	OutcomeRateLimited     = "rate_limited"
	OutcomeUnauthenticated = "unauthenticated"
	OutcomeBlockedAbuse    = "blocked_abuse"
	OutcomeError           = "error"
)

// Metrics holds the registered Prometheus collectors for the Gargoyle core.
type Metrics struct {
	// RequestsTotal counts processed requests partitioned by outcome.
	RequestsTotal *prometheus.CounterVec

	// RequestDuration measures request handling duration in seconds by outcome.
	RequestDuration *prometheus.HistogramVec

	// ActiveClients gauges the current count of active/registered clients.
	ActiveClients prometheus.Gauge

	// AbuseScoreDistribution observes heuristic and ML abuse scores (0.0 - 1.0).
	AbuseScoreDistribution prometheus.Histogram
}

// New creates and registers Gargoyle's Prometheus metrics with reg.
// If reg is nil, prometheus.DefaultRegisterer is used.
func New(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)

	return &Metrics{
		RequestsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "gargoyle",
				Name:      "requests_total",
				Help:      "Total number of HTTP requests processed by outcome.",
			},
			[]string{"outcome"},
		),

		RequestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "gargoyle",
				Name:      "request_duration_seconds",
				Help:      "Duration of HTTP request processing in seconds by outcome.",
				Buckets: []float64{
					0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
				},
			},
			[]string{"outcome"},
		),

		ActiveClients: factory.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "gargoyle",
				Name:      "active_clients",
				Help:      "Number of registered active clients in the gateway.",
			},
		),

		AbuseScoreDistribution: factory.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "gargoyle",
				Name:      "abuse_score_distribution",
				Help:      "Distribution of abuse scores evaluated across requests.",
				Buckets: []float64{
					0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
				},
			},
		),
	}
}

type decisionKey struct{}

type decisionHolder struct {
	outcome string
}

// NewDecisionContext returns a child context carrying a mutable decisionHolder.
func NewDecisionContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, decisionKey{}, &decisionHolder{})
}

// SetDecision records the gateway's decision for the request (e.g. OutcomeAllowed, OutcomeRateLimited).
func SetDecision(ctx context.Context, outcome string) {
	if h, ok := ctx.Value(decisionKey{}).(*decisionHolder); ok && h != nil {
		h.outcome = outcome
	}
}

// GetDecision retrieves the gateway decision recorded on ctx, if any.
func GetDecision(ctx context.Context) string {
	if h, ok := ctx.Value(decisionKey{}).(*decisionHolder); ok && h != nil {
		return h.outcome
	}
	return ""
}
