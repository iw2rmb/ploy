package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCancelActiveRunReposByRun_TransitionsOnlyQueuedRunning(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-repos-a", "main", "feature", []byte(`{"type":"cancel-repos"}`))

	runningRepo := createRunRepoForCancelBulkTest(t, ctx, db, fx.Mig.ID, fx.Run.ID, "https://github.com/test/cancel-repos-running", "main", "feature-running", RunRepoStatusRunning)
	successRepo := createRunRepoForCancelBulkTest(t, ctx, db, fx.Mig.ID, fx.Run.ID, "https://github.com/test/cancel-repos-success", "main", "feature-success", RunRepoStatusSuccess)
	failRepo := createRunRepoForCancelBulkTest(t, ctx, db, fx.Mig.ID, fx.Run.ID, "https://github.com/test/cancel-repos-fail", "main", "feature-fail", RunRepoStatusFail)
	cancelledRepo := createRunRepoForCancelBulkTest(t, ctx, db, fx.Mig.ID, fx.Run.ID, "https://github.com/test/cancel-repos-cancelled", "main", "feature-cancelled", RunRepoStatusCancelled)

	successBefore, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fx.Run.ID, RepoID: successRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(success) failed: %v", err)
	}
	failBefore, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fx.Run.ID, RepoID: failRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(fail) failed: %v", err)
	}
	cancelledBefore, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fx.Run.ID, RepoID: cancelledRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(cancelled) failed: %v", err)
	}

	affected, err := db.CancelActiveRunReposByRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("CancelActiveRunReposByRun() failed: %v", err)
	}
	if affected != 2 {
		t.Fatalf("CancelActiveRunReposByRun() affected=%d, want 2", affected)
	}

	queuedAfter, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fx.Run.ID, RepoID: fx.RunRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(queued after) failed: %v", err)
	}
	if queuedAfter.Status != RunRepoStatusCancelled {
		t.Fatalf("queued repo status=%q, want %q", queuedAfter.Status, RunRepoStatusCancelled)
	}
	if !queuedAfter.FinishedAt.Valid {
		t.Fatal("queued repo finished_at must be set")
	}

	runningAfter, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fx.Run.ID, RepoID: runningRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(running after) failed: %v", err)
	}
	if runningAfter.Status != RunRepoStatusCancelled {
		t.Fatalf("running repo status=%q, want %q", runningAfter.Status, RunRepoStatusCancelled)
	}
	if !runningAfter.FinishedAt.Valid {
		t.Fatal("running repo finished_at must be set")
	}

	successAfter, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fx.Run.ID, RepoID: successRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(success after) failed: %v", err)
	}
	if successAfter.Status != RunRepoStatusSuccess {
		t.Fatalf("success repo status=%q, want %q", successAfter.Status, RunRepoStatusSuccess)
	}
	if successAfter.FinishedAt != successBefore.FinishedAt {
		t.Fatalf("success repo finished_at changed: before=%v after=%v", successBefore.FinishedAt, successAfter.FinishedAt)
	}

	failAfter, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fx.Run.ID, RepoID: failRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(fail after) failed: %v", err)
	}
	if failAfter.Status != RunRepoStatusFail {
		t.Fatalf("fail repo status=%q, want %q", failAfter.Status, RunRepoStatusFail)
	}
	if failAfter.FinishedAt != failBefore.FinishedAt {
		t.Fatalf("fail repo finished_at changed: before=%v after=%v", failBefore.FinishedAt, failAfter.FinishedAt)
	}

	cancelledAfter, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fx.Run.ID, RepoID: cancelledRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(cancelled after) failed: %v", err)
	}
	if cancelledAfter.Status != RunRepoStatusCancelled {
		t.Fatalf("cancelled repo status=%q, want %q", cancelledAfter.Status, RunRepoStatusCancelled)
	}
	if cancelledAfter.FinishedAt != cancelledBefore.FinishedAt {
		t.Fatalf("cancelled repo finished_at changed: before=%v after=%v", cancelledBefore.FinishedAt, cancelledAfter.FinishedAt)
	}
}

