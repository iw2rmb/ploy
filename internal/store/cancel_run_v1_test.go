package store

import (
	"fmt"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestCancelRunV1_CancelsRunAndActiveWork(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-run-v1-a", "main", "feature-a", []byte(`{"type":"cancel-run-v1"}`))

	runningRepo := createRunRepoForCancelBulkTest(t, ctx, db, fx.Mig.ID, fx.Run.ID, "https://github.com/test/cancel-run-v1-running", "main", "feature-running", RunRepoStatusRunning)
	successRepo := createRunRepoForCancelBulkTest(t, ctx, db, fx.Mig.ID, fx.Run.ID, "https://github.com/test/cancel-run-v1-success", "main", "feature-success", RunRepoStatusSuccess)

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

	if err := db.CancelRunV1(ctx, fx.Run.ID); err != nil {
		t.Fatalf("CancelRunV1() failed: %v", err)
	}

	runAfter, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun(after) failed: %v", err)
	}
	if runAfter.Status != RunStatusCancelled {
		t.Fatalf("run status=%q, want %q", runAfter.Status, RunStatusCancelled)
	}
	if !runAfter.FinishedAt.Valid {
		t.Fatal("run finished_at must be set")
	}

	queuedRepoAfter, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fx.Run.ID, RepoID: fx.RunRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(queued after) failed: %v", err)
	}
	if queuedRepoAfter.Status != RunRepoStatusCancelled {
		t.Fatalf("queued repo status=%q, want %q", queuedRepoAfter.Status, RunRepoStatusCancelled)
	}

	runningRepoAfter, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fx.Run.ID, RepoID: runningRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(running after) failed: %v", err)
	}
	if runningRepoAfter.Status != RunRepoStatusCancelled {
		t.Fatalf("running repo status=%q, want %q", runningRepoAfter.Status, RunRepoStatusCancelled)
	}

	successRepoAfter, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fx.Run.ID, RepoID: successRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(success after) failed: %v", err)
	}
	if successRepoAfter.Status != RunRepoStatusSuccess {
		t.Fatalf("success repo status=%q, want %q", successRepoAfter.Status, RunRepoStatusSuccess)
	}

	createdAfter, err := db.GetJob(ctx, createdJob.ID)
	if err != nil {
		t.Fatalf("GetJob(created after) failed: %v", err)
	}
	if createdAfter.Status != JobStatusCancelled {
		t.Fatalf("created job status=%q, want %q", createdAfter.Status, JobStatusCancelled)
	}

	queuedAfter, err := db.GetJob(ctx, queuedJob.ID)
	if err != nil {
		t.Fatalf("GetJob(queued after) failed: %v", err)
	}
	if queuedAfter.Status != JobStatusCancelled {
		t.Fatalf("queued job status=%q, want %q", queuedAfter.Status, JobStatusCancelled)
	}

	runningAfter, err := db.GetJob(ctx, runningJob.ID)
	if err != nil {
		t.Fatalf("GetJob(running after) failed: %v", err)
	}
	if runningAfter.Status != JobStatusCancelled {
		t.Fatalf("running job status=%q, want %q", runningAfter.Status, JobStatusCancelled)
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
}

func TestCancelRunV1_RollsBackOnFailure(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-run-v1-rollback", "main", "feature-rollback", []byte(`{"type":"cancel-run-v1-rollback"}`))
	job := createJobForCancelBulkTest(t, ctx, db, fx.Run.ID, fx.MigRepo.ID, fx.RunRepo.RepoBaseRef, "created", JobStatusCreated)

	// Inject DB error during CancelActiveJobsByRun so earlier updates must roll back.
	suffix := strings.ToLower(types.NewNodeKey())
	fnName := fmt.Sprintf("test_cancel_run_v1_fail_fn_%s", suffix)
	trName := fmt.Sprintf("test_cancel_run_v1_fail_tr_%s", suffix)

	createFnSQL := fmt.Sprintf(`
CREATE FUNCTION ploy.%s() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'forced cancel jobs failure';
END;
$$;
`, fnName)
	if _, err := db.Pool().Exec(ctx, createFnSQL); err != nil {
		t.Fatalf("create trigger function failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool().Exec(ctx, fmt.Sprintf(`DROP FUNCTION IF EXISTS ploy.%s() CASCADE`, fnName))
	})

	createTriggerSQL := fmt.Sprintf(`
CREATE TRIGGER %s
BEFORE UPDATE ON ploy.jobs
FOR EACH ROW
WHEN (OLD.id = '%s' AND NEW.status = 'Cancelled')
EXECUTE FUNCTION ploy.%s();
`, trName, job.ID, fnName)
	if _, err := db.Pool().Exec(ctx, createTriggerSQL); err != nil {
		t.Fatalf("create trigger failed: %v", err)
	}

	err := db.CancelRunV1(ctx, fx.Run.ID)
	if err == nil {
		t.Fatal("expected CancelRunV1() to fail")
	}

	runAfter, getRunErr := db.GetRun(ctx, fx.Run.ID)
	if getRunErr != nil {
		t.Fatalf("GetRun(after) failed: %v", getRunErr)
	}
	if runAfter.Status != RunStatusStarted {
		t.Fatalf("run status=%q, want %q after rollback", runAfter.Status, RunStatusStarted)
	}

	repoAfter, getRepoErr := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fx.Run.ID, RepoID: fx.RunRepo.RepoID})
	if getRepoErr != nil {
		t.Fatalf("GetRunRepo(after) failed: %v", getRepoErr)
	}
	if repoAfter.Status != RunRepoStatusQueued {
		t.Fatalf("repo status=%q, want %q after rollback", repoAfter.Status, RunRepoStatusQueued)
	}

	jobAfter, getJobErr := db.GetJob(ctx, job.ID)
	if getJobErr != nil {
		t.Fatalf("GetJob(after) failed: %v", getJobErr)
	}
	if jobAfter.Status != JobStatusCreated {
		t.Fatalf("job status=%q, want %q after rollback", jobAfter.Status, JobStatusCreated)
	}
}

