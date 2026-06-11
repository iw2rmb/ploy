package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRunMigrationsTernStates(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping migration test")
	}
	ctx := context.Background()

	tests := []struct {
		name    string
		setup   func(t *testing.T, ctx context.Context, pool *pgxpool.Pool)
		wantErr error
		assert  func(t *testing.T, ctx context.Context, pool *pgxpool.Pool)
	}{
		{
			name:  "fresh empty database applies full Tern chain and is idempotent",
			setup: resetPloySchema,
			assert: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
				assertTernVersion(t, ctx, pool, TargetSchemaVersion)
				assertTableAbsent(t, ctx, pool, "schema_version")

				if err := RunMigrations(ctx, pool); err != nil {
					t.Fatalf("RunMigrations second run: %v", err)
				}
				assertTernVersion(t, ctx, pool, TargetSchemaVersion)
			},
		},
		{
			name:  "known current legacy database is baselined then cleaned up",
			setup: setupKnownCurrentLegacySchema,
			assert: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
				assertTernVersion(t, ctx, pool, TargetSchemaVersion)
				assertTableAbsent(t, ctx, pool, "schema_version")
				assertColumnAbsent(t, ctx, pool, "api_tokens", "cluster_id")
				assertColumnAbsent(t, ctx, pool, "bootstrap_tokens", "cluster_id")
				assertColumnAbsent(t, ctx, pool, "mig_repos", "target_ref")
				assertNoObsoleteNodeUpdaterRows(t, ctx, pool)
			},
		},
		{
			name:    "unsupported existing database fails clearly",
			setup:   setupUnsupportedExistingSchema,
			wantErr: ErrUnsupportedSchema,
			assert: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
				assertTernVersion(t, ctx, pool, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newIsolatedMigrationTestStore(t, ctx, dsn)

			tt.setup(t, ctx, st.Pool())
			err := RunMigrations(ctx, st.Pool())
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("RunMigrations error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("RunMigrations: %v", err)
			}
			tt.assert(t, ctx, st.Pool())
		})
	}
}

func newIsolatedMigrationTestStore(t *testing.T, ctx context.Context, baseDSN string) Store {
	t.Helper()

	adminPool, err := pgxpool.New(ctx, baseDSN)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}

	dbName := fmt.Sprintf("ploy_migration_%d_%d", os.Getpid(), time.Now().UnixNano())
	dbIdent := pgx.Identifier{dbName}.Sanitize()
	if _, err := adminPool.Exec(ctx, `CREATE DATABASE `+dbIdent); err != nil {
		adminPool.Close()
		t.Skipf("create isolated migration database: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(baseDSN)
	if err != nil {
		adminPool.Close()
		t.Fatalf("parse dsn: %v", err)
	}
	cfg.ConnConfig.Database = dbName
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = make(map[string]string)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = "ploy, public"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		_, _ = adminPool.Exec(ctx, `DROP DATABASE IF EXISTS `+dbIdent+` WITH (FORCE)`)
		adminPool.Close()
		t.Fatalf("connect isolated database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		_, _ = adminPool.Exec(ctx, `DROP DATABASE IF EXISTS `+dbIdent+` WITH (FORCE)`)
		adminPool.Close()
		t.Fatalf("ping isolated database: %v", err)
	}
	st := &PgStore{pool: pool, Queries: New(pool)}

	t.Cleanup(func() {
		st.Close()
		if _, err := adminPool.Exec(ctx, `DROP DATABASE IF EXISTS `+dbIdent+` WITH (FORCE)`); err != nil {
			t.Fatalf("drop isolated migration database %s: %v", dbName, err)
		}
		adminPool.Close()
	})

	return st
}

func resetPloySchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS ploy CASCADE`); err != nil {
		t.Fatalf("drop ploy schema: %v", err)
	}
}

func setupKnownCurrentLegacySchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	resetPloySchema(t, ctx, pool)
	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatalf("create current schema fixture: %v", err)
	}

	nodeID := types.NodeID(types.NewNodeKey())
	if _, err := pool.Exec(ctx, `
INSERT INTO ploy.nodes (id, name, ip_address, concurrency)
VALUES ($1, $2, $3, 1)
`, nodeID.String(), nodeNameForTest(nodeID), nodeAddrForTest(nodeID)); err != nil {
		t.Fatalf("insert node fixture: %v", err)
	}

	setupStatements := []struct {
		name string
		sql  string
		args []any
	}{
		{name: "remove tern state", sql: `DROP TABLE ploy.tern_schema_version`},
		{name: "create legacy version table", sql: `CREATE TABLE ploy.schema_version (version BIGINT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL)`},
		{name: "insert legacy current version", sql: `INSERT INTO ploy.schema_version (version, applied_at) VALUES ($1, now())`, args: []any{legacyCurrentSchemaVersion}},
		{name: "restore api token cluster column", sql: `ALTER TABLE ploy.api_tokens ADD COLUMN cluster_id TEXT`},
		{name: "restore bootstrap token cluster column", sql: `ALTER TABLE ploy.bootstrap_tokens ADD COLUMN cluster_id TEXT`},
		{name: "restore target ref column", sql: `ALTER TABLE ploy.mig_repos ADD COLUMN target_ref TEXT`},
		{name: "drop diagnostics constraint", sql: `ALTER TABLE ploy.node_diagnostics DROP CONSTRAINT IF EXISTS node_diagnostics_component_check`},
		{name: "allow old diagnostics component", sql: `ALTER TABLE ploy.node_diagnostics ADD CONSTRAINT node_diagnostics_component_check CHECK (component IN ('node', 'node-updater'))`},
		{name: "drop logs constraint", sql: `ALTER TABLE ploy.node_daemon_logs DROP CONSTRAINT IF EXISTS node_daemon_logs_component_check`},
		{name: "allow old logs component", sql: `ALTER TABLE ploy.node_daemon_logs ADD CONSTRAINT node_daemon_logs_component_check CHECK (component IN ('node', 'node-updater'))`},
		{name: "insert obsolete diagnostic", sql: `INSERT INTO ploy.node_diagnostics (node_id, component, status, details) VALUES ($1, 'node-updater', 'ok', '{}'::jsonb)`, args: []any{nodeID.String()}},
		{name: "insert obsolete daemon log", sql: `INSERT INTO ploy.node_daemon_logs (node_id, component, stream, message) VALUES ($1, 'node-updater', 'system', 'old updater row')`, args: []any{nodeID.String()}},
	}
	for _, stmt := range setupStatements {
		if _, err := pool.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatalf("%s: %v", stmt.name, err)
		}
	}
}

func setupUnsupportedExistingSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	resetPloySchema(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
CREATE SCHEMA ploy;
CREATE TABLE ploy.nodes (id TEXT PRIMARY KEY);
`); err != nil {
		t.Fatalf("create unsupported schema fixture: %v", err)
	}
}

