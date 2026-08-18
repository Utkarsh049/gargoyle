package rules

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"gargoyle/internal/abuse"
)

type mockSweepTracker struct {
	mu    sync.Mutex
	paths map[string]map[string]time.Time
}

func newMockSweepTracker() *mockSweepTracker {
	return &mockSweepTracker{
		paths: make(map[string]map[string]time.Time),
	}
}

func (m *mockSweepTracker) RecordAndCountDistinctPaths(_ context.Context, key string, path string, window time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-window)

	if _, ok := m.paths[key]; !ok {
		m.paths[key] = make(map[string]time.Time)
	}

	// Purge expired
	for p, ts := range m.paths[key] {
		if ts.Before(cutoff) {
			delete(m.paths[key], p)
		}
	}

	m.paths[key][path] = now
	return int64(len(m.paths[key])), nil
}

type errorSweepTracker struct{}

func (e *errorSweepTracker) RecordAndCountDistinctPaths(_ context.Context, _ string, _ string, _ time.Duration) (int64, error) {
	return 0, errors.New("redis down")
}

func TestEndpointSweepRule(t *testing.T) {
	tracker := newMockSweepTracker()
	rule := NewEndpointSweepRule(tracker, 5, 10*time.Second)
	ctx := context.Background()

	req := &abuse.RequestContext{
		ClientID:  "client-1",
		IP:        "192.0.2.1",
		Path:      "/api/v1/users",
		Timestamp: time.Now(),
	}

	// First 5 distinct endpoints should be allowed
	for i := 1; i <= 5; i++ {
		req.Path = fmt.Sprintf("/api/v1/resource-%d", i)
		dec, err := rule.Evaluate(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dec.Action != abuse.ActionAllow {
			t.Fatalf("expected action allow for request %d, got %s (reason: %s)", i, dec.Action, dec.Reason)
		}
	}

	// 6th distinct endpoint exceeds threshold 5 -> Block
	req.Path = "/api/v1/resource-6"
	dec, err := rule.Evaluate(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Action != abuse.ActionBlock {
		t.Fatalf("expected action block for 6th distinct path, got %s", dec.Action)
	}
	if dec.Score < 0.85 {
		t.Fatalf("expected score >= 0.85, got %v", dec.Score)
	}

	// Repeated hit on same path doesn't increase distinct path count
	decSame, err := rule.Evaluate(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decSame.Action != abuse.ActionBlock {
		t.Fatalf("expected repeat hit on blocked sweep state to remain blocked")
	}

	// Different client or IP is separate and not blocked
	reqOtherClient := &abuse.RequestContext{
		ClientID:  "client-2",
		IP:        "192.0.2.2",
		Path:      "/api/v1/resource-1",
		Timestamp: time.Now(),
	}
	decOther, err := rule.Evaluate(ctx, reqOtherClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decOther.Action != abuse.ActionAllow {
		t.Fatalf("expected client-2 request to be allowed, got %s", decOther.Action)
	}
}

func TestEndpointSweepRuleFailOpenOnError(t *testing.T) {
	rule := NewEndpointSweepRule(&errorSweepTracker{}, 5, 10*time.Second)
	req := &abuse.RequestContext{
		ClientID: "client-1",
		IP:       "192.0.2.1",
		Path:     "/api/v1/resource",
	}

	dec, err := rule.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error on fail open, got %v", err)
	}
	if dec.Action != abuse.ActionAllow {
		t.Fatalf("expected action allow on fail open, got %s", dec.Action)
	}
}
