package store

import (
	"context"
	"net/netip"
	"reflect"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestListStaleRunningJobs_FiltersByHeartbeatAndStatus(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	// Cancel all leftover Running jobs so the count assertions below are not
	// polluted by prior test runs sharing the same database.
	if _, err := db.Pool().Exec(ctx, "UPDATE jobs SET status = 'Cancelled' WHERE status = 'Running'"); err != nil {
		t.Fatalf("cleanup running jobs: %v", err)
	}

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/stale-list-a", "main", "feature", []byte(`{"type":"stale-list"}`))
	repoB := createRunRepoForStaleRecoveryQueryTest(t, ctx, db, fx.Mig.ID, fx.Run.ID, "https://github.com/test/stale-list-b", "main", "feature-b")

	cutoff := time.Now().UTC().Add(-45 * time.Second)
	cutoffTS := pgtype.Timestamptz{Time: cutoff, Valid: true}

	staleNode := createNodeForStaleRecoveryQueryTest(t, ctx, db)
	nilHeartbeatNode := createNodeForStaleRecoveryQueryTest(t, ctx, db)
	freshNode := createNodeForStaleRecoveryQueryTest(t, ctx, db)

	setNodeHeartbeatForStaleRecoveryQueryTest(t, ctx, db, staleNode.ID, cutoff.Add(-2*time.Minute))
	setNodeHeartbeatForStaleRecoveryQueryTest(t, ctx, db, freshNode.ID, cutoff.Add(2*time.Minute))

	staleAttemptOneA := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, 1, "stale-attempt1-a", types.JobStatusCreated)
	staleAttemptOneB := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, 1, "stale-attempt1-b", types.JobStatusCreated)
	staleAttemptTwo := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, 2, "stale-attempt2", types.JobStatusCreated)
	freshRunning := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, repoB.RepoID, repoB.RepoBaseRef, 1, "fresh-running", types.JobStatusCreated)
	_ = createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, repoB.RepoID, repoB.RepoBaseRef, 1, "stale-non-running", types.JobStatusQueued)

	setJobRunningForStaleRecoveryQueryTest(t, ctx, db, staleAttemptOneA.ID, &staleNode.ID, time.Now().UTC().Add(-3*time.Minute))
	setJobRunningForStaleRecoveryQueryTest(t, ctx, db, staleAttemptOneB.ID, &nilHeartbeatNode.ID, time.Now().UTC().Add(-2*time.Minute))
	setJobRunningForStaleRecoveryQueryTest(t, ctx, db, staleAttemptTwo.ID, nil, time.Now().UTC().Add(-1*time.Minute))
	setJobRunningForStaleRecoveryQueryTest(t, ctx, db, freshRunning.ID, &freshNode.ID, time.Now().UTC().Add(-1*time.Minute))

	rows, err := db.ListStaleRunningJobs(ctx, cutoffTS)
	if err != nil {
		t.Fatalf("ListStaleRunningJobs() failed: %v", err)
	}

	type staleKey struct {
		runID   types.RunID
		repoID  types.RepoID
		attempt int32
	}
	got := make(map[staleKey]int32, len(rows))
	for _, row := range rows {
		got[staleKey{
			runID:   row.RunID,
			repoID:  row.RepoID,
			attempt: row.Attempt,
		}] = row.RunningJobs
	}

	if got[staleKey{runID: fx.Run.ID, repoID: fx.RunRepo.RepoID, attempt: 1}] != 2 {
		t.Fatalf("attempt 1 stale running count=%d, want 2", got[staleKey{runID: fx.Run.ID, repoID: fx.RunRepo.RepoID, attempt: 1}])
	}
	if got[staleKey{runID: fx.Run.ID, repoID: fx.RunRepo.RepoID, attempt: 2}] != 1 {
		t.Fatalf("attempt 2 stale running count=%d, want 1", got[staleKey{runID: fx.Run.ID, repoID: fx.RunRepo.RepoID, attempt: 2}])
	}
	if _, ok := got[staleKey{runID: fx.Run.ID, repoID: repoB.RepoID, attempt: 1}]; ok {
		t.Fatal("fresh/non-running repo attempt must not be returned")
	}

	rowsSecondRead, err := db.ListStaleRunningJobs(ctx, cutoffTS)
	if err != nil {
		t.Fatalf("ListStaleRunningJobs(second read) failed: %v", err)
	}
	if !reflect.DeepEqual(rows, rowsSecondRead) {
		t.Fatal("ListStaleRunningJobs() order/content changed between consecutive calls")
	}
}