func TestCancelRunV1_IsScopedToRunID(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fxA := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-run-v1-scope-a", "main", "feature-a", []byte(`{"type":"cancel-run-v1-scope-a"}`))
	fxB := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-run-v1-scope-b", "main", "feature-b", []byte(`{"type":"cancel-run-v1-scope-b"}`))

	jobA := createJobForCancelBulkTest(t, ctx, db, fxA.Run.ID, fxA.MigRepo.ID, fxA.RunRepo.RepoBaseRef, "a-created", JobStatusCreated)
	jobB := createJobForCancelBulkTest(t, ctx, db, fxB.Run.ID, fxB.MigRepo.ID, fxB.RunRepo.RepoBaseRef, "b-created", JobStatusCreated)

	if err := db.CancelRunV1(ctx, fxA.Run.ID); err != nil {
		t.Fatalf("CancelRunV1(run A) failed: %v", err)
	}

	runAAfter, err := db.GetRun(ctx, fxA.Run.ID)
	if err != nil {
		t.Fatalf("GetRun(run A) failed: %v", err)
	}
	if runAAfter.Status != RunStatusCancelled {
		t.Fatalf("run A status=%q, want %q", runAAfter.Status, RunStatusCancelled)
	}

	runBAfter, err := db.GetRun(ctx, fxB.Run.ID)
	if err != nil {
		t.Fatalf("GetRun(run B) failed: %v", err)
	}
	if runBAfter.Status != RunStatusStarted {
		t.Fatalf("run B status=%q, want %q", runBAfter.Status, RunStatusStarted)
	}

	repoAAfter, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fxA.Run.ID, RepoID: fxA.RunRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(run A) failed: %v", err)
	}
	if repoAAfter.Status != RunRepoStatusCancelled {
		t.Fatalf("run A repo status=%q, want %q", repoAAfter.Status, RunRepoStatusCancelled)
	}

	repoBAfter, err := db.GetRunRepo(ctx, GetRunRepoParams{RunID: fxB.Run.ID, RepoID: fxB.RunRepo.RepoID})
	if err != nil {
		t.Fatalf("GetRunRepo(run B) failed: %v", err)
	}
	if repoBAfter.Status != RunRepoStatusQueued {
		t.Fatalf("run B repo status=%q, want %q", repoBAfter.Status, RunRepoStatusQueued)
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

func TestCancelRunV1_CancelledRunIsIdempotent(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-run-v1-idempotent", "main", "feature-idempotent", []byte(`{"type":"cancel-run-v1-idempotent"}`))
	if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{ID: fx.Run.ID, Status: RunStatusCancelled}); err != nil {
		t.Fatalf("UpdateRunStatus(cancelled) failed: %v", err)
	}

	if err := db.CancelRunV1(ctx, fx.Run.ID); err != nil {
		t.Fatalf("CancelRunV1(cancelled run) failed: %v", err)
	}

	runAfter, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun(after) failed: %v", err)
	}
	if runAfter.Status != RunStatusCancelled {
		t.Fatalf("run status=%q, want %q", runAfter.Status, RunStatusCancelled)
	}
}

func TestCancelRunV1_RollbackErrorHasContext(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-run-v1-errctx", "main", "feature-errctx", []byte(`{"type":"cancel-run-v1-errctx"}`))
	job := createJobForCancelBulkTest(t, ctx, db, fx.Run.ID, fx.MigRepo.ID, fx.RunRepo.RepoBaseRef, "created", JobStatusCreated)

	suffix := strings.ToLower(types.NewNodeKey())
	fnName := fmt.Sprintf("test_cancel_run_v1_fail_jobs_ctx_fn_%s", suffix)
	trName := fmt.Sprintf("test_cancel_run_v1_fail_jobs_ctx_tr_%s", suffix)

	createFnSQL := fmt.Sprintf(`
CREATE FUNCTION ploy.%s() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'forced cancel jobs failure for context test';
END;
$$;
`, fnName)
	if _, err := db.Pool().Exec(ctx, createFnSQL); err != nil {
		t.Fatalf("create trigger function failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool().Exec(ctx, fmt.Sprintf(`DROP FUNCTION IF EXISTS ploy.%s() CASCADE`, fnName))
	})

	createTriggerSQL := fmt.Sprintf(`
CREATE TRIGGER %s
BEFORE UPDATE ON ploy.jobs
FOR EACH ROW
WHEN (OLD.id = '%s' AND NEW.status = 'Cancelled')
EXECUTE FUNCTION ploy.%s();
`, trName, job.ID, fnName)
	if _, err := db.Pool().Exec(ctx, createTriggerSQL); err != nil {
		t.Fatalf("create trigger failed: %v", err)
	}

	err := db.CancelRunV1(ctx, fx.Run.ID)
	if err == nil {
		t.Fatal("expected CancelRunV1() to fail")
	}
	if !strings.Contains(err.Error(), "cancel active jobs") {
		t.Fatalf("error=%q, expected context about cancel active jobs", err.Error())
	}
}
