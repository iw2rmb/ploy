package store

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// SchemaVersion is the version number for the embedded schema.sql.
// Increment this when schema.sql changes to trigger re-application on existing databases.
// This uses a timestamp-like versioning scheme (YYYYMMDDNN) for clarity.
const SchemaVersion int64 = 2026042001

// RunMigrations ensures the database schema is present and records the version.
// Uses execMigrationSQL for statement-by-statement execution within a transaction.
// Schema versions are tracked in ploy.schema_version for deterministic migrations.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("running migrations", "target_version", SchemaVersion)

	// Ensure schema_version table exists before checking version.
	// This must happen outside the main transaction so we can read version.
	if err := ensureVersionTable(ctx, pool); err != nil {
		return fmt.Errorf("ensure version table: %w", err)
	}

	currentVersion, err := getCurrentVersion(ctx, pool)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	if currentVersion >= SchemaVersion {
		slog.Info("schema already at target version", "current", currentVersion, "target", SchemaVersion)
		return nil
	}

	slog.Info("applying schema", "from_version", currentVersion, "to_version", SchemaVersion)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := execMigrationSQL(ctx, tx, schemaSQL); err != nil {
		return fmt.Errorf("execute schema: %w", err)
	}

	if err := recordMigration(ctx, tx, SchemaVersion); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}

	slog.Info("schema applied successfully", "version", SchemaVersion)
	return nil
}

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