func TestCancelActiveJobsByRun_TransitionsOnlyCreatedQueuedRunning(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-jobs-a", "main", "feature", []byte(`{"type":"cancel-jobs"}`))

	createdJob := createJobForCancelBulkTest(t, ctx, db, fx.Run.ID, fx.MigRepo.ID, fx.RunRepo.RepoBaseRef, "created", JobStatusCreated)
	queuedJob := createJobForCancelBulkTest(t, ctx, db, fx.Run.ID, fx.MigRepo.ID, fx.RunRepo.RepoBaseRef, "queued", JobStatusQueued)
	runningJob := createJobForCancelBulkTest(t, ctx, db, fx.Run.ID, fx.MigRepo.ID, fx.RunRepo.RepoBaseRef, "running", JobStatusQueued)
	setJobRunningForCancelBulkTest(t, ctx, db, runningJob.ID)

	if _, err := db.Pool().Exec(ctx, `UPDATE jobs SET started_at = now() - interval '3 seconds' WHERE id = $1`, runningJob.ID); err != nil {
		t.Fatalf("set running started_at failed: %v", err)
	}

	successJob := createJobForCancelBulkTest(t, ctx, db, fx.Run.ID, fx.MigRepo.ID, fx.RunRepo.RepoBaseRef, "success", JobStatusQueued)
	setJobRunningForCancelBulkTest(t, ctx, db, successJob.ID)
	if err := db.UpdateJobCompletion(ctx, UpdateJobCompletionParams{
		ID:       successJob.ID,
		Status:   JobStatusSuccess,
		ExitCode: ptrInt32ForCancelBulkTest(0),
	}); err != nil {
		t.Fatalf("UpdateJobCompletion(success) failed: %v", err)
	}

	failJob := createJobForCancelBulkTest(t, ctx, db, fx.Run.ID, fx.MigRepo.ID, fx.RunRepo.RepoBaseRef, "fail", JobStatusQueued)
	setJobRunningForCancelBulkTest(t, ctx, db, failJob.ID)
	if err := db.UpdateJobCompletion(ctx, UpdateJobCompletionParams{
		ID:       failJob.ID,
		Status:   JobStatusFail,
		ExitCode: ptrInt32ForCancelBulkTest(1),
	}); err != nil {
		t.Fatalf("UpdateJobCompletion(fail) failed: %v", err)
	}

	cancelledJob := createJobForCancelBulkTest(t, ctx, db, fx.Run.ID, fx.MigRepo.ID, fx.RunRepo.RepoBaseRef, "cancelled", JobStatusQueued)
	cancelledFinishedAt := pgtype.Timestamptz{Time: time.Now().UTC().Add(-1 * time.Minute), Valid: true}
	if err := db.UpdateJobStatus(ctx, UpdateJobStatusParams{
		ID:         cancelledJob.ID,
		Status:     JobStatusCancelled,
		StartedAt:  pgtype.Timestamptz{Time: cancelledFinishedAt.Time.Add(-1 * time.Second), Valid: true},
		FinishedAt: cancelledFinishedAt,
		DurationMs: 1000,
	}); err != nil {
		t.Fatalf("UpdateJobStatus(cancelled) failed: %v", err)
	}

	successBefore, err := db.GetJob(ctx, successJob.ID)
	if err != nil {
		t.Fatalf("GetJob(success before) failed: %v", err)
	}
	failBefore, err := db.GetJob(ctx, failJob.ID)
	if err != nil {
		t.Fatalf("GetJob(fail before) failed: %v", err)
	}
	cancelledBefore, err := db.GetJob(ctx, cancelledJob.ID)
	if err != nil {
		t.Fatalf("GetJob(cancelled before) failed: %v", err)
	}

	affected, err := db.CancelActiveJobsByRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("CancelActiveJobsByRun() failed: %v", err)
	}
	if affected != 3 {
		t.Fatalf("CancelActiveJobsByRun() affected=%d, want 3", affected)
	}

	createdAfter, err := db.GetJob(ctx, createdJob.ID)
	if err != nil {
		t.Fatalf("GetJob(created after) failed: %v", err)
	}
	if createdAfter.Status != JobStatusCancelled {
		t.Fatalf("created job status=%q, want %q", createdAfter.Status, JobStatusCancelled)
	}
	if !createdAfter.FinishedAt.Valid {
		t.Fatal("created job finished_at must be set")
	}
	if createdAfter.DurationMs != 0 {
		t.Fatalf("created job duration_ms=%d, want 0", createdAfter.DurationMs)
	}

	queuedAfter, err := db.GetJob(ctx, queuedJob.ID)
	if err != nil {
		t.Fatalf("GetJob(queued after) failed: %v", err)
	}
	if queuedAfter.Status != JobStatusCancelled {
		t.Fatalf("queued job status=%q, want %q", queuedAfter.Status, JobStatusCancelled)
	}
	if !queuedAfter.FinishedAt.Valid {
		t.Fatal("queued job finished_at must be set")
	}
	if queuedAfter.DurationMs != 0 {
		t.Fatalf("queued job duration_ms=%d, want 0", queuedAfter.DurationMs)
	}

	runningAfter, err := db.GetJob(ctx, runningJob.ID)
	if err != nil {
		t.Fatalf("GetJob(running after) failed: %v", err)
	}
	if runningAfter.Status != JobStatusCancelled {
		t.Fatalf("running job status=%q, want %q", runningAfter.Status, JobStatusCancelled)
	}
	if !runningAfter.FinishedAt.Valid {
		t.Fatal("running job finished_at must be set")
	}
	if runningAfter.DurationMs <= 0 {
		t.Fatalf("running job duration_ms=%d, want > 0", runningAfter.DurationMs)
	}

	successAfter, err := db.GetJob(ctx, successJob.ID)
	if err != nil {
		t.Fatalf("GetJob(success after) failed: %v", err)
	}
	if successAfter.Status != JobStatusSuccess {
		t.Fatalf("success job status=%q, want %q", successAfter.Status, JobStatusSuccess)
	}
	if successAfter.DurationMs != successBefore.DurationMs {
		t.Fatalf("success job duration_ms changed: before=%d after=%d", successBefore.DurationMs, successAfter.DurationMs)
	}

	failAfter, err := db.GetJob(ctx, failJob.ID)
	if err != nil {
		t.Fatalf("GetJob(fail after) failed: %v", err)
	}
	if failAfter.Status != JobStatusFail {
		t.Fatalf("fail job status=%q, want %q", failAfter.Status, JobStatusFail)
	}
	if failAfter.DurationMs != failBefore.DurationMs {
		t.Fatalf("fail job duration_ms changed: before=%d after=%d", failBefore.DurationMs, failAfter.DurationMs)
	}

	cancelledAfter, err := db.GetJob(ctx, cancelledJob.ID)
	if err != nil {
		t.Fatalf("GetJob(cancelled after) failed: %v", err)
	}
	if cancelledAfter.Status != JobStatusCancelled {
		t.Fatalf("cancelled job status=%q, want %q", cancelledAfter.Status, JobStatusCancelled)
	}
	if cancelledAfter.DurationMs != cancelledBefore.DurationMs {
		t.Fatalf("cancelled job duration_ms changed: before=%d after=%d", cancelledBefore.DurationMs, cancelledAfter.DurationMs)
	}
}

