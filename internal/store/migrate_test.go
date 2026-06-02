package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRunMigrations(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping migration test")
	}
	ctx := context.Background()

	// Create a store and run migrations.
	st, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer st.Close()

	// Run migrations.
	if err := RunMigrations(ctx, st.Pool()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Verify schema_version table exists and has entries.
	var count int
	err = st.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM ploy.schema_version").Scan(&count)
	if err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one migration to be applied")
	}

	// Verify the latest entry has a non-zero applied_at.
	var appliedAt time.Time
	err = st.Pool().QueryRow(ctx, "SELECT applied_at FROM ploy.schema_version WHERE version = $1", SchemaVersion).Scan(&appliedAt)
	if err != nil {
		t.Fatalf("query applied_at: %v", err)
	}
	if appliedAt.IsZero() {
		t.Fatal("expected applied_at to be set for applied migration")
	}

	// Get the current version.
	version, err := getCurrentVersion(ctx, st.Pool())
	if err != nil {
		t.Fatalf("getCurrentVersion: %v", err)
	}
	if version == 0 {
		t.Fatal("expected version > 0 after migrations")
	}
	if version != SchemaVersion {
		t.Fatalf("version mismatch: got %d, want %d", version, SchemaVersion)
	}

	// Run migrations again (should be idempotent).
	if err := RunMigrations(ctx, st.Pool()); err != nil {
		t.Fatalf("RunMigrations (second run): %v", err)
	}

	// Verify version hasn't changed.
	newVersion, err := getCurrentVersion(ctx, st.Pool())
	if err != nil {
		t.Fatalf("getCurrentVersion (after second run): %v", err)
	}
	if newVersion != version {
		t.Fatalf("version changed after idempotent run: got %d, want %d", newVersion, version)
	}
}

func TestEnsureVersionTable(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping version table test")
	}
	ctx := context.Background()

	st, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer st.Close()

	// Call ensureVersionTable multiple times (should be idempotent).
	for i := 0; i < 3; i++ {
		if err := ensureVersionTable(ctx, st.Pool()); err != nil {
			t.Fatalf("ensureVersionTable (iteration %d): %v", i, err)
		}
	}

	// Verify table exists.
	var exists bool
	err = st.Pool().QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'ploy' AND table_name = 'schema_version')").Scan(&exists)
	if err != nil {
		t.Fatalf("query information_schema: %v", err)
	}
	if !exists {
		t.Fatal("schema_version table does not exist")
	}
}

func TestGetCurrentVersion(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping version test")
	}
	ctx := context.Background()

	st, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer st.Close()

	// Ensure version table exists.
	if err := ensureVersionTable(ctx, st.Pool()); err != nil {
		t.Fatalf("ensureVersionTable: %v", err)
	}

	// Clear any existing versions for a clean test.
	_, err = st.Pool().Exec(ctx, "DELETE FROM ploy.schema_version")
	if err != nil {
		t.Fatalf("clear schema_version: %v", err)
	}

	// Get version (should be 0 initially).
	version, err := getCurrentVersion(ctx, st.Pool())
	if err != nil {
		t.Fatalf("getCurrentVersion: %v", err)
	}
	if version != 0 {
		t.Fatalf("initial version: got %d, want 0", version)
	}

	// Insert a version.
	_, err = st.Pool().Exec(ctx, "INSERT INTO ploy.schema_version (version, applied_at) VALUES (5, now())")
	if err != nil {
		t.Fatalf("insert version: %v", err)
	}

	// Get version again (should be 5).
	version, err = getCurrentVersion(ctx, st.Pool())
	if err != nil {
		t.Fatalf("getCurrentVersion (after insert): %v", err)
	}
	if version != 5 {
		t.Fatalf("version after insert: got %d, want 5", version)
	}
}

func TestRunMigrations_RemovesObsoleteNodeUpdaterDiagnostics(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping migration test")
	}
	ctx := context.Background()

	st, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer st.Close()
	cleanTestTables(t, ctx, st)

	nodeID := types.NodeID(types.NewNodeKey())
	_, err = st.CreateNode(ctx, CreateNodeParams{
		ID:          nodeID,
		Name:        nodeNameForTest(nodeID),
		IpAddress:   nodeAddrForTest(nodeID),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	setupStatements := []struct {
		name string
		sql  string
		args []any
	}{
		{name: "drop diagnostics constraint", sql: `ALTER TABLE ploy.node_diagnostics DROP CONSTRAINT IF EXISTS node_diagnostics_component_check`},
		{name: "allow old diagnostics component", sql: `ALTER TABLE ploy.node_diagnostics ADD CONSTRAINT node_diagnostics_component_check CHECK (component IN ('node', 'node-updater'))`},
		{name: "drop logs constraint", sql: `ALTER TABLE ploy.node_daemon_logs DROP CONSTRAINT IF EXISTS node_daemon_logs_component_check`},
		{name: "allow old logs component", sql: `ALTER TABLE ploy.node_daemon_logs ADD CONSTRAINT node_daemon_logs_component_check CHECK (component IN ('node', 'node-updater'))`},
		{name: "insert obsolete diagnostic", sql: `INSERT INTO ploy.node_diagnostics (node_id, component, status, details) VALUES ($1, 'node-updater', 'ok', '{}'::jsonb)`, args: []any{nodeID.String()}},
		{name: "insert obsolete daemon log", sql: `INSERT INTO ploy.node_daemon_logs (node_id, component, stream, message) VALUES ($1, 'node-updater', 'system', 'old updater row')`, args: []any{nodeID.String()}},
		{name: "remove current version", sql: `DELETE FROM ploy.schema_version WHERE version >= $1`, args: []any{SchemaVersion}},
		{name: "restore prior version", sql: `INSERT INTO ploy.schema_version (version, applied_at) VALUES (2026060202, now()) ON CONFLICT (version) DO UPDATE SET applied_at = EXCLUDED.applied_at`},
	}
	for _, stmt := range setupStatements {
		if _, err := st.Pool().Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatalf("%s: %v", stmt.name, err)
		}
	}

	if err := RunMigrations(ctx, st.Pool()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	var diagCount, logCount int
	if err := st.Pool().QueryRow(ctx, `SELECT count(*) FROM ploy.node_diagnostics WHERE component = 'node-updater'`).Scan(&diagCount); err != nil {
		t.Fatalf("count obsolete diagnostics: %v", err)
	}
	if diagCount != 0 {
		t.Fatalf("obsolete diagnostics count = %d, want 0", diagCount)
	}
	if err := st.Pool().QueryRow(ctx, `SELECT count(*) FROM ploy.node_daemon_logs WHERE component = 'node-updater'`).Scan(&logCount); err != nil {
		t.Fatalf("count obsolete daemon logs: %v", err)
	}
	if logCount != 0 {
		t.Fatalf("obsolete daemon logs count = %d, want 0", logCount)
	}

	_, err = st.Pool().Exec(ctx, `
INSERT INTO ploy.node_diagnostics (node_id, component, status, details)
VALUES ($1, 'node-updater', 'ok', '{}'::jsonb)
`, nodeID.String())
	if err == nil {
		t.Fatal("expected node_diagnostics node-updater insert to fail")
	}
}
