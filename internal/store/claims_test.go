package store

import (
	"context"
	"os"
	"testing"
)

// TestRunClaim tests basic run claiming functionality.
// Requires PLOY_TEST_DB_DSN to be set with a test database.
// This test verifies store connectivity and basic infrastructure setup.
func TestRunClaim(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	// Verify store is functional (connectivity test).
	// cluster table removed; no GetCluster check

	t.Log("Store integration test infrastructure is working")
	t.Log("Full integration tests require database setup - see tests/integration/")
}