func TestCancelBulkQueries_AreScopedToRunID(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fxA := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-scope-a", "main", "feature-a", []byte(`{"type":"cancel-scope-a"}`))
	fxB := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-scope-b", "main", "feature-b", []byte(`{"type":"cancel-scope-b"}`))

	jobA := createJobForCancelBulkTest(t, ctx, db, fxA.Run.ID, fxA.MigRepo.ID, fxA.RunRepo.RepoBaseRef, "run-a-created", JobStatusCreated)
	jobB := createJobForCancelBulkTest(t, ctx, db, fxB.Run.ID, fxB.MigRepo.ID, fxB.RunRepo.RepoBaseRef, "run-b-created", JobStatusCreated)

	affectedRepos, err := db.CancelActiveRunReposByRun(ctx, fxA.Run.ID)
	if err != nil {
		t.Fatalf("CancelActiveRunReposByRun(run A) failed: %v", err)
	}
	if affectedRepos != 1 {
		t.Fatalf("CancelActiveRunReposByRun(run A) affected=%d, want 1", affectedRepos)
	}

	affectedJobs, err := db.CancelActiveJobsByRun(ctx, fxA.Run.ID)
	if err != nil {
		t.Fatalf("CancelActiveJobsByRun(run A) failed: %v", err)
	}
	if affectedJobs != 1 {
		t.Fatalf("CancelActiveJobsByRun(run A) affected=%d, want 1", affectedJobs)
	}

	runARepoAfter, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fxA.Run.ID, RepoID: fxA.RunRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(run A) failed: %v", err)
	}
	if runARepoAfter.Status != RunRepoStatusCancelled {
		t.Fatalf("run A repo status=%q, want %q", runARepoAfter.Status, RunRepoStatusCancelled)
	}

	runBRepoAfter, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fxB.Run.ID, RepoID: fxB.RunRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(run B) failed: %v", err)
	}
	if runBRepoAfter.Status != RunRepoStatusQueued {
		t.Fatalf("run B repo status=%q, want %q", runBRepoAfter.Status, RunRepoStatusQueued)
	}

	jobAAfter, err := db.GetJob(ctx, jobA.ID)
	if err != nil {
		t.Fatalf("GetJob(run A) failed: %v", err)
	}
	if jobAAfter.Status != JobStatusCancelled {
		t.Fatalf("run A job status=%q, want %q", jobAAfter.Status, JobStatusCancelled)
	}

	jobBAfter, err := db.GetJob(ctx, jobB.ID)
	if err != nil {
		t.Fatalf("GetJob(run B) failed: %v", err)
	}
	if jobBAfter.Status != JobStatusCreated {
		t.Fatalf("run B job status=%q, want %q", jobBAfter.Status, JobStatusCreated)
	}
}

