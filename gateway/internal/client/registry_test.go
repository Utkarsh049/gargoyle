package client

import (
	"context"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

// fakeStore is an in-memory Store used only for tests, keyed by
// api_key_hash the same way PostgresStore is.
type fakeStore struct {
	byHash map[string]*Client
	calls  atomic.Int64
}

func (f *fakeStore) FindByAPIKeyHash(_ context.Context, hash string) (*Client, error) {
	f.calls.Add(1)
	c, ok := f.byHash[hash]
	if !ok {
		return nil, ErrNotFound
	}
	return c, nil
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing test URL %q: %v", raw, err)
	}
	return u
}

func TestRegistryLookupCachesHits(t *testing.T) {
	const apiKey = "gk_live_test-key"
	client := &Client{ID: "1", Name: "acme", TargetURL: mustURL(t, "http://localhost:9001")}
	store := &fakeStore{byHash: map[string]*Client{HashAPIKey(apiKey): client}}
	registry := NewRegistry(store, time.Minute)

	for i := 0; i < 5; i++ {
		got, err := registry.Lookup(context.Background(), apiKey)
		if err != nil {
			t.Fatalf("Lookup: %v", err)
		}
		if got != client {
			t.Fatalf("Lookup returned a different *Client than the store holds")
		}
	}

	if calls := store.calls.Load(); calls != 1 {
		t.Fatalf("expected exactly 1 store call after caching, got %d", calls)
	}
}

func TestRegistryLookupNotFoundIsNeverCached(t *testing.T) {
	store := &fakeStore{byHash: map[string]*Client{}}
	registry := NewRegistry(store, time.Minute)

	for i := 0; i < 3; i++ {
		_, err := registry.Lookup(context.Background(), "gk_live_does-not-exist")
		if err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	}

	if calls := store.calls.Load(); calls != 3 {
		t.Fatalf("expected every miss to hit the store (3 calls), got %d", calls)
	}
}

func TestRegistryLookupExpiresAfterTTL(t *testing.T) {
	const apiKey = "gk_live_test-key"
	client := &Client{ID: "1", Name: "acme", TargetURL: mustURL(t, "http://localhost:9001")}
	store := &fakeStore{byHash: map[string]*Client{HashAPIKey(apiKey): client}}
	registry := NewRegistry(store, 10*time.Millisecond)

	if _, err := registry.Lookup(context.Background(), apiKey); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if _, err := registry.Lookup(context.Background(), apiKey); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if calls := store.calls.Load(); calls != 1 {
		t.Fatalf("expected 1 store call before TTL expiry, got %d", calls)
	}

	time.Sleep(20 * time.Millisecond)

	if _, err := registry.Lookup(context.Background(), apiKey); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if calls := store.calls.Load(); calls != 2 {
		t.Fatalf("expected a second store call after TTL expiry, got %d", calls)
	}
}

func TestRegistryLookupDistinguishesClientsByKey(t *testing.T) {
	clientA := &Client{ID: "a", Name: "acme", TargetURL: mustURL(t, "http://localhost:9001")}
	clientB := &Client{ID: "b", Name: "globex", TargetURL: mustURL(t, "http://localhost:9002")}
	store := &fakeStore{byHash: map[string]*Client{
		HashAPIKey("gk_live_a"): clientA,
		HashAPIKey("gk_live_b"): clientB,
	}}
	registry := NewRegistry(store, time.Minute)

	gotA, err := registry.Lookup(context.Background(), "gk_live_a")
	if err != nil {
		t.Fatalf("Lookup a: %v", err)
	}
	gotB, err := registry.Lookup(context.Background(), "gk_live_b")
	if err != nil {
		t.Fatalf("Lookup b: %v", err)
	}

	if gotA.ID != "a" || gotB.ID != "b" {
		t.Fatalf("expected distinct clients, got %q and %q", gotA.ID, gotB.ID)
	}
	if gotA.TargetURL.String() == gotB.TargetURL.String() {
		t.Fatalf("expected distinct target URLs, got the same one for both clients")
	}
}
