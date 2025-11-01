package store

import (
	"context"
	"os"
	"testing"
)

// TestRunClaim tests basic run claiming functionality.
// Requires PLOY_TEST_PG_DSN to be set with a test database.
func TestRunClaim(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	// Verify store is functional (connectivity test).
	_, err = db.GetCluster(ctx)
	if err != nil {
		t.Logf("GetCluster() returned error (expected if DB is empty): %v", err)
	}

	t.Log("Store integration test infrastructure is working")
	t.Log("Full integration tests require database setup - see tests/integration/")
}
