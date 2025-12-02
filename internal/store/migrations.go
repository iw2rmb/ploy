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

	// Execute schema SQL (supports multi-statement files).
	schemaSQL := getSchemaSQL()
	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		return fmt.Errorf("execute schema: %w", err)
	}

	slog.Info("schema applied successfully")
	return nil
}
