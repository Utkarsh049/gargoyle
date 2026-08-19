// Package logstore provides decision audit logging to PostgreSQL.
package logstore

import (
	"context"
	"time"
)

// Entry represents a single decision audit log record.
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