func TestCountStaleNodesWithRunningJobs_CountsDistinctAssignedStaleNodes(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	// Ensure no leftover Running jobs or stale nodes from prior test runs
	// affect the exact count assertion below.
	if _, err := db.Pool().Exec(ctx, "UPDATE jobs SET status = 'Cancelled' WHERE status = 'Running'"); err != nil {
		t.Fatalf("cleanup running jobs: %v", err)
	}

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/stale-node-count-a", "main", "feature", []byte(`{"type":"stale-node-count"}`))
	repoB := createRunRepoForStaleRecoveryQueryTest(t, ctx, db, fx.Mig.ID, fx.Run.ID, "https://github.com/test/stale-node-count-b", "main", "feature-b")

	cutoff := time.Now().UTC().Add(-45 * time.Second)
	cutoffTS := pgtype.Timestamptz{Time: cutoff, Valid: true}

	staleNodeA := createNodeForStaleRecoveryQueryTest(t, ctx, db)
	staleNodeB := createNodeForStaleRecoveryQueryTest(t, ctx, db)
	nilHeartbeatNode := createNodeForStaleRecoveryQueryTest(t, ctx, db)
	freshNode := createNodeForStaleRecoveryQueryTest(t, ctx, db)

	setNodeHeartbeatForStaleRecoveryQueryTest(t, ctx, db, staleNodeA.ID, cutoff.Add(-2*time.Minute))
	setNodeHeartbeatForStaleRecoveryQueryTest(t, ctx, db, staleNodeB.ID, cutoff.Add(-3*time.Minute))
	setNodeHeartbeatForStaleRecoveryQueryTest(t, ctx, db, freshNode.ID, cutoff.Add(2*time.Minute))

	staleAOne := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, 1, "stale-a-1", types.JobStatusCreated)
	staleATwo := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, repoB.RepoID, repoB.RepoBaseRef, 1, "stale-a-2", types.JobStatusCreated)
	staleB := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, 2, "stale-b", types.JobStatusCreated)
	nilHeartbeat := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, repoB.RepoID, repoB.RepoBaseRef, 2, "nil-heartbeat", types.JobStatusCreated)
	orphaned := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, 3, "orphaned", types.JobStatusCreated)
	fresh := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, repoB.RepoID, repoB.RepoBaseRef, 3, "fresh", types.JobStatusCreated)

	setJobRunningForStaleRecoveryQueryTest(t, ctx, db, staleAOne.ID, &staleNodeA.ID, time.Now().UTC().Add(-2*time.Minute))
	setJobRunningForStaleRecoveryQueryTest(t, ctx, db, staleATwo.ID, &staleNodeA.ID, time.Now().UTC().Add(-2*time.Minute))
	setJobRunningForStaleRecoveryQueryTest(t, ctx, db, staleB.ID, &staleNodeB.ID, time.Now().UTC().Add(-2*time.Minute))
	setJobRunningForStaleRecoveryQueryTest(t, ctx, db, nilHeartbeat.ID, &nilHeartbeatNode.ID, time.Now().UTC().Add(-2*time.Minute))
	setJobRunningForStaleRecoveryQueryTest(t, ctx, db, orphaned.ID, nil, time.Now().UTC().Add(-2*time.Minute))
	setJobRunningForStaleRecoveryQueryTest(t, ctx, db, fresh.ID, &freshNode.ID, time.Now().UTC().Add(-2*time.Minute))

	count, err := db.CountStaleNodesWithRunningJobs(ctx, cutoffTS)
	if err != nil {
		t.Fatalf("CountStaleNodesWithRunningJobs() failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("CountStaleNodesWithRunningJobs()=%d, want 3", count)
	}
}

