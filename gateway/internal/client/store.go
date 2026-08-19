package client

import (
	"context"
	"errors"
)

// ErrNotFound is returned when no client matches the query.
var ErrNotFound = errors.New("client: not found")

// Store defines persistence operations for client records.
type Store interface {
	FindByAPIKeyHash(ctx context.Context, hash string) (*Client, error)
	CountClients(ctx context.Context) (int, error)
}
