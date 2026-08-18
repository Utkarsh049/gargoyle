package logstore

import (
	"context"
	"sync"
	"testing"
	"time"
)

// FakeStore is an in-memory Store implementation for unit tests.
type FakeStore struct {
	mu      sync.Mutex
	entries []Entry
}

func NewFakeStore() *FakeStore {
	return &FakeStore{entries: make([]Entry, 0)}
}

func (f *FakeStore) Write(_ context.Context, entry Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	f.entries = append(f.entries, entry)
	return nil
}

func (f *FakeStore) FindRecentByClientID(_ context.Context, clientID string, limit int) ([]Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var matched []Entry
	for i := len(f.entries) - 1; i >= 0; i-- {
		if f.entries[i].ClientID == clientID {
			matched = append(matched, f.entries[i])
			if limit > 0 && len(matched) >= limit {
				break
			}
		}
	}
	return matched, nil
}

func (f *FakeStore) Entries() []Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	copied := make([]Entry, len(f.entries))
	copy(copied, f.entries)
	return copied
}

func TestFakeStoreWriteAndQuery(t *testing.T) {
	store := NewFakeStore()
	ctx := context.Background()

	e1 := Entry{
		ClientID:   "client-1",
		Timestamp:  time.Now().Add(-10 * time.Minute),
		IP:         "192.0.2.1",
		Path:       "/api/v1/resource",
		Outcome:    "rate_limited",
		AbuseScore: 0.0,
		Reason:     "rate limit exceeded",
	}

	e2 := Entry{
		ClientID:   "client-2",
		Timestamp:  time.Now().Add(-5 * time.Minute),
		IP:         "192.0.2.2",
		Path:       "/api/v1/other",
		Outcome:    "rate_limited",
		AbuseScore: 0.0,
		Reason:     "rate limit exceeded",
	}

	e3 := Entry{
		ClientID:   "client-1",
		Timestamp:  time.Now(),
		IP:         "192.0.2.1",
		Path:       "/api/v1/login",
		Outcome:    "blocked_abuse",
		AbuseScore: 0.95,
		Reason:     "credential stuffing heuristic",
	}

	if err := store.Write(ctx, e1); err != nil {
		t.Fatalf("Write e1 failed: %v", err)
	}
	if err := store.Write(ctx, e2); err != nil {
		t.Fatalf("Write e2 failed: %v", err)
	}
	if err := store.Write(ctx, e3); err != nil {
		t.Fatalf("Write e3 failed: %v", err)
	}

	c1Entries, err := store.FindRecentByClientID(ctx, "client-1", 10)
	if err != nil {
		t.Fatalf("FindRecentByClientID failed: %v", err)
	}
	if len(c1Entries) != 2 {
		t.Fatalf("expected 2 entries for client-1, got %d", len(c1Entries))
	}
	// e3 was inserted last, so it should be first in reverse-chronological order
	if c1Entries[0].Path != "/api/v1/login" {
		t.Fatalf("expected most recent entry to be /api/v1/login, got %q", c1Entries[0].Path)
	}
	if c1Entries[1].Path != "/api/v1/resource" {
		t.Fatalf("expected older entry to be /api/v1/resource, got %q", c1Entries[1].Path)
	}

	c2Entries, err := store.FindRecentByClientID(ctx, "client-2", 1)
	if err != nil {
		t.Fatalf("FindRecentByClientID for client-2 failed: %v", err)
	}
	if len(c2Entries) != 1 || c2Entries[0].ClientID != "client-2" {
		t.Fatalf("unexpected entries for client-2: %+v", c2Entries)
	}
}
