package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SchemaVersion is the version number for the embedded schema.sql.
// Increment this when schema.sql changes to trigger re-application on existing databases.
// This uses a timestamp-like versioning scheme (YYYYMMDDNN) for clarity.
const SchemaVersion int64 = 2026032601

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

	// Check current version.
	currentVersion, err := getCurrentVersion(ctx, pool)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	if currentVersion >= SchemaVersion {
		slog.Info("schema already at target version", "current", currentVersion, "target", SchemaVersion)
		return nil
	}

	slog.Info("applying schema", "from_version", currentVersion, "to_version", SchemaVersion)

	// Execute schema in a transaction using execMigrationSQL for proper
	// statement-by-statement execution.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	schemaSQL := getSchemaSQL()
	if err := execMigrationSQL(ctx, tx, schemaSQL); err != nil {
		return fmt.Errorf("execute schema: %w", err)
	}

	// Record the migration version.
	if err := recordMigration(ctx, tx, SchemaVersion); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}

	slog.Info("schema applied successfully", "version", SchemaVersion)
	return nil
}
