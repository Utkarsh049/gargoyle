// Package ratelimit provides sliding-window rate limiting.
package ratelimit

import (
	"context"
	"time"
)

// Result describes the outcome of a rate-limit check.
type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	ResetAfter time.Duration
}

// Limiter defines the rate limiting interface.
type Limiter interface {
	// Allow checks and records whether clientID is within its rate limit for the window.
	Allow(ctx context.Context, clientID string, limit int) (Result, error)
}
