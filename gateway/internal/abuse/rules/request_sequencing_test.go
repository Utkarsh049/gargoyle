package rules

import (
	"context"
	"sync"
	"testing"
	"time"

	"gargoyle/internal/abuse"
)

type mockTimingTracker struct {
	mu      sync.Mutex
	history map[string][]int64
}

func newMockTimingTracker() *mockTimingTracker {
	return &mockTimingTracker{
		history: make(map[string][]int64),
	}
}

func (m *mockTimingTracker) RecordTimestampAndGetHistory(_ context.Context, key string, timestamp time.Time, maxSamples int, _ time.Duration) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	nowMs := timestamp.UnixMilli()
	// LPUSH: newest first
	list := append([]int64{nowMs}, m.history[key]...)
	if len(list) > maxSamples {
		list = list[:maxSamples]
	}
	m.history[key] = list

	copied := make([]int64, len(list))
	copy(copied, list)
	return copied, nil
}

func TestRequestSequencingRuleRoboticPacing(t *testing.T) {
	tracker := newMockTimingTracker()
	rule := NewRequestSequencingRule(tracker)
	ctx := context.Background()

	startTime := time.Now()
	req := &abuse.RequestContext{
		ClientID:  "client-1",
		IP:        "192.0.2.1",
		Path:      "/api/v1/resource",
		Timestamp: startTime,
	}

	// Simulate exact 100ms interval bot loop: t=0ms, t=100ms, t=200ms, t=300ms, t=400ms, t=500ms
	var lastDec abuse.Decision
	var err error
	for i := 0; i < 6; i++ {
		req.Timestamp = startTime.Add(time.Duration(i*100) * time.Millisecond)
		lastDec, err = rule.Evaluate(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error on evaluation %d: %v", i, err)
		}
	}

	// With 6 samples at exact 100ms intervals, standard deviation is 0.0ms -> Block
	if lastDec.Action != abuse.ActionBlock {
		t.Fatalf("expected action block for robotic timing, got %s (reason: %s)", lastDec.Action, lastDec.Reason)
	}
	if lastDec.Score < 0.80 {
		t.Fatalf("expected score >= 0.80, got %v", lastDec.Score)
	}
}

func TestRequestSequencingRuleNaturalJitter(t *testing.T) {
	tracker := newMockTimingTracker()
	rule := NewRequestSequencingRule(tracker)
	ctx := context.Background()

	startTime := time.Now()
	req := &abuse.RequestContext{
		ClientID:  "client-natural",
		IP:        "192.0.2.5",
		Path:      "/api/v1/resource",
		Timestamp: startTime,
	}

	// Human / realistic jitter: 120ms, 450ms, 80ms, 950ms, 300ms, 600ms
	delays := []int{0, 120, 570, 650, 1600, 1900, 2500}
	var lastDec abuse.Decision
	var err error
	for _, d := range delays {
		req.Timestamp = startTime.Add(time.Duration(d) * time.Millisecond)
		lastDec, err = rule.Evaluate(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Human traffic should be allowed
	if lastDec.Action != abuse.ActionAllow {
		t.Fatalf("expected action allow for natural jitter, got %s (reason: %s)", lastDec.Action, lastDec.Reason)
	}
}
