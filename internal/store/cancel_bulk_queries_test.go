package store

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCancelActiveRunsByWave_TransitionsOnlyQueuedRunning(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-repos-a", "main", []byte(`{"type":"cancel-repos"}`))

	runningRun := createRunForStoreTest(t, ctx, db, fx.Wave.ID, fx.Mig.ID, fx.Spec.ID, "https://github.com/test/cancel-repos-running", "feature-running", types.RunStatusRunning)
	successRun := createRunForStoreTest(t, ctx, db, fx.Wave.ID, fx.Mig.ID, fx.Spec.ID, "https://github.com/test/cancel-repos-success", "feature-success", types.RunStatusSuccess)
	failRun := createRunForStoreTest(t, ctx, db, fx.Wave.ID, fx.Mig.ID, fx.Spec.ID, "https://github.com/test/cancel-repos-fail", "feature-fail", types.RunStatusFail)
	cancelledRun := createRunForStoreTest(t, ctx, db, fx.Wave.ID, fx.Mig.ID, fx.Spec.ID, "https://github.com/test/cancel-repos-cancelled", "feature-cancelled", types.RunStatusCancelled)

	successBefore, err := db.GetRun(ctx, successRun.ID)
	if err != nil {
		t.Fatalf("GetRun(success) failed: %v", err)
	}
	failBefore, err := db.GetRun(ctx, failRun.ID)
	if err != nil {
		t.Fatalf("GetRun(fail) failed: %v", err)
	}
	cancelledBefore, err := db.GetRun(ctx, cancelledRun.ID)
	if err != nil {
		t.Fatalf("GetRun(cancelled) failed: %v", err)
	}

	affected, err := db.CancelActiveRunsByWave(ctx, fx.Wave.ID)
	if err != nil {
		t.Fatalf("CancelActiveRunsByWave() failed: %v", err)
	}
	if affected != 2 {
		t.Fatalf("CancelActiveRunsByWave() affected=%d, want 2", affected)
	}

	queuedAfter, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun(queued after) failed: %v", err)
	}
	if queuedAfter.Status != types.RunStatusCancelled {
		t.Fatalf("queued repo status=%q, want %q", queuedAfter.Status, types.RunStatusCancelled)
	}
	if !queuedAfter.FinishedAt.Valid {
		t.Fatal("queued repo finished_at must be set")
	}

	runningAfter, err := db.GetRun(ctx, runningRun.ID)
	if err != nil {
		t.Fatalf("GetRun(running after) failed: %v", err)
	}
	if runningAfter.Status != types.RunStatusCancelled {
		t.Fatalf("running repo status=%q, want %q", runningAfter.Status, types.RunStatusCancelled)
	}
	if !runningAfter.FinishedAt.Valid {
		t.Fatal("running repo finished_at must be set")
	}

	successAfter, err := db.GetRun(ctx, successRun.ID)
	if err != nil {
		t.Fatalf("GetRun(success after) failed: %v", err)
	}
	if successAfter.Status != types.RunStatusSuccess {
		t.Fatalf("success repo status=%q, want %q", successAfter.Status, types.RunStatusSuccess)
	}
	if successAfter.FinishedAt != successBefore.FinishedAt {
		t.Fatalf("success repo finished_at changed: before=%v after=%v", successBefore.FinishedAt, successAfter.FinishedAt)
	}

	failAfter, err := db.GetRun(ctx, failRun.ID)
	if err != nil {
		t.Fatalf("GetRun(fail after) failed: %v", err)
	}
	if failAfter.Status != types.RunStatusFail {
		t.Fatalf("fail repo status=%q, want %q", failAfter.Status, types.RunStatusFail)
	}
	if failAfter.FinishedAt != failBefore.FinishedAt {
		t.Fatalf("fail repo finished_at changed: before=%v after=%v", failBefore.FinishedAt, failAfter.FinishedAt)
	}

	cancelledAfter, err := db.GetRun(ctx, cancelledRun.ID)
	if err != nil {
		t.Fatalf("GetRun(cancelled after) failed: %v", err)
	}
	if cancelledAfter.Status != types.RunStatusCancelled {
		t.Fatalf("cancelled repo status=%q, want %q", cancelledAfter.Status, types.RunStatusCancelled)
	}
	if cancelledAfter.FinishedAt != cancelledBefore.FinishedAt {
		t.Fatalf("cancelled repo finished_at changed: before=%v after=%v", cancelledBefore.FinishedAt, cancelledAfter.FinishedAt)
	}
}

