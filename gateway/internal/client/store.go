package client

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Store and Registry implementations when no
// client matches the given lookup key.
var ErrNotFound = errors.New("client: not found")

// Store is the persistence interface for client records. Production code
// uses PostgresStore; tests can supply an in-memory fake without needing a
// real database.
type Store interface {
	FindByAPIKeyHash(ctx context.Context, hash string) (*Client, error)
}
