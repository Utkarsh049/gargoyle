package ratelimit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gargoyle/internal/client"
)

type mockLimiter struct {
	allowFunc func(ctx context.Context, clientID string, limit int) (Result, error)
}

func (m *mockLimiter) Allow(ctx context.Context, clientID string, limit int) (Result, error) {
	return m.allowFunc(ctx, clientID, limit)
}

func TestMiddlewareAllowed(t *testing.T) {
	testClient := &client.Client{
		ID:        "client-123",
		Name:      "test-client",
		RateLimit: 60,
	}

	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, clientID string, limit int) (Result, error) {
			if clientID != "client-123" || limit != 60 {
				t.Fatalf("unexpected call args: clientID=%s limit=%d", clientID, limit)
			}
			return Result{
				Allowed:    true,
				Limit:      60,
				Remaining:  59,
				ResetAfter: 30 * time.Second,
			}, nil
		},
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend response"))
	})

	mw := Middleware(limiter, logger)(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(client.NewContext(req.Context(), testClient))
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if !nextCalled {
		t.Fatal("expected downstream handler to be called for allowed request")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-RateLimit-Limit") != "60" {
		t.Fatalf("expected X-RateLimit-Limit=60, got %q", rec.Header().Get("X-RateLimit-Limit"))
	}
	if rec.Header().Get("X-RateLimit-Remaining") != "59" {
		t.Fatalf("expected X-RateLimit-Remaining=59, got %q", rec.Header().Get("X-RateLimit-Remaining"))
	}
	if rec.Header().Get("X-RateLimit-Reset") != "30" {
		t.Fatalf("expected X-RateLimit-Reset=30, got %q", rec.Header().Get("X-RateLimit-Reset"))
	}
}

func TestMiddlewareThrottled(t *testing.T) {
	testClient := &client.Client{
		ID:        "client-123",
		Name:      "test-client",
		RateLimit: 10,
	}

	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, clientID string, limit int) (Result, error) {
			return Result{
				Allowed:    false,
				Limit:      10,
				Remaining:  0,
				ResetAfter: 15 * time.Second,
			}, nil
		},
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	mw := Middleware(limiter, logger)(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(client.NewContext(req.Context(), testClient))
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if nextCalled {
		t.Fatal("downstream handler should not be called when throttled")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "15" {
		t.Fatalf("expected Retry-After=15, got %q", rec.Header().Get("Retry-After"))
	}
	if rec.Header().Get("X-RateLimit-Limit") != "10" {
		t.Fatalf("expected X-RateLimit-Limit=10, got %q", rec.Header().Get("X-RateLimit-Limit"))
	}
	if rec.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("expected X-RateLimit-Remaining=0, got %q", rec.Header().Get("X-RateLimit-Remaining"))
	}
	if rec.Header().Get("X-RateLimit-Reset") != "15" {
		t.Fatalf("expected X-RateLimit-Reset=15, got %q", rec.Header().Get("X-RateLimit-Reset"))
	}

	var resp rateLimitErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}
	if resp.Error != "rate limit exceeded" || resp.RetryAfter != 15 {
		t.Fatalf("unexpected error response body: %+v", resp)
	}
}

func TestMiddlewareFailOpenOnLimiterError(t *testing.T) {
	testClient := &client.Client{
		ID:        "client-123",
		Name:      "test-client",
		RateLimit: 50,
	}

	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, clientID string, limit int) (Result, error) {
			return Result{}, errors.New("redis connection refused")
		},
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	mw := Middleware(limiter, logger)(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(client.NewContext(req.Context(), testClient))
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if !nextCalled {
		t.Fatal("expected request to fail open and reach downstream handler when Redis fails")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestMiddlewareUnlimitedClient(t *testing.T) {
	testClient := &client.Client{
		ID:        "client-unlimited",
		Name:      "unlimited-client",
		RateLimit: 0, // unlimited
	}

	limiterCalled := false
	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, clientID string, limit int) (Result, error) {
			limiterCalled = true
			return Result{}, nil
		},
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	mw := Middleware(limiter, logger)(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(client.NewContext(req.Context(), testClient))
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if limiterCalled {
		t.Fatal("limiter should not be queried for clients with rate_limit <= 0")
	}
	if !nextCalled {
		t.Fatal("downstream handler should be called for unlimited clients")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}
