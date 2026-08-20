package client

import "context"

type contextKey struct{}

// NewContext returns a new context carrying the resolved Client.
func NewContext(ctx context.Context, c *Client) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

// FromContext retrieves the Client stored in ctx, if any.
func FromContext(ctx context.Context) (*Client, bool) {
	c, ok := ctx.Value(contextKey{}).(*Client)
	return c, ok
}