func TestCancelActiveJobsByRunRepoAttempt_TransitionsOnlyTargetAttempt(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/stale-cancel-a", "main", "feature", []byte(`{"type":"stale-cancel"}`))
	repoB := createRunRepoForStaleRecoveryQueryTest(t, ctx, db, fx.Mig.ID, fx.Run.ID, "https://github.com/test/stale-cancel-b", "main", "feature-b")

	targetCreated := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, 1, "target-created", types.JobStatusCreated)
	targetQueued := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, 1, "target-queued", types.JobStatusQueued)
	targetRunning := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, 1, "target-running", types.JobStatusCreated)
	targetSuccess := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, 1, "target-success", types.JobStatusSuccess)
	otherAttemptQueued := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, fx.RunRepo.RepoID, fx.RunRepo.RepoBaseRef, 2, "other-attempt-queued", types.JobStatusQueued)
	otherRepoQueued := createJobForStaleRecoveryQueryTest(t, ctx, db, fx.Run.ID, repoB.RepoID, repoB.RepoBaseRef, 1, "other-repo-queued", types.JobStatusQueued)

	setJobRunningForStaleRecoveryQueryTest(t, ctx, db, targetRunning.ID, nil, time.Now().UTC().Add(-3*time.Second))

	affected, err := db.CancelActiveJobsByRunRepoAttempt(ctx, CancelActiveJobsByRunRepoAttemptParams{
		RunID:   fx.Run.ID,
		RepoID:  fx.RunRepo.RepoID,
		Attempt: 1,
	})
	if err != nil {
		t.Fatalf("CancelActiveJobsByRunRepoAttempt() failed: %v", err)
	}
	if affected != 3 {
		t.Fatalf("CancelActiveJobsByRunRepoAttempt() affected=%d, want 3", affected)
	}

	createdAfter, err := db.GetJob(ctx, targetCreated.ID)
	if err != nil {
		t.Fatalf("GetJob(targetCreated) failed: %v", err)
	}
	if createdAfter.Status != types.JobStatusCancelled {
		t.Fatalf("targetCreated status=%q, want %q", createdAfter.Status, types.JobStatusCancelled)
	}
	if !createdAfter.FinishedAt.Valid {
		t.Fatal("targetCreated finished_at must be set")
	}
	if createdAfter.DurationMs != 0 {
		t.Fatalf("targetCreated duration_ms=%d, want 0", createdAfter.DurationMs)
	}

	queuedAfter, err := db.GetJob(ctx, targetQueued.ID)
	if err != nil {
		t.Fatalf("GetJob(targetQueued) failed: %v", err)
	}
	if queuedAfter.Status != types.JobStatusCancelled {
		t.Fatalf("targetQueued status=%q, want %q", queuedAfter.Status, types.JobStatusCancelled)
	}
	if !queuedAfter.FinishedAt.Valid {
		t.Fatal("targetQueued finished_at must be set")
	}
	if queuedAfter.DurationMs != 0 {
		t.Fatalf("targetQueued duration_ms=%d, want 0", queuedAfter.DurationMs)
	}

	runningAfter, err := db.GetJob(ctx, targetRunning.ID)
	if err != nil {
		t.Fatalf("GetJob(targetRunning) failed: %v", err)
	}
	if runningAfter.Status != types.JobStatusCancelled {
		t.Fatalf("targetRunning status=%q, want %q", runningAfter.Status, types.JobStatusCancelled)
	}
	if !runningAfter.FinishedAt.Valid {
		t.Fatal("targetRunning finished_at must be set")
	}
	if runningAfter.DurationMs <= 0 {
		t.Fatalf("targetRunning duration_ms=%d, want > 0", runningAfter.DurationMs)
	}

	successAfter, err := db.GetJob(ctx, targetSuccess.ID)
	if err != nil {
		t.Fatalf("GetJob(targetSuccess) failed: %v", err)
	}
	if successAfter.Status != types.JobStatusSuccess {
		t.Fatalf("targetSuccess status=%q, want %q", successAfter.Status, types.JobStatusSuccess)
	}

	otherAttemptAfter, err := db.GetJob(ctx, otherAttemptQueued.ID)
	if err != nil {
		t.Fatalf("GetJob(otherAttemptQueued) failed: %v", err)
	}
	if otherAttemptAfter.Status != types.JobStatusQueued {
		t.Fatalf("otherAttemptQueued status=%q, want %q", otherAttemptAfter.Status, types.JobStatusQueued)
	}

	otherRepoAfter, err := db.GetJob(ctx, otherRepoQueued.ID)
	if err != nil {
		t.Fatalf("GetJob(otherRepoQueued) failed: %v", err)
	}
	if otherRepoAfter.Status != types.JobStatusQueued {
		t.Fatalf("otherRepoQueued status=%q, want %q", otherRepoAfter.Status, types.JobStatusQueued)
	}
}

