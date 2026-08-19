package ratelimit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// keyPrefix scopes rate-limit keys in Redis.
const keyPrefix = "gargoyle:ratelimit:"

// slidingWindowScript atomically checks and updates rate limits using Redis sorted sets.
var slidingWindowScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]
local clear_before = now - window

-- 1. Remove entries outside the current sliding window
redis.call('ZREMRANGEBYSCORE', key, '-inf', clear_before)

-- 2. Count current entries in the window
local current_count = redis.call('ZCARD', key)

if current_count < limit then
    -- Allowed: record request timestamp and refresh key TTL
    redis.call('ZADD', key, now, member)
    redis.call('PEXPIRE', key, window)
    local remaining = limit - current_count - 1
    local reset_seconds = math.ceil(window / 1000)
    return {1, remaining, reset_seconds}
else
    -- Exceeded: calculate time until oldest entry in current window expires
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local reset_ms = window
    if #oldest > 0 then
        local oldest_score = tonumber(oldest[2])
        local ttl_ms = (oldest_score + window) - now
        if ttl_ms > 0 then
            reset_ms = ttl_ms
        end
    end
    local reset_seconds = math.ceil(reset_ms / 1000)
    return {0, 0, reset_seconds}
end
`)

// RedisLimiter implements Limiter using Redis sorted sets.
type RedisLimiter struct {
	rdb    *redis.Client
	window time.Duration
}

// NewRedisLimiter creates a RedisLimiter with the specified window duration.
func NewRedisLimiter(rdb *redis.Client, window time.Duration) *RedisLimiter {
	if window < time.Millisecond {
		window = time.Millisecond
	}
	return &RedisLimiter{
		rdb:    rdb,
		window: window,
	}
}

// Allow evaluates clientID's rate limit over the configured sliding window.
func (r *RedisLimiter) Allow(ctx context.Context, clientID string, limit int) (Result, error) {
	if limit <= 0 {
		return Result{
			Allowed:    true,
			Limit:      limit,
			Remaining:  0,
			ResetAfter: 0,
		}, nil
	}

	now := time.Now()
	nowMs := now.UnixMilli()
	windowMs := r.window.Milliseconds()
	member := generateMember(now.UnixNano())
	key := keyPrefix + clientID

	res, err := slidingWindowScript.Run(ctx, r.rdb, []string{key}, nowMs, windowMs, limit, member).Result()
	if err != nil {
		return Result{}, fmt.Errorf("ratelimit: redis script failed: %w", err)
	}

	vals, ok := res.([]any)
	if !ok || len(vals) < 3 {
		return Result{}, fmt.Errorf("ratelimit: unexpected script response format: %v", res)
	}

	allowedVal, _ := vals[0].(int64)
	remainingVal, _ := vals[1].(int64)
	resetSecVal, _ := vals[2].(int64)

	return Result{
		Allowed:    allowedVal == 1,
		Limit:      limit,
		Remaining:  int(remainingVal),
		ResetAfter: time.Duration(resetSecVal) * time.Second,
	}, nil
}

func generateMember(nano int64) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return strconv.FormatInt(nano, 10) + "-" + hex.EncodeToString(b)
}
