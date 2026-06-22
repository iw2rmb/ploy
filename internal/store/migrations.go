package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/tern/v2/migrate"
)

//go:embed migrations/tern/*.sql
var embeddedTernMigrations embed.FS

const (
	legacyCurrentSchemaVersion int64 = 2026060203
	ternVersionTable                 = "ploy.tern_schema_version"

	// TargetSchemaVersion is the highest embedded Tern migration version.
	TargetSchemaVersion int32 = 6
)

// ErrUnsupportedSchema is returned when an existing database cannot be safely
// baselined into the Tern migration chain.
var ErrUnsupportedSchema = errors.New("store: unsupported database schema for Tern adoption")

// RunMigrations applies embedded Tern migrations. Existing databases from the
// final custom migration state are baselined at Tern version 1 so cleanup
// migration 2 can run normally on adoption.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("running migrations", "target_version", TargetSchemaVersion)

	if _, err := pool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS ploy`); err != nil {
		return fmt.Errorf("ensure ploy schema: %w", err)
	}

	acquired, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer acquired.Release()

	migrator, err := newTernMigrator(ctx, acquired.Conn())
	if err != nil {
		return err
	}

	currentVersion, err := migrator.GetCurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("get current Tern version: %w", err)
	}

	if currentVersion == 0 {
		state, err := inspectTernAdoptionState(ctx, acquired.Conn())
		if err != nil {
			return fmt.Errorf("inspect schema state: %w", err)
		}
		switch {
		case state.fresh:
			// Empty database: run the full Tern chain from version 0.
		case state.knownCurrentLegacy:
			slog.Info("baselining legacy schema for Tern", "legacy_version", state.legacyVersion, "tern_version", int32(1))
			if err := migrator.SetVersion(ctx, 1); err != nil {
				return fmt.Errorf("baseline legacy schema: %w", err)
			}
		default:
			return fmt.Errorf("%w: expected empty database or ploy.schema_version >= %d",
				ErrUnsupportedSchema, legacyCurrentSchemaVersion)
		}
	}

	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("run Tern migrations: %w", err)
	}

	version, err := migrator.GetCurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("get migrated Tern version: %w", err)
	}
	slog.Info("schema migrations complete", "version", version)
	return nil
}

// CurrentSchemaVersion returns the applied Tern schema version.
func CurrentSchemaVersion(ctx context.Context, pool *pgxpool.Pool) (int32, error) {
	var version int32
	err := pool.QueryRow(ctx, `SELECT version FROM ploy.tern_schema_version`).Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

func newTernMigrator(ctx context.Context, conn *pgx.Conn) (*migrate.Migrator, error) {
	migrationsFS, err := fs.Sub(embeddedTernMigrations, "migrations/tern")
	if err != nil {
		return nil, fmt.Errorf("load embedded migrations: %w", err)
	}

	migrator, err := migrate.NewMigrator(ctx, conn, ternVersionTable)
	if err != nil {
		return nil, fmt.Errorf("create Tern migrator: %w", err)
	}
	if err := migrator.LoadMigrations(migrationsFS); err != nil {
		return nil, fmt.Errorf("load Tern migrations: %w", err)
	}
	migrator.OnStart = func(sequence int32, name, direction, _ string) {
		slog.Info("running schema migration", "version", sequence, "name", name, "direction", direction)
	}
	return migrator, nil
}

type ternAdoptionState struct {
	fresh              bool
	knownCurrentLegacy bool
	legacyVersion      int64
}

func inspectTernAdoptionState(ctx context.Context, conn *pgx.Conn) (ternAdoptionState, error) {
	objectCount, err := countPloyObjectsExcludingTern(ctx, conn)
	if err != nil {
		return ternAdoptionState{}, err
	}
	if objectCount == 0 {
		return ternAdoptionState{fresh: true}, nil
	}

	legacyVersion, err := currentLegacyVersion(ctx, conn)
	if err != nil {
		return ternAdoptionState{}, err
	}

	return ternAdoptionState{
		knownCurrentLegacy: legacyVersion >= legacyCurrentSchemaVersion,
		legacyVersion:      legacyVersion,
	}, nil
}

func countPloyObjectsExcludingTern(ctx context.Context, conn *pgx.Conn) (int, error) {
	var count int
	err := conn.QueryRow(ctx, `
SELECT (
  SELECT count(*)
  FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = 'ploy'
    AND c.relkind IN ('r', 'p', 'v', 'm', 'S')
    AND c.relname <> 'tern_schema_version'
) + (
  SELECT count(*)
  FROM pg_type t
  JOIN pg_namespace n ON n.oid = t.typnamespace
  WHERE n.nspname = 'ploy'
    AND t.typtype = 'e'
)
`).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func currentLegacyVersion(ctx context.Context, conn *pgx.Conn) (int64, error) {
	var hasLegacyVersionTable bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass('ploy.schema_version') IS NOT NULL`).Scan(&hasLegacyVersionTable); err != nil {
		return 0, err
	}
	if !hasLegacyVersionTable {
		return 0, nil
	}

	var version int64
	if err := conn.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM ploy.schema_version`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}
