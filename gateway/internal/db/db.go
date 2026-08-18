// Package db manages the Postgres connection pool and schema migrations
// shared by every Gargoyle process that needs persistence (the gateway
// itself, and operator tools like cmd/gargoylectl).
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a connection pool to Postgres and verifies connectivity
// with a bounded ping before returning. Failing fast here means a bad
// connection string surfaces as a clear startup error instead of a
// confusing failure on the first incoming request.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("db: creating connection pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: connecting to postgres: %w", err)
	}

	return pool, nil
}
