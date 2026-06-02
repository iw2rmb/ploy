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

//go:embed migrations/wave_model_20260601.sql
var waveModelMigrationSQL string

// SchemaVersion is the version number for the embedded schema.sql.
// Increment this when schema.sql changes to trigger current-contract maintenance
// on existing databases. This uses a timestamp-like versioning scheme
// (YYYYMMDDNN) for clarity.
const SchemaVersion int64 = 2026060203

const waveModelSchemaVersion int64 = 2026060101

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

	if currentVersion >= waveModelSchemaVersion {
		if err := applyCurrentSchemaMaintenance(ctx, pool, currentVersion); err != nil {
			return err
		}
		return nil
	}

	needsWaveMigration, err := needsWaveModelMigration(ctx, pool)
	if err != nil {
		return fmt.Errorf("inspect schema state: %w", err)
	}

	slog.Info("applying schema", "from_version", currentVersion, "to_version", SchemaVersion)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if needsWaveMigration {
		slog.Info("applying wave model data migration")
		if err := execMigrationSQL(ctx, tx, waveModelMigrationSQL); err != nil {
			return fmt.Errorf("execute wave model migration: %w", err)
		}
	} else if err := execMigrationSQL(ctx, tx, schemaSQL); err != nil {
		return fmt.Errorf("execute schema: %w", err)
	}

	if err := applyCurrentSchemaMaintenanceSQL(ctx, tx); err != nil {
		return err
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

func applyCurrentSchemaMaintenance(ctx context.Context, pool *pgxpool.Pool, currentVersion int64) error {
	slog.Info("applying current schema maintenance", "from_version", currentVersion, "to_version", SchemaVersion)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin maintenance transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := applyCurrentSchemaMaintenanceSQL(ctx, tx); err != nil {
		return err
	}
	if err := recordMigration(ctx, tx, SchemaVersion); err != nil {
		return fmt.Errorf("record maintenance: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit maintenance: %w", err)
	}

	slog.Info("current schema maintenance applied", "version", SchemaVersion)
	return nil
}

func applyCurrentSchemaMaintenanceSQL(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `ALTER TABLE IF EXISTS ploy.mig_repos DROP COLUMN IF EXISTS target_ref`); err != nil {
		return fmt.Errorf("drop mig_repos.target_ref: %w", err)
	}
	if _, err := tx.Exec(ctx, `
ALTER TABLE IF EXISTS ploy.runs
  ALTER COLUMN source_commit_sha SET DEFAULT '',
  ALTER COLUMN repo_sha0 SET DEFAULT '',
  ALTER COLUMN status SET DEFAULT 'Queued',
  ALTER COLUMN attempt SET DEFAULT 1,
  ALTER COLUMN stats SET DEFAULT '{}'::jsonb
`); err != nil {
		return fmt.Errorf("align runs defaults: %w", err)
	}
	if _, err := tx.Exec(ctx, `
DO $$
BEGIN
  IF to_regclass('ploy.node_daemon_logs') IS NOT NULL THEN
    DELETE FROM ploy.node_daemon_logs
    WHERE component <> 'node';
  END IF;
END $$;
`); err != nil {
		return fmt.Errorf("delete obsolete node daemon logs: %w", err)
	}
	if _, err := tx.Exec(ctx, `
DO $$
BEGIN
  IF to_regclass('ploy.node_diagnostics') IS NOT NULL THEN
    DELETE FROM ploy.node_diagnostics
    WHERE component <> 'node';
  END IF;
END $$;
`); err != nil {
		return fmt.Errorf("delete obsolete node diagnostics: %w", err)
	}
	if _, err := tx.Exec(ctx, `
ALTER TABLE IF EXISTS ploy.node_diagnostics
  DROP CONSTRAINT IF EXISTS node_diagnostics_component_check,
  ADD CONSTRAINT node_diagnostics_component_check CHECK (component IN ('node'))
`); err != nil {
		return fmt.Errorf("align node diagnostics component constraint: %w", err)
	}
	if _, err := tx.Exec(ctx, `
ALTER TABLE IF EXISTS ploy.node_daemon_logs
  DROP CONSTRAINT IF EXISTS node_daemon_logs_component_check,
  ADD CONSTRAINT node_daemon_logs_component_check CHECK (component IN ('node'))
`); err != nil {
		return fmt.Errorf("align node daemon logs component constraint: %w", err)
	}
	return nil
}

func needsWaveModelMigration(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var state struct {
		hasRunRepos       bool
		hasWaves          bool
		hasRunRepoStatus  bool
		hasWaveStatus     bool
		hasRunsWaveID     bool
		hasRunRepoActions bool
	}
	err := pool.QueryRow(ctx, `
SELECT
  to_regclass('ploy.run_repos') IS NOT NULL AS has_run_repos,
  to_regclass('ploy.waves') IS NOT NULL AS has_waves,
  EXISTS (
    SELECT 1
    FROM pg_type t
    JOIN pg_namespace n ON n.oid = t.typnamespace
    WHERE n.nspname = 'ploy' AND t.typname = 'run_repo_status'
  ) AS has_run_repo_status,
  EXISTS (
    SELECT 1
    FROM pg_type t
    JOIN pg_namespace n ON n.oid = t.typnamespace
    WHERE n.nspname = 'ploy' AND t.typname = 'wave_status'
  ) AS has_wave_status,
  EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'ploy' AND table_name = 'runs' AND column_name = 'wave_id'
  ) AS has_runs_wave_id,
  to_regclass('ploy.run_repo_actions') IS NOT NULL AS has_run_repo_actions
`).Scan(
		&state.hasRunRepos,
		&state.hasWaves,
		&state.hasRunRepoStatus,
		&state.hasWaveStatus,
		&state.hasRunsWaveID,
		&state.hasRunRepoActions,
	)
	if err != nil {
		return false, err
	}

	if !state.hasRunRepos {
		return false, nil
	}
	if !state.hasWaves && state.hasRunRepoStatus && !state.hasWaveStatus && !state.hasRunsWaveID && state.hasRunRepoActions {
		return true, nil
	}
	return false, fmt.Errorf("old run_repos table exists but schema is not the expected pre-wave shape: waves=%t run_repo_status=%t wave_status=%t runs.wave_id=%t run_repo_actions=%t",
		state.hasWaves, state.hasRunRepoStatus, state.hasWaveStatus, state.hasRunsWaveID, state.hasRunRepoActions)
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

// CurrentSchemaVersion returns the highest schema version applied to the database.
func CurrentSchemaVersion(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	return getCurrentVersion(ctx, pool)
}

// recordMigration inserts a migration version into schema_version within a transaction.
func recordMigration(ctx context.Context, tx pgx.Tx, version int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO ploy.schema_version (version, applied_at) VALUES ($1, now())`, version)
	return err
}
