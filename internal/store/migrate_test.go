package store

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestRunMigrations(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping migration test")
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
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping version table test")
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
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping version test")
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
