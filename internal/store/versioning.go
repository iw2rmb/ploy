package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ensureVersionTable creates the schema_version table if it doesn't exist.
// The table is also defined in schema.sql; this function allows versioning
// to work independently for migration tracking.
func ensureVersionTable(ctx context.Context, pool *pgxpool.Pool) error {
	// Use separate Exec calls to avoid multi-statement execution issues
	// with the extended protocol.
	if _, err := pool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS ploy`); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS ploy.schema_version (
        version BIGINT PRIMARY KEY,
        applied_at TIMESTAMPTZ NOT NULL
    )`)
	return err
}

// getCurrentVersion returns the highest applied migration version.
// Returns 0 if no migrations have been applied.
func getCurrentVersion(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var version int64
	err := pool.QueryRow(ctx, "SELECT COALESCE(MAX(version), 0) FROM ploy.schema_version").Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// recordMigration inserts a migration version into schema_version within a transaction.
func recordMigration(ctx context.Context, tx pgx.Tx, version int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO ploy.schema_version (version, applied_at) VALUES ($1, now())`, version)
	return err
}
