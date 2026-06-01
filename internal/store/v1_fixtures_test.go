package store

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

const testSHA = "0123456789abcdef0123456789abcdef01234567"

func createRunForStoreTest(
	t *testing.T,
	ctx context.Context,
	db Store,
	waveID types.WaveID,
	migID types.MigID,
	specID types.SpecID,
	repoURL, baseRef string,
	status types.RunStatus,
) Run {
	t.Helper()

	migRepoID := types.NewMigRepoID()
	mr, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:      migRepoID,
		MigID:   migID,
		Url:     repoURL,
		BaseRef: baseRef,
	})
	if err != nil {
		t.Fatalf("CreateMigRepo(%s) failed: %v", repoURL, err)
	}

	run, err := db.CreateRun(ctx, CreateRunParams{
		ID:              types.NewRunID(),
		WaveID:          waveID,
		MigID:           migID,
		SpecID:          specID,
		RepoID:          mr.RepoID,
		RepoBaseRef:     mr.BaseRef,
		SourceCommitSha: testSHA,
		RepoSha0:        testSHA,
	})
	if err != nil {
		t.Fatalf("CreateRun(%s) failed: %v", repoURL, err)
	}

	if status != types.RunStatusQueued {
		if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{
			ID:     run.ID,
			Status: status,
		}); err != nil {
			t.Fatalf("UpdateRunStatus(%s -> %s) failed: %v", repoURL, status, err)
		}
	}

	out, err := db.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun(%s) failed: %v", repoURL, err)
	}
	return out
}

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
	Wave    Wave
	Run     Run
}

func newV1Fixture(t *testing.T, ctx context.Context, db Store, repoURL, baseRef string, specJSON []byte) v1Fixture {
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
		ID:      migRepoID,
		MigID:   migID,
		Url:     repoURL,
		BaseRef: baseRef,
	})
	if err != nil {
		t.Fatalf("CreateMigRepo() failed: %v", err)
	}

	waveID := types.NewWaveID()
	wave, err := db.CreateWave(ctx, CreateWaveParams{
		ID:        waveID,
		MigID:     migID,
		SpecID:    specID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateWave() failed: %v", err)
	}

	run, err := db.CreateRun(ctx, CreateRunParams{
		ID:              types.NewRunID(),
		WaveID:          waveID,
		MigID:           migID,
		SpecID:          specID,
		RepoID:          migRepo.RepoID,
		RepoBaseRef:     baseRef,
		SourceCommitSha: testSHA,
		RepoSha0:        testSHA,
		CreatedBy:       &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}

	return v1Fixture{
		Mig:     mig,
		Spec:    spec,
		MigRepo: migRepo,
		Wave:    wave,
		Run:     run,
	}
}
