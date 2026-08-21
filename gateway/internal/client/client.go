// Package client manages tenant registration, API key authentication, and client caching.
package client

import (
	"net/url"
	"time"
)

// Client represents a registered tenant.
type Client struct {
	ID         string
	Name       string
	APIKeyHash string
	TargetURL  *url.URL
	RateLimit  int
	CreatedAt  time.Time
}
