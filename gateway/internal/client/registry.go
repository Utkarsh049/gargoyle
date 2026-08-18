package client

import (
	"context"
	"sync"
	"time"
)

// Registry resolves a raw API key (as received on the wire) to a Client,
// caching positive lookups in memory for ttl so the hot request path
// doesn't hit Postgres on every single request (see PROJECT.md §8).
type Registry struct {
	store Store
	ttl   time.Duration

	mu    sync.RWMutex
	cache map[string]cachedEntry
}

type cachedEntry struct {
	client    *Client
	expiresAt time.Time
}

// NewRegistry builds a Registry over store, caching hits for ttl.
func NewRegistry(store Store, ttl time.Duration) *Registry {
	return &Registry{
		store: store,
		ttl:   ttl,
		cache: make(map[string]cachedEntry),
	}
}

// Lookup resolves apiKey to its Client, returning ErrNotFound if the key
// doesn't match any registered client.
//
// Only successful lookups are cached. An unbounded stream of invalid keys
// (typos, credential-stuffing attempts) would otherwise grow the cache
// without bound since every distinct bad key is a new map entry; sending
// every miss to the store keeps this cache's memory bounded by the number
// of real clients. Throttling repeated invalid attempts is Phase 3's job
// (rate limiting), not this cache's.
func (r *Registry) Lookup(ctx context.Context, apiKey string) (*Client, error) {
	hash := HashAPIKey(apiKey)

	if c, ok := r.fromCache(hash); ok {
		return c, nil
	}

	c, err := r.store.FindByAPIKeyHash(ctx, hash)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[hash] = cachedEntry{client: c, expiresAt: time.Now().Add(r.ttl)}
	r.mu.Unlock()

	return c, nil
}

func (r *Registry) fromCache(hash string) (*Client, bool) {
	r.mu.RLock()
	entry, ok := r.cache[hash]
	r.mu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.client, true
}
