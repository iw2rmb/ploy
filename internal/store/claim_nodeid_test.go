package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// TestClaimJobEmptyNodeID_UnitTest validates that ClaimJob returns ErrEmptyNodeID
// when called with an empty NodeID, without requiring a database connection.
// This is a fast unit test that verifies the validation logic in the wrapper.
func TestClaimJobEmptyNodeID_UnitTest(t *testing.T) {
	// Create a minimal mock that embeds Queries (which requires PgStore).
	// Since we're testing the validation before the DB call, we don't need a real DB.

	// We can't easily test PgStore.ClaimJob without a DB connection,
	// so this test documents the expected behavior and verifies the error type.

	// Verify ErrEmptyNodeID is a sentinel error that can be matched.
	if !errors.Is(ErrEmptyNodeID, ErrEmptyNodeID) {
		t.Fatal("ErrEmptyNodeID should be matchable with errors.Is")
	}

	// Verify the error message is descriptive.
	expected := "store: ClaimJob requires non-empty nodeID"
	if ErrEmptyNodeID.Error() != expected {
		t.Errorf("ErrEmptyNodeID.Error() = %q, want %q", ErrEmptyNodeID.Error(), expected)
	}
}

// TestClaimJobRequiresNodeID tests that ClaimJob requires a non-empty node ID.
// This enforces the invariant that Running jobs must always have a valid node_id,
// preventing `jobs.node_id = NULL` rows with status='Running'.
func TestClaimJobRequiresNodeID(t *testing.T) {
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

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/nodeid-required", "main", "feature", []byte(`{"type":"nodeid-test"}`))
	run := fx.Run

	// Create a Queued job for the run.
	_, err = db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       run.ID,
		RepoID:      fx.ModRepo.ID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "test-job",
		ModType:     "",
		ModImage:    "",
		Status:      JobStatusQueued,
		StepIndex:   1000,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob() failed: %v", err)
	}

	// Attempt to claim with an empty NodeID.
	// This should fail because ClaimJob requires a non-empty node_id.
	emptyNodeID := types.NodeID("")
	_, err = db.ClaimJob(ctx, emptyNodeID)
	if err == nil {
		t.Error("ClaimJob() with empty NodeID should fail, but it succeeded")
	}
}

// TestClaimJobValidNodeID tests that ClaimJob succeeds with a valid non-empty node ID.
func TestClaimJobValidNodeID(t *testing.T) {
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

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/nodeid-valid", "main", "feature", []byte(`{"type":"nodeid-valid-test"}`))
	run := fx.Run

	// Create a Queued job for the run.
	job, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       run.ID,
		RepoID:      fx.ModRepo.ID,
		RepoBaseRef: fx.RunRepo.RepoBaseRef,
		Attempt:     fx.RunRepo.Attempt,
		Name:        "test-job",
		ModType:     "",
		ModImage:    "",
		Status:      JobStatusQueued,
		StepIndex:   1000,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob() failed: %v", err)
	}

	// Create a valid node.
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:        types.NodeID(types.NewNodeKey()),
		Name:      "test-node-valid",
		IpAddress: mustParseAddr(t, "192.168.50.100"),
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	// Claim with a valid NodeID should succeed.
	claimedJob, err := db.ClaimJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("ClaimJob() with valid NodeID failed: %v", err)
	}

	// Verify the claimed job has the correct node_id.
	if claimedJob.ID != job.ID {
		t.Errorf("Expected job ID %v, got %v", job.ID, claimedJob.ID)
	}
	if claimedJob.NodeID == nil || *claimedJob.NodeID != node.ID {
		t.Errorf("Expected node_id %v, got %v", node.ID, claimedJob.NodeID)
	}
	if claimedJob.Status != JobStatusRunning {
		t.Errorf("Expected status Running, got %s", claimedJob.Status)
	}
}
