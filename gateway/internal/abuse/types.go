// Package abuse implements Gargoyle's heuristic and ML abuse detection
// engine (see PROJECT.md §6).
//
// Abuse detection evaluates requests for malicious traffic patterns that
// stay under normal rate limits (such as scraping sweeps, robotic timing
// pacing, and header anomalies).
package abuse

import (
	"context"
	"net/http"
	"time"
)

// Action represents the decision action taken by the abuse engine.
type Action string

const (
	// ActionAllow permits the request through to the upstream proxy.
	ActionAllow Action = "allow"

	// ActionBlock rejects the request with 403 Forbidden.
	ActionBlock Action = "block"

	// ActionFlag permits the request but tags it for operator review.
	ActionFlag Action = "flag"
)

// Decision describes the outcome of an abuse check.
type Decision struct {
	// Action is the enforcement action (allow, block, flag).
	Action Action

	// Score is the computed risk score from 0.0 (clean) to 1.0 (certain abuse).
	Score float64

	// Rule is the name of the heuristic rule or model that produced this decision.
	Rule string

	// Reason is a human-readable explanation of why the rule fired.
	Reason string
}

// RequestContext contains normalized request features passed to heuristic rules.
type RequestContext struct {
	ClientID  string
	IP        string
	Path      string
	Method    string
	Header    http.Header
	UserAgent string
	Timestamp time.Time
}

// Rule is the interface implemented by all heuristic checks and ML scorers.
type Rule interface {
	// Name returns the unique identifier for this rule.
	Name() string

	// Evaluate analyzes the request context and returns an abuse decision.
	Evaluate(ctx context.Context, req *RequestContext) (Decision, error)
}
