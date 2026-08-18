package db

import (
	"context"
	"embed"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationsFS embeds every .sql file so the binary is self-contained —
// there's no separate migrations directory to ship or mount alongside it.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

const createSchemaMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

// Migrate applies every embedded migration that hasn't already been
// recorded in schema_migrations, in filename order (hence the numeric
// prefixes like 0001_). Each migration's DDL and its bookkeeping insert
// run in a single transaction, so a failed migration is never left
// half-applied and unrecorded.
//
// This is deliberately a minimal, dependency-free migration runner rather
// than a full tool like golang-migrate — Gargoyle's schema needs are small
// and this keeps startup self-contained with no extra binary or CLI step.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, createSchemaMigrationsTableSQL); err != nil {
		return fmt.Errorf("db: creating schema_migrations table: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("db: reading embedded migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		version := entry.Name()

		var applied bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
		).Scan(&applied)
		if err != nil {
			return fmt.Errorf("db: checking migration status for %s: %w", version, err)
		}
		if applied {
			continue
		}

		contents, err := migrationsFS.ReadFile("migrations/" + version)
		if err != nil {
			return fmt.Errorf("db: reading migration %s: %w", version, err)
		}

		if err := applyMigration(ctx, pool, version, string(contents)); err != nil {
			return err
		}
	}

	return nil
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool, version, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: beginning transaction for migration %s: %w", version, err)
	}
	defer tx.Rollback(ctx) // no-op once Commit has succeeded

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("db: applying migration %s: %w", version, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return fmt.Errorf("db: recording migration %s: %w", version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: committing migration %s: %w", version, err)
	}

	return nil
}
