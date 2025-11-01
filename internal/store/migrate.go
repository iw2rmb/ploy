// Package store provides PostgreSQL-backed data persistence using pgx and sqlc.
package store

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

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

		// Execute migration SQL.
		if _, err := tx.Exec(ctx, m.SQL); err != nil {
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

// ensureVersionTable creates the schema_version table if it doesn't exist.
func ensureVersionTable(ctx context.Context, pool *pgxpool.Pool) error {
	sql := `
		CREATE SCHEMA IF NOT EXISTS ploy;
		CREATE TABLE IF NOT EXISTS ploy.schema_version (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`
	_, err := pool.Exec(ctx, sql)
	return err
}

// getCurrentVersion returns the highest applied migration version.
// Returns 0 if no migrations have been applied.
func getCurrentVersion(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	var version int
	err := pool.QueryRow(ctx, "SELECT COALESCE(MAX(version), 0) FROM ploy.schema_version").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("query schema_version: %w", err)
	}
	return version, nil
}

// loadMigrations reads all .sql files from the migrations directory and returns them sorted by version.
func loadMigrations() ([]Migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}

	var migrations []Migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Parse version from filename (e.g., "001_initial.sql" -> version 1).
		var version int
		var name string
		if _, err := fmt.Sscanf(entry.Name(), "%d_", &version); err != nil {
			return nil, fmt.Errorf("parse version from %s: %w", entry.Name(), err)
		}
		name = strings.TrimSuffix(strings.TrimPrefix(entry.Name(), fmt.Sprintf("%03d_", version)), ".sql")

		// Read migration SQL.
		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, Migration{
			Version: version,
			Name:    name,
			SQL:     string(content),
		})
	}

	// Sort by version.
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}
