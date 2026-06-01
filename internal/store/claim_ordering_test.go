package store

import (
	"context"
	"os"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// TestClaimJobOrderingDeterministic verifies that ClaimJob ordering is deterministic
// when multiple jobs have the same next_id. Ties should resolve by job id (ASC).
//
// The implementation scopes ordering to the correct domain and adds a stable
// tie-breaker (… ORDER BY run_id, repo_id, attempt, next_id, id) so claim
// behavior cannot vary when next_id ties.
//
// Requires PLOY_TEST_DB_DSN to be set with a test database.
func TestClaimJobOrderingDeterministic(t *testing.T) {
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
	cleanTestTables(t, ctx, db)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/deterministic-order", "main", []byte(`{"type":"deterministic"}`))
	run := fx.Run

	if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{ID: run.ID, Status: types.RunStatusRunning}); err != nil {
		t.Fatalf("UpdateRunStatus(running) failed: %v", err)
	}

	// Create jobs with the same scheduling shape to test tie-breaking by id.
	const numJobs = 5

	jobIDs := make([]types.JobID, numJobs)
	for i := 0; i < numJobs; i++ {
		jobIDs[i] = types.NewJobID()
	}

	// Insert in reverse generation order so a missing tie-breaker would be likely to show up.
	insertIDs := make([]types.JobID, numJobs)
	copy(insertIDs, jobIDs)
	for i, j := 0, len(insertIDs)-1; i < j; i, j = i+1, j-1 {
		insertIDs[i], insertIDs[j] = insertIDs[j], insertIDs[i]
	}

	for i := 0; i < numJobs; i++ {
		_, err := db.CreateJob(ctx, CreateJobParams{
			ID:          insertIDs[i],
			RunID:       run.ID,
			RepoID:      fx.MigRepo.RepoID,
			RepoBaseRef: fx.Run.RepoBaseRef,
			Attempt:     fx.Run.Attempt,
			Name:        "job-tie-" + insertIDs[i].String(),
			JobType:     "mig",
			JobImage:    "",
			Status:      types.JobStatusQueued,
			NextID:      nil,
			Meta:        []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("CreateJob(%d) failed: %v", i, err)
		}
	}

	// Compute expected order using the database collation.
	idStrings := make([]string, 0, numJobs)
	for _, id := range insertIDs {
		idStrings = append(idStrings, id.String())
	}

	rows, err := db.Pool().Query(ctx, `SELECT id FROM jobs WHERE id = ANY($1::text[]) ORDER BY id ASC`, idStrings)
	if err != nil {
		t.Fatalf("Query(expected IDs) failed: %v", err)
	}
	defer rows.Close()

	expected := make([]types.JobID, 0, numJobs)
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("Scan(expected ID) failed: %v", err)
		}
		expected = append(expected, types.JobID(s))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("Rows(expected IDs) failed: %v", err)
	}
	if len(expected) != numJobs {
		t.Fatalf("Expected %d jobs in expected-order query, got %d", numJobs, len(expected))
	}

	// Create a test node.
	detID := types.NodeID(types.NewNodeKey())
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          detID,
		Name:        nodeNameForTest(detID),
		IpAddress:   nodeAddrForTest(detID),
		Concurrency: 1,
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
		if claimedIDs[i] != expected[i] {
			t.Errorf("ClaimJob order mismatch at position %d: got %s, want %s",
				i, claimedIDs[i], expected[i])
		}
	}

	// Log the claim order for debugging.
	t.Logf("Expected order (DB-sorted by id): %v", expected)
	t.Logf("Actual claim order:            %v", claimedIDs)
}

func TestClaimJobOrderingScopedByRunAttempt(t *testing.T) {
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
	cleanTestTables(t, ctx, db)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/scoped-order", "main", []byte(`{"type":"scoped"}`))
	run2 := createRunForStoreTest(t, ctx, db, fx.Wave.ID, fx.Mig.ID, fx.Spec.ID, "https://github.com/test/scoped-order-2", "main", types.RunStatusQueued)

	if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{ID: fx.Run.ID, Status: types.RunStatusRunning}); err != nil {
		t.Fatalf("UpdateRunStatus(run1 running) failed: %v", err)
	}
	if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{ID: run2.ID, Status: types.RunStatusRunning}); err != nil {
		t.Fatalf("UpdateRunStatus(run2 running) failed: %v", err)
	}

	var runLowStr string
	if err := db.Pool().QueryRow(ctx, `SELECT id FROM (VALUES ($1::text), ($2::text)) v(id) ORDER BY id ASC LIMIT 1`, fx.Run.ID, run2.ID).Scan(&runLowStr); err != nil {
		t.Fatalf("QueryRow(runLow) failed: %v", err)
	}
	runLow := types.RunID(runLowStr)
	runHigh := fx.Run.ID
	if runLowStr == fx.Run.ID.String() {
		runHigh = run2.ID
	}

	runByID := map[types.RunID]Run{
		fx.Run.ID: fx.Run,
		run2.ID:   run2,
	}

	createJob := func(runID types.RunID) types.JobID {
		t.Helper()
		run, ok := runByID[runID]
		if !ok {
			t.Fatalf("missing run for run_id=%s", runID)
		}

		jobID := types.NewJobID()
		if _, err := db.CreateJob(ctx, CreateJobParams{
			ID:          jobID,
			RunID:       runID,
			RepoID:      run.RepoID,
			RepoBaseRef: run.RepoBaseRef,
			Attempt:     run.Attempt,
			Name:        "job-scoped-" + jobID.String(),
			JobType:     "mig",
			JobImage:    "",
			Status:      types.JobStatusQueued,
			NextID:      nil,
			Meta:        []byte(`{}`),
		}); err != nil {
			t.Fatalf("CreateJob(run_id=%s) failed: %v", runID, err)
		}
		return jobID
	}

	jobRunLow := createJob(runLow)
	jobRunHigh := createJob(runHigh)

	scopedID := types.NodeID(types.NewNodeKey())
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          scopedID,
		Name:        nodeNameForTest(scopedID),
		IpAddress:   nodeAddrForTest(scopedID),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	first, err := db.ClaimJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("ClaimJob(first) failed: %v", err)
	}
	second, err := db.ClaimJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("ClaimJob(second) failed: %v", err)
	}

	if first.ID != jobRunLow || second.ID != jobRunHigh {
		t.Fatalf("ClaimJob order got [%s %s], want [%s %s] (run_low=%s)",
			first.ID, second.ID, jobRunLow, jobRunHigh, runLow)
	}
}
