package client

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is the production Store implementation, backed by the
// `clients` table.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore builds a PostgresStore over an existing connection pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

const clientColumns = `id::text, name, api_key_hash, target_url, rate_limit, plan_tier, created_at`

const findByAPIKeyHashQuery = `SELECT ` + clientColumns + ` FROM clients WHERE api_key_hash = $1`

// FindByAPIKeyHash looks up the client whose api_key_hash matches hash. It
// returns ErrNotFound if no client matches.
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

// NewClientParams holds the fields needed to register a new client.
// APIKeyHash must already be hashed (see HashAPIKey) — PostgresStore never
// sees or stores a plaintext key.
type NewClientParams struct {
	Name       string
	APIKeyHash string
	TargetURL  string
	RateLimit  int
	PlanTier   string
}

const insertClientQuery = `
	INSERT INTO clients (name, api_key_hash, target_url, rate_limit, plan_tier)
	VALUES ($1, $2, $3, $4, $5)
	RETURNING ` + clientColumns

// CreateClient inserts a new client record. It's used by cmd/gargoylectl
// today, ahead of the HTTP admin API that lands in Phase 9.
func (s *PostgresStore) CreateClient(ctx context.Context, params NewClientParams) (*Client, error) {
	row := s.pool.QueryRow(ctx, insertClientQuery,
		params.Name, params.APIKeyHash, params.TargetURL, params.RateLimit, params.PlanTier,
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
	if err := row.Scan(&c.ID, &c.Name, &c.APIKeyHash, &targetRaw, &c.RateLimit, &c.PlanTier, &c.CreatedAt); err != nil {
		return nil, err
	}

	targetURL, err := url.Parse(targetRaw)
	if err != nil {
		return nil, fmt.Errorf("parsing stored target_url %q for client %s: %w", targetRaw, c.ID, err)
	}
	c.TargetURL = targetURL

	return &c, nil
}