func assertTernVersion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int32) {
	t.Helper()
	got, err := CurrentSchemaVersion(ctx, pool)
	if err != nil {
		t.Fatalf("CurrentSchemaVersion: %v", err)
	}
	if got != want {
		t.Fatalf("CurrentSchemaVersion = %d, want %d", got, want)
	}
}

func assertTableAbsent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('ploy.' || $1) IS NOT NULL`, table).Scan(&exists); err != nil {
		t.Fatalf("check table %s: %v", table, err)
	}
	if exists {
		t.Fatalf("table ploy.%s exists, want absent", table)
	}
}

func assertColumnAbsent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, column string) {
	t.Helper()
	var exists bool
	err := pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM information_schema.columns
  WHERE table_schema = 'ploy' AND table_name = $1 AND column_name = $2
)
`, table, column).Scan(&exists)
	if err != nil {
		t.Fatalf("check column %s.%s: %v", table, column, err)
	}
	if exists {
		t.Fatalf("column ploy.%s.%s exists, want absent", table, column)
	}
}

func assertNoObsoleteNodeUpdaterRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var diagCount, logCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ploy.node_diagnostics WHERE component = 'node-updater'`).Scan(&diagCount); err != nil {
		t.Fatalf("count obsolete diagnostics: %v", err)
	}
	if diagCount != 0 {
		t.Fatalf("obsolete diagnostics count = %d, want 0", diagCount)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ploy.node_daemon_logs WHERE component = 'node-updater'`).Scan(&logCount); err != nil {
		t.Fatalf("count obsolete daemon logs: %v", err)
	}
	if logCount != 0 {
		t.Fatalf("obsolete daemon logs count = %d, want 0", logCount)
	}

	var nodeID string
	if err := pool.QueryRow(ctx, `SELECT id FROM ploy.nodes LIMIT 1`).Scan(&nodeID); err != nil {
		t.Fatalf("select node fixture: %v", err)
	}
	_, err := pool.Exec(ctx, `
INSERT INTO ploy.node_diagnostics (node_id, component, status, details)
VALUES ($1, 'node-updater', 'ok', '{}'::jsonb)
`, nodeID)
	if err == nil {
		t.Fatal("expected node_diagnostics node-updater insert to fail")
	}
}
