// Package ratelimit implements Gargoyle's sliding-window rate limiter.
// Each client has a configured rate limit (requests per minute) defined in
// Postgres, and limits are enforced atomically per client_id using Redis sorted sets.
package ratelimit

import (
	"context"
	"time"
)

// Result describes the outcome of a rate-limit check.
type Result struct {
	// Allowed indicates whether the incoming request is permitted under the limit.
	Allowed bool

	// Limit is the client's maximum allowed requests within the sliding window.
	Limit int

	// Remaining is the number of remaining requests allowed in the current window.
	Remaining int

	// ResetAfter is the duration until the rate limit window clears or allows
	// the next request (used for Retry-After and X-RateLimit-Reset headers).
	ResetAfter time.Duration
}

// Limiter is the interface for evaluating and recording request rate limits.
// Production code uses RedisLimiter; tests can supply mock implementations.
type Limiter interface {
	// Allow checks whether clientID is within its rate limit for the sliding
	// window, and records the request if allowed.
	Allow(ctx context.Context, clientID string, limit int) (Result, error)
}
