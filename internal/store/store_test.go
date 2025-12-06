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

	// Connectivity verified by running migrations in NewStore; no cluster table anymore.
}

// TestNewStore_InvalidDSN verifies that Store creation fails gracefully with an invalid DSN.
func TestNewStore_InvalidDSN(t *testing.T) {
	ctx := context.Background()
	_, err := NewStore(ctx, "invalid-dsn")
	if err == nil {
		t.Fatal("NewStore() should have failed with invalid DSN")
	}
}

// TestCreateRun_WithAndWithoutName verifies that runs can be created with or without
// an optional batch name, and that the name round-trips correctly through the database.
// This test covers the optional batch naming feature per ROADMAP.md line 24.
func TestCreateRun_WithAndWithoutName(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	// Subtest: Create a run without a name (nil).
	t.Run("without_name", func(t *testing.T) {
		run, err := db.CreateRun(ctx, CreateRunParams{
			Name:      nil, // No batch name.
			RepoUrl:   "https://github.com/test/no-name",
			Spec:      []byte(`{"type":"unnamed-run"}`),
			Status:    RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature/unnamed",
		})
		if err != nil {
			t.Fatalf("CreateRun() failed: %v", err)
		}

		// Verify name is nil.
		if run.Name != nil {
			t.Errorf("expected run.Name to be nil, got %q", *run.Name)
		}

		// Fetch and verify round-trip.
		fetched, err := db.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetRun() failed: %v", err)
		}
		if fetched.Name != nil {
			t.Errorf("expected fetched run.Name to be nil, got %q", *fetched.Name)
		}
	})

	// Subtest: Create a run with a batch name.
	t.Run("with_name", func(t *testing.T) {
		batchName := "my-batch-2024-12-06"
		run, err := db.CreateRun(ctx, CreateRunParams{
			Name:      &batchName,
			RepoUrl:   "https://github.com/test/with-name",
			Spec:      []byte(`{"type":"named-run"}`),
			Status:    RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature/named",
		})
		if err != nil {
			t.Fatalf("CreateRun() failed: %v", err)
		}

		// Verify name is set.
		if run.Name == nil {
			t.Fatal("expected run.Name to be set, got nil")
		}
		if *run.Name != batchName {
			t.Errorf("expected run.Name = %q, got %q", batchName, *run.Name)
		}

		// Fetch and verify round-trip.
		fetched, err := db.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetRun() failed: %v", err)
		}
		if fetched.Name == nil {
			t.Fatal("expected fetched run.Name to be set, got nil")
		}
		if *fetched.Name != batchName {
			t.Errorf("expected fetched run.Name = %q, got %q", batchName, *fetched.Name)
		}
	})

	// Subtest: List runs includes name field correctly.
	t.Run("list_includes_name", func(t *testing.T) {
		runs, err := db.ListRuns(ctx, ListRunsParams{Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("ListRuns() failed: %v", err)
		}

		// Find at least one named and one unnamed run from previous subtests.
		var foundNamed, foundUnnamed bool
		for _, r := range runs {
			if r.Name != nil && *r.Name == "my-batch-2024-12-06" {
				foundNamed = true
			}
			if r.Name == nil && r.RepoUrl == "https://github.com/test/no-name" {
				foundUnnamed = true
			}
		}

		if !foundNamed {
			t.Error("expected to find a named run in ListRuns output")
		}
		if !foundUnnamed {
			t.Error("expected to find an unnamed run in ListRuns output")
		}
	})
}
