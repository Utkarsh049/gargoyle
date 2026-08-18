// Package client implements Gargoyle's client registry: resolving an
// incoming API key to the registered tenant it belongs to (name, target
// backend, rate limit, plan tier), backed by Postgres and fronted by a
// short-TTL in-memory cache so the hot request path doesn't hit the
// database on every request (see PROJECT.md §2 and §8).
package client

import (
	"net/url"
	"time"
)

// Client is a registered Gargoyle tenant, as stored in the `clients`
// Postgres table (see internal/db/migrations/0001_create_clients.sql).
type Client struct {
	ID         string
	Name       string
	APIKeyHash string
	TargetURL  *url.URL
	RateLimit  int
	PlanTier   string
	CreatedAt  time.Time
}