func openStoreForCancelBulkTests(t *testing.T) (context.Context, Store) {
	t.Helper()

	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	t.Cleanup(db.Close)
	return ctx, db
}

func createRunRepoForCancelBulkTest(t *testing.T, ctx context.Context, db Store, modID types.MigID, runID types.RunID, repoURL, baseRef, targetRef string, status RunRepoStatus) RunRepo {
	t.Helper()

	repoID := types.NewMigRepoID()
	mr, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:        repoID,
		MigID:     modID,
		RepoUrl:   repoURL,
		BaseRef:   baseRef,
		TargetRef: targetRef,
	})
	if err != nil {
		t.Fatalf("CreateMigRepo(%s) failed: %v", repoURL, err)
	}

	rr, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
		MigID:         modID,
		RunID:         runID,
		RepoID:        mr.ID,
		RepoBaseRef:   mr.BaseRef,
		RepoTargetRef: mr.TargetRef,
	})
	if err != nil {
		t.Fatalf("CreateRunRepo(%s) failed: %v", repoURL, err)
	}

	if status != RunRepoStatusQueued {
		if err := db.UpdateRunRepoStatus(ctx, UpdateRunRepoStatusParams{
			RunID:  runID,
			RepoID: rr.RepoID,
			Status: status,
		}); err != nil {
			t.Fatalf("UpdateRunRepoStatus(%s -> %s) failed: %v", repoURL, status, err)
		}
	}

	out, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: runID, RepoID: rr.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(%s) failed: %v", repoURL, err)
	}
	return out
}

func createJobForCancelBulkTest(t *testing.T, ctx context.Context, db Store, runID types.RunID, repoID types.MigRepoID, repoBaseRef, name string, status JobStatus) Job {
	t.Helper()

	job, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: repoBaseRef,
		Attempt:     1,
		Name:        name,
		Status:      status,
		JobType:     "mod",
		JobImage:    "test-image",
		NextID:      nil,
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(%s) failed: %v", name, err)
	}
	return job
}

func setJobRunningForCancelBulkTest(t *testing.T, ctx context.Context, db Store, jobID types.JobID) {
	t.Helper()

	if err := db.UpdateJobStatus(ctx, UpdateJobStatusParams{
		ID:         jobID,
		Status:     JobStatusRunning,
		StartedAt:  pgtype.Timestamptz{},
		FinishedAt: pgtype.Timestamptz{},
		DurationMs: 0,
	}); err != nil {
		t.Fatalf("UpdateJobStatus(%s -> Running) failed: %v", jobID, err)
	}
}

func ptrInt32ForCancelBulkTest(v int32) *int32 {
	return &v
}
