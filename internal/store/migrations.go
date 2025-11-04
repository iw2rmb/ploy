package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration represents a single database migration script.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// RunMigrations ensures the database schema is present and up-to-date.
// It creates a schema_version table if missing, then applies all pending migrations.
// Each migration is executed in a transaction and logged. The current version is logged on success.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// Create schema_version table if it doesn't exist.
	if err := ensureVersionTable(ctx, pool); err != nil {
		return fmt.Errorf("ensure version table: %w", err)
	}

	// Load all migration files.
	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	// Get current schema version.
	currentVersion, err := getCurrentVersion(ctx, pool)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	// Apply pending migrations.
	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}

		slog.Info("applying migration", "version", m.Version, "name", m.Name)

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin transaction for migration %d: %w", m.Version, err)
		}

		// Execute migration SQL (supports multi-statement files).
		if err := execMigrationSQL(ctx, tx, m.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("execute migration %d (%s): %w", m.Version, m.Name, err)
		}

		// Update schema_version.
		if _, err := tx.Exec(ctx, "INSERT INTO ploy.schema_version (version, name) VALUES ($1, $2)", m.Version, m.Name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
		}

		slog.Info("migration applied", "version", m.Version, "name", m.Name)
		currentVersion = m.Version
	}

	slog.Info("schema up-to-date", "version", currentVersion)
	return nil
}