func createNodeForStaleRecoveryQueryTest(t *testing.T, ctx context.Context, db Store) Node {
	t.Helper()

	key := types.NewNodeKey()
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          types.NodeID(key),
		Name:        "stale-recovery-node-" + key,
		IpAddress:   netip.AddrFrom4([4]byte{key[0], key[1], key[2], key[3]}),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}
	return node
}

func setNodeHeartbeatForStaleRecoveryQueryTest(t *testing.T, ctx context.Context, db Store, nodeID types.NodeID, heartbeat time.Time) {
	t.Helper()

	if err := db.UpdateNodeHeartbeat(ctx, UpdateNodeHeartbeatParams{
		ID:             nodeID,
		LastHeartbeat:  pgtype.Timestamptz{Time: heartbeat.UTC(), Valid: true},
		CpuTotalMillis: 2000,
		CpuFreeMillis:  1000,
		MemTotalBytes:  4096,
		MemFreeBytes:   2048,
		DiskTotalBytes: 8192,
		DiskFreeBytes:  4096,
		Version:        "test",
	}); err != nil {
		t.Fatalf("UpdateNodeHeartbeat(%s) failed: %v", nodeID, err)
	}
}

func createRunRepoForStaleRecoveryQueryTest(
	t *testing.T,
	ctx context.Context,
	db Store,
	migID types.MigID,
	runID types.RunID,
	repoURL, baseRef, targetRef string,
) RunRepo {
	t.Helper()

	repoID := types.NewMigRepoID()
	migRepo, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:        repoID,
		MigID:     migID,
		Url:       repoURL,
		BaseRef:   baseRef,
		TargetRef: targetRef,
	})
	if err != nil {
		t.Fatalf("CreateMigRepo(%s) failed: %v", repoURL, err)
	}

	runRepo, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
		MigID:           migID,
		RunID:           runID,
		RepoID:          migRepo.RepoID,
		RepoBaseRef:     migRepo.BaseRef,
		RepoTargetRef:   migRepo.TargetRef,
		SourceCommitSha: "0123456789abcdef0123456789abcdef01234567",
		RepoSha0:        "0123456789abcdef0123456789abcdef01234567",
	})
	if err != nil {
		t.Fatalf("CreateRunRepo(%s) failed: %v", repoURL, err)
	}

	return runRepo
}

func createJobForStaleRecoveryQueryTest(
	t *testing.T,
	ctx context.Context,
	db Store,
	runID types.RunID,
	repoID types.RepoID,
	repoBaseRef string,
	attempt int32,
	name string,
	status types.JobStatus,
) Job {
	t.Helper()

	job, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: repoBaseRef,
		Attempt:     attempt,
		Name:        name,
		Status:      status,
		JobType:     "mig",
		JobImage:    "test-image",
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(%s) failed: %v", name, err)
	}
	return job
}

func setJobRunningForStaleRecoveryQueryTest(
	t *testing.T,
	ctx context.Context,
	db Store,
	jobID types.JobID,
	nodeID *types.NodeID,
	startedAt time.Time,
) {
	t.Helper()

	if _, err := db.Pool().Exec(ctx, `
UPDATE jobs
SET status = 'Running',
    node_id = $2,
    started_at = $3,
    finished_at = NULL,
    duration_ms = 0
WHERE id = $1
`, jobID, nodeID, pgtype.Timestamptz{Time: startedAt.UTC(), Valid: true}); err != nil {
		t.Fatalf("set job running for %s failed: %v", jobID, err)
	}
}
