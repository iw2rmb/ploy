package store

import (
	"fmt"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestCancelRun_CancelsRunAndActiveJobs(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-run-a", "main", []byte(`{"type":"cancel-run"}`))

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

	if err := db.CancelRun(ctx, fx.Run.ID); err != nil {
		t.Fatalf("CancelRun() failed: %v", err)
	}

	runAfter, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun(after) failed: %v", err)
	}
	if runAfter.Status != types.RunStatusCancelled {
		t.Fatalf("run status=%q, want %q", runAfter.Status, types.RunStatusCancelled)
	}
	if !runAfter.FinishedAt.Valid {
		t.Fatal("run finished_at must be set")
	}

	runningAfter, err := db.GetJob(ctx, runningJob.ID)
	if err != nil {
		t.Fatalf("GetJob(running after) failed: %v", err)
	}
	if runningAfter.Status != types.JobStatusCancelled {
		t.Fatalf("running job status=%q, want %q", runningAfter.Status, types.JobStatusCancelled)
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
}

func TestCancelRun_RollsBackOnFailure(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-run-rollback", "main", []byte(`{"type":"cancel-run-rollback"}`))
	if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{ID: fx.Run.ID, Status: types.RunStatusRunning}); err != nil {
		t.Fatalf("UpdateRunStatus(running) failed: %v", err)
	}
	job := createJobForStoreTest(t, ctx, db, fx.Run.ID, fx.MigRepo.RepoID, fx.Run.RepoBaseRef, 1, "created", types.JobStatusCreated)

	suffix := strings.NewReplacer("-", "_").Replace(strings.ToLower(types.NewNodeKey()))
	fnName := fmt.Sprintf("test_cancel_run_fail_fn_%s", suffix)
	trName := fmt.Sprintf("test_cancel_run_fail_tr_%s", suffix)

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

	err := db.CancelRun(ctx, fx.Run.ID)
	if err == nil {
		t.Fatal("expected CancelRun() to fail")
	}
	if !strings.Contains(err.Error(), "cancel active jobs") {
		t.Fatalf("error=%q, expected context about cancel active jobs", err.Error())
	}

	runAfter, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun(after) failed: %v", err)
	}
	if runAfter.Status != types.RunStatusRunning {
		t.Fatalf("run status=%q, want %q after rollback", runAfter.Status, types.RunStatusRunning)
	}

	jobAfter, err := db.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob(after) failed: %v", err)
	}
	if jobAfter.Status != types.JobStatusCreated {
		t.Fatalf("job status=%q, want %q after rollback", jobAfter.Status, types.JobStatusCreated)
	}
}

func TestCancelRun_IsScopedToRunID(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fxA := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-run-scope-a", "main", []byte(`{"type":"cancel-run-scope-a"}`))
	fxB := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-run-scope-b", "main", []byte(`{"type":"cancel-run-scope-b"}`))

	jobA := createJobForStoreTest(t, ctx, db, fxA.Run.ID, fxA.MigRepo.RepoID, fxA.Run.RepoBaseRef, 1, "a-created", types.JobStatusCreated)
	jobB := createJobForStoreTest(t, ctx, db, fxB.Run.ID, fxB.MigRepo.RepoID, fxB.Run.RepoBaseRef, 1, "b-created", types.JobStatusCreated)

	if err := db.CancelRun(ctx, fxA.Run.ID); err != nil {
		t.Fatalf("CancelRun(run A) failed: %v", err)
	}

	runAAfter, err := db.GetRun(ctx, fxA.Run.ID)
	if err != nil {
		t.Fatalf("GetRun(run A) failed: %v", err)
	}
	if runAAfter.Status != types.RunStatusCancelled {
		t.Fatalf("run A status=%q, want %q", runAAfter.Status, types.RunStatusCancelled)
	}

	runBAfter, err := db.GetRun(ctx, fxB.Run.ID)
	if err != nil {
		t.Fatalf("GetRun(run B) failed: %v", err)
	}
	if runBAfter.Status != types.RunStatusQueued {
		t.Fatalf("run B status=%q, want %q", runBAfter.Status, types.RunStatusQueued)
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

func TestCancelRun_CancelledRunIsIdempotent(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/test/cancel-run-idempotent", "main", []byte(`{"type":"cancel-run-idempotent"}`))
	if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{ID: fx.Run.ID, Status: types.RunStatusCancelled}); err != nil {
		t.Fatalf("UpdateRunStatus(cancelled) failed: %v", err)
	}

	if err := db.CancelRun(ctx, fx.Run.ID); err != nil {
		t.Fatalf("CancelRun(cancelled run) failed: %v", err)
	}

	runAfter, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun(after) failed: %v", err)
	}
	if runAfter.Status != types.RunStatusCancelled {
		t.Fatalf("run status=%q, want %q", runAfter.Status, types.RunStatusCancelled)
	}
}
