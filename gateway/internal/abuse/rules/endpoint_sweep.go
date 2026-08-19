package rules

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"

	"gargoyle/internal/abuse"
)

// SweepTracker tracks and counts distinct endpoints visited by a client/IP
// within a rolling time window.
type SweepTracker interface {
	RecordAndCountDistinctPaths(ctx context.Context, key string, path string, window time.Duration) (int64, error)
}

// EndpointSweepRule detects directory busting, automated scraping sweeps,
// and path enumeration attacks where an IP or client hits many distinct
// endpoints in rapid succession.
type EndpointSweepRule struct {
	tracker   SweepTracker
	threshold int
	window    time.Duration
}

// NewEndpointSweepRule builds a new EndpointSweepRule.
func NewEndpointSweepRule(tracker SweepTracker, threshold int, window time.Duration) *EndpointSweepRule {
	if threshold <= 0 {
		threshold = 10
	}
	if window < time.Millisecond {
		window = 10 * time.Second
	}
	return &EndpointSweepRule{
		tracker:   tracker,
		threshold: threshold,
		window:    window,
	}
}

func (r *EndpointSweepRule) Name() string {
	return "endpoint_sweep"
}

func (r *EndpointSweepRule) Evaluate(ctx context.Context, req *abuse.RequestContext) (abuse.Decision, error) {
	if req == nil || r.tracker == nil {
		return abuse.Decision{Action: abuse.ActionAllow, Score: 0.0, Rule: r.Name()}, nil
	}

	key := fmt.Sprintf("sweep:%s:%s", req.ClientID, req.IP)
	count, err := r.tracker.RecordAndCountDistinctPaths(ctx, key, req.Path, r.window)
	if err != nil {
		// Fail open on tracking error
		return abuse.Decision{Action: abuse.ActionAllow, Score: 0.0, Rule: r.Name()}, nil
	}

	if int(count) > r.threshold {
		return abuse.Decision{
			Action: abuse.ActionBlock,
			Score:  0.90,
			Rule:   r.Name(),
			Reason: fmt.Sprintf("endpoint sweep detected: %d distinct endpoints visited within %v (threshold: %d)", count, r.window, r.threshold),
		}, nil
	}

	// Linear ramp-up score if approaching threshold
	var score float64
	if r.threshold > 0 && count > 1 {
		ratio := float64(count) / float64(r.threshold)
		if ratio >= 0.7 {
			score = math.Min(0.75, ratio*0.75)
		}
	}

	return abuse.Decision{
		Action: abuse.ActionAllow,
		Score:  score,
		Rule:   r.Name(),
		Reason: "",
	}, nil
}

// RedisSweepTracker implements SweepTracker using Redis Sorted Sets with sliding window expiry.
type RedisSweepTracker struct {
	client *redis.Client
	script *redis.Script
}

const sweepScriptLua = `
local key = KEYS[1]
local path = ARGV[1]
local now = tonumber(ARGV[2])
local window_ms = tonumber(ARGV[3])
local ttl_sec = tonumber(ARGV[4])

local min_score = now - window_ms
redis.call('ZREMRANGEBYSCORE', key, '-inf', '(' .. min_score)
redis.call('ZADD', key, now, path)
local count = redis.call('ZCARD', key)
redis.call('EXPIRE', key, ttl_sec)
return count
`

// NewRedisSweepTracker constructs a Redis-backed sweep tracker.
func NewRedisSweepTracker(client *redis.Client) *RedisSweepTracker {
	return &RedisSweepTracker{
		client: client,
		script: redis.NewScript(sweepScriptLua),
	}
}

func (t *RedisSweepTracker) RecordAndCountDistinctPaths(ctx context.Context, key string, path string, window time.Duration) (int64, error) {
	nowMs := time.Now().UnixMilli()
	windowMs := window.Milliseconds()
	ttlSec := int(math.Ceil(window.Seconds())) + 5
	if ttlSec < 5 {
		ttlSec = 5
	}

	res, err := t.script.Run(ctx, t.client, []string{key}, path, nowMs, windowMs, ttlSec).Int64()
	if err != nil {
		return 0, fmt.Errorf("abuse: redis sweep tracking failed: %w", err)
	}
	return res, nil
}
