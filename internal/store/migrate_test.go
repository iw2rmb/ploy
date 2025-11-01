package store

import (
	"context"
	"os"
	"testing"
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

	// Get the current version.
	version, err := getCurrentVersion(ctx, st.Pool())
	if err != nil {
		t.Fatalf("getCurrentVersion: %v", err)
	}
	if version == 0 {
		t.Fatal("expected version > 0 after migrations")
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

func TestLoadMigrations(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}

	if len(migrations) == 0 {
		t.Fatal("expected at least one migration")
	}

	// Verify migrations are sorted by version.
	for i := 1; i < len(migrations); i++ {
		if migrations[i].Version <= migrations[i-1].Version {
			t.Fatalf("migrations not sorted: %d <= %d", migrations[i].Version, migrations[i-1].Version)
		}
	}

	// Verify first migration has version 1.
	if migrations[0].Version != 1 {
		t.Fatalf("first migration version: got %d, want 1", migrations[0].Version)
	}

	// Verify each migration has SQL content.
	for _, m := range migrations {
		if m.SQL == "" {
			t.Fatalf("migration %d has empty SQL", m.Version)
		}
		if m.Name == "" {
			t.Fatalf("migration %d has empty name", m.Version)
		}
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

	// Get version (should be 0 initially).
	version, err := getCurrentVersion(ctx, st.Pool())
	if err != nil {
		t.Fatalf("getCurrentVersion: %v", err)
	}
	if version != 0 {
		t.Fatalf("initial version: got %d, want 0", version)
	}

	// Insert a version.
	_, err = st.Pool().Exec(ctx, "INSERT INTO ploy.schema_version (version, name) VALUES (5, 'test')")
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
