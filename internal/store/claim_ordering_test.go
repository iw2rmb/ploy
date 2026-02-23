package store

import (
	"context"
	"os"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// TestClaimJobOrderingDeterministic verifies that ClaimJob ordering is deterministic
// when multiple jobs have the same step_index. Ties should resolve by job id (ASC).
//
// The implementation scopes ordering to the correct domain and adds a stable
// tie-breaker (… ORDER BY run_id, repo_id, attempt, step_index, id) so claim
// behavior cannot vary when step_index ties.
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
			RepoID:      fx.ModRepo.ID,
			RepoBaseRef: fx.RunRepo.RepoBaseRef,
			Attempt:     fx.RunRepo.Attempt,
			Name:        "job-tie-" + insertIDs[i].String(),
			JobType:     "",
			JobImage:    "",
			Status:      JobStatusQueued,
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
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          types.NodeID(types.NewNodeKey()),
		Name:        "test-node-deterministic",
		IpAddress:   mustParseAddr(t, "192.168.50.100"),
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

func TestClaimJobOrderingScopedByRunRepoAttempt(t *testing.T) {
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

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/scoped-order", "main", "feature", []byte(`{"type":"scoped"}`))

	// Add a second repo to the same run.
	repo2ID := types.NewModRepoID()
	repo2, err := db.CreateModRepo(ctx, CreateModRepoParams{
		ID:        repo2ID,
		ModID:     fx.Mod.ID,
		RepoUrl:   "https://github.com/test/scoped-order-2",
		BaseRef:   fx.ModRepo.BaseRef,
		TargetRef: fx.ModRepo.TargetRef,
	})
	if err != nil {
		t.Fatalf("CreateModRepo(repo2) failed: %v", err)
	}

	runRepo2, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
		ModID:         fx.Mod.ID,
		RunID:         fx.Run.ID,
		RepoID:        repo2.ID,
		RepoBaseRef:   fx.RunRepo.RepoBaseRef,
		RepoTargetRef: fx.RunRepo.RepoTargetRef,
	})
	if err != nil {
		t.Fatalf("CreateRunRepo(runRepo2) failed: %v", err)
	}

	// Add a second run that includes both repos.
	run2ID := types.NewRunID()
	run2, err := db.CreateRun(ctx, CreateRunParams{
		ID:        run2ID,
		ModID:     fx.Mod.ID,
		SpecID:    fx.Spec.ID,
		CreatedBy: fx.Run.CreatedBy,
	})
	if err != nil {
		t.Fatalf("CreateRun(run2) failed: %v", err)
	}

	run2Repo1, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
		ModID:         fx.Mod.ID,
		RunID:         run2.ID,
		RepoID:        fx.ModRepo.ID,
		RepoBaseRef:   fx.RunRepo.RepoBaseRef,
		RepoTargetRef: fx.RunRepo.RepoTargetRef,
	})
	if err != nil {
		t.Fatalf("CreateRunRepo(run2Repo1) failed: %v", err)
	}

	run2Repo2, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
		ModID:         fx.Mod.ID,
		RunID:         run2.ID,
		RepoID:        repo2.ID,
		RepoBaseRef:   fx.RunRepo.RepoBaseRef,
		RepoTargetRef: fx.RunRepo.RepoTargetRef,
	})
	if err != nil {
		t.Fatalf("CreateRunRepo(run2Repo2) failed: %v", err)
	}

	// Determine which run_id/repo_id sort first in the database (collation-safe).
	var runLowStr string
	if err := db.Pool().QueryRow(ctx, `SELECT id FROM (VALUES ($1::text), ($2::text)) v(id) ORDER BY id ASC LIMIT 1`, fx.Run.ID, run2.ID).Scan(&runLowStr); err != nil {
		t.Fatalf("QueryRow(runLow) failed: %v", err)
	}
	runLow := types.RunID(runLowStr)
	runHigh := fx.Run.ID
	if runLowStr == fx.Run.ID.String() {
		runHigh = run2.ID
	}

	var repoLowStr string
	if err := db.Pool().QueryRow(ctx, `SELECT id FROM (VALUES ($1::text), ($2::text)) v(id) ORDER BY id ASC LIMIT 1`, fx.ModRepo.ID, repo2.ID).Scan(&repoLowStr); err != nil {
		t.Fatalf("QueryRow(repoLow) failed: %v", err)
	}
	repoLow := types.ModRepoID(repoLowStr)
	repoHigh := fx.ModRepo.ID
	if repoLowStr == fx.ModRepo.ID.String() {
		repoHigh = repo2.ID
	}

	type runRepoKey struct {
		run  types.RunID
		repo types.ModRepoID
	}
	runRepoByKey := map[runRepoKey]RunRepo{
		{run: fx.Run.ID, repo: fx.ModRepo.ID}: fx.RunRepo,
		{run: fx.Run.ID, repo: repo2.ID}:      runRepo2,
		{run: run2.ID, repo: fx.ModRepo.ID}:   run2Repo1,
		{run: run2.ID, repo: repo2.ID}:        run2Repo2,
	}

	createJob := func(runID types.RunID, repoID types.ModRepoID, stepIndex types.StepIndex) types.JobID {
		t.Helper()
		rr, ok := runRepoByKey[runRepoKey{run: runID, repo: repoID}]
		if !ok {
			t.Fatalf("missing runRepo for run_id=%s repo_id=%s", runID, repoID)
		}

		jobID := types.NewJobID()
		if _, err := db.CreateJob(ctx, CreateJobParams{
			ID:          jobID,
			RunID:       runID,
			RepoID:      repoID,
			RepoBaseRef: rr.RepoBaseRef,
			Attempt:     rr.Attempt,
			Name:        "job-scoped-" + jobID.String(),
			JobType:     "",
			JobImage:    "",
			Status:      JobStatusQueued,
			NextID:      nil,
			Meta:        []byte(`{}`),
		}); err != nil {
			t.Fatalf("CreateJob(run_id=%s repo_id=%s) failed: %v", runID, repoID, err)
		}
		return jobID
	}

	// Force conflicts so ordering must respect run_id/repo_id before step_index:
	// - run_low + repo_low gets the largest step_index
	// - run_low + repo_high gets a smaller step_index
	// - run_high + repo_low gets the smallest step_index
	jobRunLowRepoLow := createJob(runLow, repoLow, types.StepIndex(2000))
	jobRunLowRepoHigh := createJob(runLow, repoHigh, types.StepIndex(1000))
	jobRunHighRepoLow := createJob(runHigh, repoLow, types.StepIndex(500))

	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          types.NodeID(types.NewNodeKey()),
		Name:        "test-node-scoped",
		IpAddress:   mustParseAddr(t, "192.168.50.101"),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	claimed := make([]types.JobID, 0, 3)
	for i := 0; i < 3; i++ {
		job, err := db.ClaimJob(ctx, node.ID)
		if err != nil {
			t.Fatalf("ClaimJob(%d) failed: %v", i, err)
		}
		claimed = append(claimed, job.ID)
	}

	want := []types.JobID{
		jobRunLowRepoLow,
		jobRunLowRepoHigh,
		jobRunHighRepoLow,
	}
	for i := range want {
		if claimed[i] != want[i] {
			t.Fatalf("ClaimJob order mismatch at position %d: got %s, want %s (run_low=%s repo_low=%s)",
				i, claimed[i], want[i], runLow, repoLow)
		}
	}
}
