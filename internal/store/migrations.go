package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration represents a single database migration script.
// Kept for backwards compatibility but no longer used.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// RunMigrations ensures the database schema is present.
// Since there are no production deployments, this simply executes internal/store/schema.sql
// to create all tables from scratch. No incremental migrations needed.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("applying schema from internal/store/schema.sql")

	// First check if the core schema already exists. When the runs table is
	// present in the ploy schema, we treat migrations as already applied and
	// skip executing the embedded schema.sql. This makes RunMigrations safe
	// to call on every server startup without failing on CREATE TYPE/CREATE
	// TABLE statements that are not idempotent.
	const existsSQL = `
SELECT EXISTS (
  SELECT 1
  FROM information_schema.tables
  WHERE table_schema = 'ploy' AND table_name = 'runs'
)`

	var exists bool
	if err := pool.QueryRow(ctx, existsSQL).Scan(&exists); err != nil {
		return fmt.Errorf("check existing schema: %w", err)
	}
	if exists {
		slog.Info("schema already present, skipping schema.sql execution")
		return nil
	}

	// Execute schema SQL (supports multi-statement files).
	schemaSQL := getSchemaSQL()
	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		return fmt.Errorf("execute schema: %w", err)
	}

	slog.Info("schema applied successfully")
	return nil
}
