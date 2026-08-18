package ratelimit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisLimiterZeroOrNegativeLimit(t *testing.T) {
	limiter := NewRedisLimiter(nil, time.Minute)

	res, err := limiter.Allow(context.Background(), "client-1", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Allowed {
		t.Fatal("expected limit <= 0 to be allowed")
	}

	res, err = limiter.Allow(context.Background(), "client-1", -10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Allowed {
		t.Fatal("expected negative limit to be allowed")
	}
}

func TestRedisLimiterIntegration(t *testing.T) {
	redisURL := os.Getenv("GARGOYLE_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("skipping Redis integration test: GARGOYLE_TEST_REDIS_URL not set")
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("invalid redis url: %v", err)
	}

	rdb := redis.NewClient(opts)
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("skipping Redis integration test: cannot ping redis: %v", err)
	}

	clientID := "test-client-sliding-window"
	_ = rdb.Del(ctx, keyPrefix+clientID).Err()
	defer func() {
		_ = rdb.Del(context.Background(), keyPrefix+clientID).Err()
	}()

	window := 2 * time.Second
	limiter := NewRedisLimiter(rdb, window)
	limit := 3

	// 1st request -> allowed (remaining 2)
	res1, err := limiter.Allow(ctx, clientID, limit)
	if err != nil || !res1.Allowed || res1.Remaining != 2 {
		t.Fatalf("req 1 failed: res=%+v err=%v", res1, err)
	}

	// 2nd request -> allowed (remaining 1)
	res2, err := limiter.Allow(ctx, clientID, limit)
	if err != nil || !res2.Allowed || res2.Remaining != 1 {
		t.Fatalf("req 2 failed: res=%+v err=%v", res2, err)
	}

	// 3rd request -> allowed (remaining 0)
	res3, err := limiter.Allow(ctx, clientID, limit)
	if err != nil || !res3.Allowed || res3.Remaining != 0 {
		t.Fatalf("req 3 failed: res=%+v err=%v", res3, err)
	}

	// 4th request -> blocked (rate limited)
	res4, err := limiter.Allow(ctx, clientID, limit)
	if err != nil || res4.Allowed || res4.Remaining != 0 {
		t.Fatalf("req 4 expected blocked: res=%+v err=%v", res4, err)
	}
	if res4.ResetAfter <= 0 {
		t.Fatalf("expected positive ResetAfter duration, got %v", res4.ResetAfter)
	}

	// Wait for window to slide and clear
	time.Sleep(window + 100*time.Millisecond)

	// 5th request -> allowed again
	res5, err := limiter.Allow(ctx, clientID, limit)
	if err != nil || !res5.Allowed {
		t.Fatalf("req 5 expected allowed after window expiry: res=%+v err=%v", res5, err)
	}
}
