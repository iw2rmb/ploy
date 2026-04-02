package store

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// createRunRepoForStoreTest creates a MigRepo + RunRepo under migID/runID with
// the given repoURL, baseRef, targetRef, and initial status. If status is
// RunRepoStatusQueued the status update call is skipped (Queued is the default).
// Used by cancel_bulk_queries_test.go and stale_recovery_queries_test.go to
// avoid duplicate fixture helpers.
func createRunRepoForStoreTest(
	t *testing.T,
	ctx context.Context,
	db Store,
	migID types.MigID,
	runID types.RunID,
	repoURL, baseRef, targetRef string,
	status types.RunRepoStatus,
) RunRepo {
	t.Helper()

	repoID := types.NewMigRepoID()
	mr, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:        repoID,
		MigID:     migID,
		Url:       repoURL,
		BaseRef:   baseRef,
		TargetRef: targetRef,
	})
	if err != nil {
		t.Fatalf("CreateMigRepo(%s) failed: %v", repoURL, err)
	}

	rr, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
		MigID:           migID,
		RunID:           runID,
		RepoID:          mr.RepoID,
		RepoBaseRef:     mr.BaseRef,
		RepoTargetRef:   mr.TargetRef,
		SourceCommitSha: "0123456789abcdef0123456789abcdef01234567",
		RepoSha0:        "0123456789abcdef0123456789abcdef01234567",
	})
	if err != nil {
		t.Fatalf("CreateRunRepo(%s) failed: %v", repoURL, err)
	}

	if status != types.RunRepoStatusQueued {
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

// createJobForStoreTest creates a job with the given attempt, name, and initial
// status. Used by cancel_bulk_queries_test.go and stale_recovery_queries_test.go
// to avoid duplicate fixture helpers.
func createJobForStoreTest(
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

type v1Fixture struct {
	Mig     Mig
	Spec    Spec
	MigRepo MigRepo
	Run     Run
	RunRepo RunRepo
}

func newV1Fixture(t *testing.T, ctx context.Context, db Store, repoURL, baseRef, targetRef string, specJSON []byte) v1Fixture {
	t.Helper()

	createdBy := "test-user"

	specID := types.NewSpecID()
	spec, err := db.CreateSpec(ctx, CreateSpecParams{
		ID:        specID,
		Name:      "test-spec",
		Spec:      specJSON,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateSpec() failed: %v", err)
	}

	migID := types.NewMigID()
	mig, err := db.CreateMig(ctx, CreateMigParams{
		ID:        migID,
		Name:      "test-mig-" + migID.String(),
		SpecID:    &specID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig() failed: %v", err)
	}

	migRepoID := types.NewMigRepoID()
	migRepo, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:        migRepoID,
		MigID:     migID,
		Url:       repoURL,
		BaseRef:   baseRef,
		TargetRef: targetRef,
	})
	if err != nil {
		t.Fatalf("CreateMigRepo() failed: %v", err)
	}

	runID := types.NewRunID()
	run, err := db.CreateRun(ctx, CreateRunParams{
		ID:        runID,
		MigID:     migID,
		SpecID:    specID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}

	runRepo, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
		MigID:           migID,
		RunID:           runID,
		RepoID:          migRepo.RepoID,
		RepoBaseRef:     baseRef,
		RepoTargetRef:   targetRef,
		SourceCommitSha: "0123456789abcdef0123456789abcdef01234567",
		RepoSha0:        "0123456789abcdef0123456789abcdef01234567",
	})
	if err != nil {
		t.Fatalf("CreateRunRepo() failed: %v", err)
	}

	return v1Fixture{
		Mig:     mig,
		Spec:    spec,
		MigRepo: migRepo,
		Run:     run,
		RunRepo: runRepo,
	}
}