func TestCancelActiveJobsByRun_TransitionsOnlyCreatedQueuedRunning(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-jobs-a", "main", []byte(`{"type":"cancel-jobs"}`))

	createdJob := createJobForStoreTest(t, ctx, db, fx.Run.ID, fx.MigRepo.RepoID, fx.Run.RepoBaseRef, 1, "created", types.JobStatusCreated)
	queuedJob := createJobForStoreTest(t, ctx, db, fx.Run.ID, fx.MigRepo.RepoID, fx.Run.RepoBaseRef, 1, "queued", types.JobStatusQueued)
	runningJob := createJobForStoreTest(t, ctx, db, fx.Run.ID, fx.MigRepo.RepoID, fx.Run.RepoBaseRef, 1, "running", types.JobStatusQueued)
	setJobRunningForCancelBulkTest(t, ctx, db, runningJob.ID)

	if _, err := db.Pool().Exec(ctx, `UPDATE jobs SET started_at = now() - interval '3 seconds' WHERE id = $1`, runningJob.ID); err != nil {
		t.Fatalf("set running started_at failed: %v", err)
	}

	successJob := createJobForStoreTest(t, ctx, db, fx.Run.ID, fx.MigRepo.RepoID, fx.Run.RepoBaseRef, 1, "success", types.JobStatusQueued)
	setJobRunningForCancelBulkTest(t, ctx, db, successJob.ID)
	if err := db.UpdateJobCompletion(ctx, UpdateJobCompletionParams{
		ID:       successJob.ID,
		Status:   types.JobStatusSuccess,
		ExitCode: ptrInt32ForCancelBulkTest(0),
	}); err != nil {
		t.Fatalf("UpdateJobCompletion(success) failed: %v", err)
	}

	failJob := createJobForStoreTest(t, ctx, db, fx.Run.ID, fx.MigRepo.RepoID, fx.Run.RepoBaseRef, 1, "fail", types.JobStatusQueued)
	setJobRunningForCancelBulkTest(t, ctx, db, failJob.ID)
	if err := db.UpdateJobCompletion(ctx, UpdateJobCompletionParams{
		ID:       failJob.ID,
		Status:   types.JobStatusFail,
		ExitCode: ptrInt32ForCancelBulkTest(1),
	}); err != nil {
		t.Fatalf("UpdateJobCompletion(fail) failed: %v", err)
	}

	cancelledJob := createJobForStoreTest(t, ctx, db, fx.Run.ID, fx.MigRepo.RepoID, fx.Run.RepoBaseRef, 1, "cancelled", types.JobStatusQueued)
	cancelledFinishedAt := pgtype.Timestamptz{Time: time.Now().UTC().Add(-1 * time.Minute), Valid: true}
	if err := db.UpdateJobStatus(ctx, UpdateJobStatusParams{
		ID:         cancelledJob.ID,
		Status:     types.JobStatusCancelled,
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
	if createdAfter.Status != types.JobStatusCancelled {
		t.Fatalf("created job status=%q, want %q", createdAfter.Status, types.JobStatusCancelled)
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
	if queuedAfter.Status != types.JobStatusCancelled {
		t.Fatalf("queued job status=%q, want %q", queuedAfter.Status, types.JobStatusCancelled)
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
	if runningAfter.Status != types.JobStatusCancelled {
		t.Fatalf("running job status=%q, want %q", runningAfter.Status, types.JobStatusCancelled)
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
	if successAfter.Status != types.JobStatusSuccess {
		t.Fatalf("success job status=%q, want %q", successAfter.Status, types.JobStatusSuccess)
	}
	if successAfter.DurationMs != successBefore.DurationMs {
		t.Fatalf("success job duration_ms changed: before=%d after=%d", successBefore.DurationMs, successAfter.DurationMs)
	}

	failAfter, err := db.GetJob(ctx, failJob.ID)
	if err != nil {
		t.Fatalf("GetJob(fail after) failed: %v", err)
	}
	if failAfter.Status != types.JobStatusFail {
		t.Fatalf("fail job status=%q, want %q", failAfter.Status, types.JobStatusFail)
	}
	if failAfter.DurationMs != failBefore.DurationMs {
		t.Fatalf("fail job duration_ms changed: before=%d after=%d", failBefore.DurationMs, failAfter.DurationMs)
	}

	cancelledAfter, err := db.GetJob(ctx, cancelledJob.ID)
	if err != nil {
		t.Fatalf("GetJob(cancelled after) failed: %v", err)
	}
	if cancelledAfter.Status != types.JobStatusCancelled {
		t.Fatalf("cancelled job status=%q, want %q", cancelledAfter.Status, types.JobStatusCancelled)
	}
	if cancelledAfter.DurationMs != cancelledBefore.DurationMs {
		t.Fatalf("cancelled job duration_ms changed: before=%d after=%d", cancelledBefore.DurationMs, cancelledAfter.DurationMs)
	}
}

func TestCancelBulkQueries_AreScopedToRunID(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fxA := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-scope-a", "main", []byte(`{"type":"cancel-scope-a"}`))
	fxB := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-scope-b", "main", []byte(`{"type":"cancel-scope-b"}`))

	jobA := createJobForStoreTest(t, ctx, db, fxA.Run.ID, fxA.MigRepo.RepoID, fxA.Run.RepoBaseRef, 1, "run-a-created", types.JobStatusCreated)
	jobB := createJobForStoreTest(t, ctx, db, fxB.Run.ID, fxB.MigRepo.RepoID, fxB.Run.RepoBaseRef, 1, "run-b-created", types.JobStatusCreated)

	affectedRepos, err := db.CancelActiveRunsByWave(ctx, fxA.Wave.ID)
	if err != nil {
		t.Fatalf("CancelActiveRunsByWave(run A) failed: %v", err)
	}
	if affectedRepos != 1 {
		t.Fatalf("CancelActiveRunsByWave(run A) affected=%d, want 1", affectedRepos)
	}

	affectedJobs, err := db.CancelActiveJobsByRun(ctx, fxA.Run.ID)
	if err != nil {
		t.Fatalf("CancelActiveJobsByRun(run A) failed: %v", err)
	}
	if affectedJobs != 1 {
		t.Fatalf("CancelActiveJobsByRun(run A) affected=%d, want 1", affectedJobs)
	}

	runARepoAfter, err := db.GetRun(ctx, fxA.Run.ID)
	if err != nil {
		t.Fatalf("GetRun(run A) failed: %v", err)
	}
	if runARepoAfter.Status != types.RunStatusCancelled {
		t.Fatalf("run A repo status=%q, want %q", runARepoAfter.Status, types.RunStatusCancelled)
	}

	runBRepoAfter, err := db.GetRun(ctx, fxB.Run.ID)
	if err != nil {
		t.Fatalf("GetRun(run B) failed: %v", err)
	}
	if runBRepoAfter.Status != types.RunStatusQueued {
		t.Fatalf("run B repo status=%q, want %q", runBRepoAfter.Status, types.RunStatusQueued)
	}

	jobAAfter, err := db.GetJob(ctx, jobA.ID)
	if err != nil {
		t.Fatalf("GetJob(run A) failed: %v", err)
	}
	if jobAAfter.Status != types.JobStatusCancelled {
		t.Fatalf("run A job status=%q, want %q", jobAAfter.Status, types.JobStatusCancelled)
	}

	jobBAfter, err := db.GetJob(ctx, jobB.ID)
	if err != nil {
		t.Fatalf("GetJob(run B) failed: %v", err)
	}
	if jobBAfter.Status != types.JobStatusCreated {
		t.Fatalf("run B job status=%q, want %q", jobBAfter.Status, types.JobStatusCreated)
	}
}

func openStoreForCancelBulkTests(t *testing.T) (context.Context, Store) {
	t.Helper()
	return newTestStore(t)
}

func setJobRunningForCancelBulkTest(t *testing.T, ctx context.Context, db Store, jobID types.JobID) {
	t.Helper()

	if err := db.UpdateJobStatus(ctx, UpdateJobStatusParams{
		ID:         jobID,
		Status:     types.JobStatusRunning,
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
