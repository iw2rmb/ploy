package store

import (
	"context"
	"os"
	"sort"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// TestClaimJobOrderingDeterministic verifies that ClaimJob ordering is deterministic
// when multiple jobs have the same step_index. Ties should resolve by job id (ASC).
//
// This test addresses roadmap/refactor/store.md: "Job claiming and scheduling are
// under-specified" — the fix adds a tie-breaker (ORDER BY step_index ASC, id ASC)
// to make ordering deterministic and prevent non-deterministic claim behavior.
//
// Requires PLOY_TEST_PG_DSN to be set with a test database.
func TestClaimJobOrderingDeterministic(t *testing.T) {
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

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/deterministic-order", "main", "feature", []byte(`{"type":"deterministic"}`))
	run := fx.Run

	// Create jobs with the SAME step_index to test tie-breaking by id.
	const sameStepIndex = types.StepIndex(5000)
	const numJobs = 5

	jobIDs := make([]types.JobID, numJobs)
	for i := 0; i < numJobs; i++ {
		jobIDs[i] = types.NewJobID()
	}

	// Sort job IDs lexicographically to know expected claim order.
	sortedIDs := make([]types.JobID, numJobs)
	copy(sortedIDs, jobIDs)
	sort.Slice(sortedIDs, func(i, j int) bool {
		return sortedIDs[i].String() < sortedIDs[j].String()
	})

	// Create jobs in arbitrary order (not sorted by ID).
	for i := 0; i < numJobs; i++ {
		_, err := db.CreateJob(ctx, CreateJobParams{
			ID:          jobIDs[i],
			RunID:       run.ID,
			RepoID:      fx.ModRepo.ID,
			RepoBaseRef: fx.RunRepo.RepoBaseRef,
			Attempt:     fx.RunRepo.Attempt,
			Name:        "job-tie-" + jobIDs[i].String(),
			ModType:     "",
			ModImage:    "",
			Status:      JobStatusQueued,
			StepIndex:   sameStepIndex,
			Meta:        []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("CreateJob(%d) failed: %v", i, err)
		}
	}

	// Create a test node.
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:        types.NodeID(types.NewNodeKey()),
		Name:      "test-node-deterministic",
		IpAddress: mustParseAddr(t, "192.168.50.100"),
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	// Claim all jobs and verify they come in sorted ID order (tie-breaker).
	claimedIDs := make([]types.JobID, 0, numJobs)
	for i := 0; i < numJobs; i++ {
		claimed, err := db.ClaimJob(ctx, node.ID)
		if err != nil {
			t.Fatalf("ClaimJob(%d) failed: %v", i, err)
		}
		claimedIDs = append(claimedIDs, claimed.ID)
	}

	// Verify order matches expected (sorted by id ASC).
	for i := 0; i < numJobs; i++ {
		if claimedIDs[i] != sortedIDs[i] {
			t.Errorf("ClaimJob order mismatch at position %d: got %s, want %s",
				i, claimedIDs[i], sortedIDs[i])
		}
	}

	// Log the claim order for debugging.
	t.Logf("Expected order (sorted by id): %v", sortedIDs)
	t.Logf("Actual claim order:            %v", claimedIDs)
}
