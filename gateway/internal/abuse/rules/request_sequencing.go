package rules

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"gargoyle/internal/abuse"
)

// TimingTracker records and retrieves recent request timestamps for an IP/client.
type TimingTracker interface {
	RecordTimestampAndGetHistory(ctx context.Context, key string, timestamp time.Time, maxSamples int, ttl time.Duration) ([]int64, error)
}

// RequestSequencingRule detects robotic/automated bot scripts by analyzing
// the uniformity of inter-arrival time intervals between consecutive requests.
//
// Human traffic naturally contains high timing jitter (standard deviation > 100ms),
// whereas automated scripts with fixed loops/sleeps exhibit near-zero interval
// variance (see PROJECT.md §6).
type RequestSequencingRule struct {
	tracker        TimingTracker
	minSamples     int
	maxSamples     int
	stdDevThreshMs float64
	ttl            time.Duration
}

// NewRequestSequencingRule constructs a new RequestSequencingRule.
func NewRequestSequencingRule(tracker TimingTracker) *RequestSequencingRule {
	return &RequestSequencingRule{
		tracker:        tracker,
		minSamples:     5,
		maxSamples:     10,
		stdDevThreshMs: 15.0, // standard deviation < 15ms indicates robotic pacing
		ttl:            30 * time.Second,
	}
}

func (r *RequestSequencingRule) Name() string {
	return "request_sequencing"
}

func (r *RequestSequencingRule) Evaluate(ctx context.Context, req *abuse.RequestContext) (abuse.Decision, error) {
	if req == nil || r.tracker == nil {
		return abuse.Decision{Action: abuse.ActionAllow, Score: 0.0, Rule: r.Name()}, nil
	}

	ts := req.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	key := fmt.Sprintf("timing:%s:%s", req.ClientID, req.IP)
	history, err := r.tracker.RecordTimestampAndGetHistory(ctx, key, ts, r.maxSamples, r.ttl)
	if err != nil {
		// Fail-open on tracking error
		return abuse.Decision{Action: abuse.ActionAllow, Score: 0.0, Rule: r.Name()}, nil
	}

	// We need at least minSamples to calculate interval variance
	if len(history) < r.minSamples {
		return abuse.Decision{Action: abuse.ActionAllow, Score: 0.0, Rule: r.Name()}, nil
	}

	// History is in reverse chronological order: [t_n, t_{n-1}, ..., t_1]
	// Compute intervals between consecutive requests
	intervals := make([]float64, len(history)-1)
	var sum float64
	for i := 0; i < len(history)-1; i++ {
		delta := float64(history[i] - history[i+1])
		if delta < 0 {
			delta = -delta
		}
		intervals[i] = delta
		sum += delta
	}

	mean := sum / float64(len(intervals))

	// If mean interval is greater than 10 seconds, ignore timing uniformity
	if mean > 10000 {
		return abuse.Decision{Action: abuse.ActionAllow, Score: 0.0, Rule: r.Name()}, nil
	}

	// Compute sample variance and standard deviation
	var varianceSum float64
	for _, interval := range intervals {
		diff := interval - mean
		varianceSum += diff * diff
	}
	stdDev := math.Sqrt(varianceSum / float64(len(intervals)))

	// Check for robotic timing uniformity (stdDev < threshold)
	if stdDev < r.stdDevThreshMs {
		return abuse.Decision{
			Action: abuse.ActionBlock,
			Score:  0.85,
			Rule:   r.Name(),
			Reason: fmt.Sprintf("robotic timing uniformity detected: interval standard deviation %.2fms (mean %.2fms) across %d requests", stdDev, mean, len(history)),
		}, nil
	}

	return abuse.Decision{
		Action: abuse.ActionAllow,
		Score:  0.0,
		Rule:   r.Name(),
		Reason: "",
	}, nil
}

// RedisTimingTracker stores recent request timestamps in a bounded Redis List.
type RedisTimingTracker struct {
	client *redis.Client
	script *redis.Script
}

const timingScriptLua = `
local key = KEYS[1]
local now_ms = tonumber(ARGV[1])
local max_samples = tonumber(ARGV[2])
local ttl_sec = tonumber(ARGV[3])

redis.call('LPUSH', key, now_ms)
redis.call('LTRIM', key, 0, max_samples - 1)
redis.call('EXPIRE', key, ttl_sec)
return redis.call('LRANGE', key, 0, max_samples - 1)
`

// NewRedisTimingTracker constructs a Redis-backed timing tracker.
func NewRedisTimingTracker(client *redis.Client) *RedisTimingTracker {
	return &RedisTimingTracker{
		client: client,
		script: redis.NewScript(timingScriptLua),
	}
}

func (t *RedisTimingTracker) RecordTimestampAndGetHistory(ctx context.Context, key string, timestamp time.Time, maxSamples int, ttl time.Duration) ([]int64, error) {
	nowMs := timestamp.UnixMilli()
	ttlSec := int(math.Ceil(ttl.Seconds()))
	if ttlSec < 5 {
		ttlSec = 5
	}

	raw, err := t.script.Run(ctx, t.client, []string{key}, nowMs, maxSamples, ttlSec).StringSlice()
	if err != nil {
		return nil, fmt.Errorf("abuse: redis timing tracking failed: %w", err)
	}

	res := make([]int64, 0, len(raw))
	for _, s := range raw {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			continue
		}
		res = append(res, v)
	}
	return res, nil
}
