// Package logstore provides per-client decision audit logging to Postgres
// (see PROJECT.md §1, §4, and §9).
//
// Unlike Prometheus metrics (which are aggregate), request_logs records
// high-cardinality details (IP, path, reason, timestamp) for blocked or
// flagged requests without causing metric cardinality blowup.
package logstore

import (
	"context"
	"time"
)

// Entry represents a single decision log record for a client request.
type Entry struct {
	ID         string
	ClientID   string
	Timestamp  time.Time
	IP         string
	Path       string
	Outcome    string
	AbuseScore float64
	Reason     string
}

// Store is the persistence interface for request logs.
type Store interface {
	// Write persists a log entry to storage.
	Write(ctx context.Context, entry Entry) error

	// FindRecentByClientID retrieves the most recent log entries for a given client.
	FindRecentByClientID(ctx context.Context, clientID string, limit int) ([]Entry, error)
}
