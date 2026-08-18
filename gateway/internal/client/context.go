package client

import "context"

// contextKey is an unexported type so keys from this package can never
// collide with context values set by other packages.
type contextKey struct{}

// NewContext returns a copy of ctx carrying c, so downstream handlers (the
// reverse proxy, future rate limiter/abuse checks, logging) can retrieve
// the resolved client without re-parsing the request.
func NewContext(ctx context.Context, c *Client) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

// FromContext retrieves the Client stored by NewContext, if any.
func FromContext(ctx context.Context) (*Client, bool) {
	c, ok := ctx.Value(contextKey{}).(*Client)
	return c, ok
}
