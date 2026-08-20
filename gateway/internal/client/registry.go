package client

import (
	"context"
	"sync"
	"time"
)

// Registry resolves raw API keys to Client instances with TTL-based caching.
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

// NewRegistry constructs a Registry backed by store with an in-memory cache TTL.
func NewRegistry(store Store, ttl time.Duration) *Registry {
	return &Registry{
		store: store,
		ttl:   ttl,
		cache: make(map[string]cachedEntry),
	}
}

// Lookup resolves an API key to a Client, returning ErrNotFound on cache miss and store miss.
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

// CountClients delegates to the store to get the registered client count.
func (r *Registry) CountClients(ctx context.Context) (int, error) {
	return r.store.CountClients(ctx)
}
