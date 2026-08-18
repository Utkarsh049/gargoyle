package abuse

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"gargoyle/internal/client"
	"gargoyle/internal/logstore"
	"gargoyle/internal/metrics"
)

type mockLogStore struct {
	mu      sync.Mutex
	entries []logstore.Entry
}

func (m *mockLogStore) Write(_ context.Context, entry logstore.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockLogStore) FindRecentByClientID(_ context.Context, _ string, _ int) ([]logstore.Entry, error) {
	return nil, nil
}

func (m *mockLogStore) Entries() []logstore.Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]logstore.Entry, len(m.entries))
	copy(copied, m.entries)
	return copied
}

func TestAbuseMiddlewareAllowed(t *testing.T) {
	engine := NewEngine(0.8, &mockRule{
		name: "clean_rule",
		evalFunc: func(_ context.Context, _ *RequestContext) (Decision, error) {
			return Decision{Action: ActionAllow, Score: 0.1}, nil
		},
	})

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	promMetrics := metrics.New(prometheus.NewRegistry())
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	testClient := &client.Client{ID: "client-1", Name: "legit-corp"}
	mw := Middleware(engine, nil, promMetrics, logger)(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/data", nil)
	req = req.WithContext(client.NewContext(req.Context(), testClient))
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if !nextCalled {
		t.Fatal("expected downstream handler to be called for allowed request")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestAbuseMiddlewareBlocked(t *testing.T) {
	engine := NewEngine(0.8, &mockRule{
		name: "bot_detector",
		evalFunc: func(_ context.Context, _ *RequestContext) (Decision, error) {
			return Decision{
				Action: ActionBlock,
				Score:  0.95,
				Rule:   "bot_detector",
				Reason: "automated scraper detected",
			}, nil
		},
	})

	fakeLogStore := &mockLogStore{}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	promMetrics := metrics.New(prometheus.NewRegistry())

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	testClient := &client.Client{ID: "client-bad", Name: "scraper-bot"}
	mw := Middleware(engine, fakeLogStore, promMetrics, logger)(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets", nil)
	req.RemoteAddr = "192.0.2.99:54321"
	req = req.WithContext(metrics.NewDecisionContext(req.Context()))
	req = req.WithContext(client.NewContext(req.Context(), testClient))
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if nextCalled {
		t.Fatal("downstream handler should not be called for blocked request")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}

	var resp abuseErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != "request blocked due to abuse detection" || resp.Reason != "automated scraper detected" {
		t.Fatalf("unexpected error response: %+v", resp)
	}

	if metrics.GetDecision(req.Context()) != metrics.OutcomeBlockedAbuse {
		t.Fatalf("expected decision %q, got %q", metrics.OutcomeBlockedAbuse, metrics.GetDecision(req.Context()))
	}

	// Wait briefly for async log write
	time.Sleep(50 * time.Millisecond)
	entries := fakeLogStore.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].ClientID != "client-bad" || entries[0].Outcome != "blocked_abuse" || entries[0].AbuseScore != 0.95 {
		t.Fatalf("unexpected log entry: %+v", entries[0])
	}
}
