package client

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements Store backed by PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a PostgresStore over a connection pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

const clientColumns = `id::text, name, api_key_hash, target_url, rate_limit, created_at`

const findByAPIKeyHashQuery = `SELECT ` + clientColumns + ` FROM clients WHERE api_key_hash = $1`

// FindByAPIKeyHash looks up a client by the SHA-256 hash of their API key.
func (s *PostgresStore) FindByAPIKeyHash(ctx context.Context, hash string) (*Client, error) {
	row := s.pool.QueryRow(ctx, findByAPIKeyHashQuery, hash)

	c, err := scanClient(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("client: querying by api key hash: %w", err)
	}
	return c, nil
}

// NewClientParams holds the attributes required to create a new client.
type NewClientParams struct {
	Name       string
	APIKeyHash string
	TargetURL  string
	RateLimit  int
}

const insertClientQuery = `
	INSERT INTO clients (name, api_key_hash, target_url, rate_limit)
	VALUES ($1, $2, $3, $4)
	RETURNING ` + clientColumns

// CreateClient inserts a new client record.
func (s *PostgresStore) CreateClient(ctx context.Context, params NewClientParams) (*Client, error) {
	row := s.pool.QueryRow(ctx, insertClientQuery,
		params.Name, params.APIKeyHash, params.TargetURL, params.RateLimit,
	)

	c, err := scanClient(row)
	if err != nil {
		return nil, fmt.Errorf("client: creating client: %w", err)
	}
	return c, nil
}

// CountClients returns the total number of registered clients in Postgres.
func (s *PostgresStore) CountClients(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM clients`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("client: counting clients: %w", err)
	}
	return count, nil
}

// scanClient scans a row shaped like clientColumns into a Client,
// including parsing the stored target_url into a *url.URL.
func scanClient(row pgx.Row) (*Client, error) {
	var (
		c         Client
		targetRaw string
	)
	if err := row.Scan(&c.ID, &c.Name, &c.APIKeyHash, &targetRaw, &c.RateLimit, &c.CreatedAt); err != nil {
		return nil, err
	}

	targetURL, err := url.Parse(targetRaw)
	if err != nil {
		return nil, fmt.Errorf("parsing stored target_url %q for client %s: %w", targetRaw, c.ID, err)
	}
	c.TargetURL = targetURL

	return &c, nil
}
