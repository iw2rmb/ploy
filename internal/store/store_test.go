package store

import (
	"context"
	"os"
	"testing"
)

// TestNewStore verifies that Store creation works with a valid DSN.
// This test is skipped if PLOY_TEST_PG_DSN is not set, following the pattern
// of integration tests that require external dependencies.
func TestNewStore(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store initialization test")
	}

	ctx := context.Background()
	store, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer store.Close()

	// Verify we can call a basic query.
	_, err = store.GetCluster(ctx)
	if err != nil {
		// It's okay if the cluster doesn't exist yet; we're just testing connectivity.
		t.Logf("GetCluster returned error (expected if DB is empty): %v", err)
	}
}

// TestNewStore_InvalidDSN verifies that Store creation fails gracefully with an invalid DSN.
func TestNewStore_InvalidDSN(t *testing.T) {
	ctx := context.Background()
	_, err := NewStore(ctx, "invalid-dsn")
	if err == nil {
		t.Fatal("NewStore() should have failed with invalid DSN")
	}
}
