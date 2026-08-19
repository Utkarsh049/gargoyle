package logstore

import (
	"context"
	"fmt"
	"time"

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

const insertLogQuery = `
	INSERT INTO request_logs (client_id, timestamp, ip, path, outcome, abuse_score, reason)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
`

// Write writes an Entry into request_logs.
func (s *PostgresStore) Write(ctx context.Context, entry Entry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	_, err := s.pool.Exec(ctx, insertLogQuery,
		entry.ClientID,
		entry.Timestamp,
		entry.IP,
		entry.Path,
		entry.Outcome,
		entry.AbuseScore,
		entry.Reason,
	)
	if err != nil {
		return fmt.Errorf("logstore: inserting request log: %w", err)
	}
	return nil
}

const findRecentQuery = `
	SELECT id::text, client_id::text, timestamp, ip, path, outcome, abuse_score, reason
	FROM request_logs
	WHERE client_id = $1
	ORDER BY timestamp DESC
	LIMIT $2
`

// FindRecentByClientID queries recent decision logs for a tenant, ordered by timestamp descending.
func (s *PostgresStore) FindRecentByClientID(ctx context.Context, clientID string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.pool.Query(ctx, findRecentQuery, clientID, limit)
	if err != nil {
		return nil, fmt.Errorf("logstore: querying recent logs for client %s: %w", clientID, err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.ClientID, &e.Timestamp, &e.IP, &e.Path, &e.Outcome, &e.AbuseScore, &e.Reason); err != nil {
			return nil, fmt.Errorf("logstore: scanning log entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("logstore: iterating log entries: %w", err)
	}

	return entries, nil
}
