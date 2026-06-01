package integration

import (
	"context"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type v1RunFixture struct {
	Spec    store.Spec
	Mig     store.Mig
	MigRepo store.MigRepo
	Wave    store.Wave
	Run     store.Run
}

func newV1RunFixture(t *testing.T, ctx context.Context, db store.Store, repoURL, baseRef string, specJSON []byte) v1RunFixture {
	t.Helper()

	createdBy := "smoke-test"

	specID := domaintypes.NewSpecID()
	spec, err := db.CreateSpec(ctx, store.CreateSpecParams{
		ID:        specID,
		Name:      "smoke-workflow",
		Spec:      specJSON,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateSpec() failed: %v", err)
	}

	migID := domaintypes.NewMigID()
	mig, err := db.CreateMig(ctx, store.CreateMigParams{
		ID:        migID,
		Name:      "smoke-" + migID.String(),
		SpecID:    &spec.ID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig() failed: %v", err)
	}

	migRepoID := domaintypes.NewMigRepoID()
	migRepo, err := db.CreateMigRepo(ctx, store.CreateMigRepoParams{
		ID:      migRepoID,
		MigID:   migID,
		Url:     repoURL,
		BaseRef: baseRef,
	})
	if err != nil {
		t.Fatalf("CreateMigRepo() failed: %v", err)
	}

	runID := domaintypes.NewRunID()
	waveID := domaintypes.WaveID(runID.String())
	wave, err := db.CreateWave(ctx, store.CreateWaveParams{
		ID:        waveID,
		MigID:     migID,
		SpecID:    spec.ID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateWave() failed: %v", err)
	}

	run, err := db.CreateRun(ctx, store.CreateRunParams{
		ID:              runID,
		WaveID:          wave.ID,
		MigID:           migID,
		SpecID:          spec.ID,
		RepoID:          migRepo.RepoID,
		RepoBaseRef:     baseRef,
		SourceCommitSha: "0123456789abcdef0123456789abcdef01234567",
		RepoSha0:        "0123456789abcdef0123456789abcdef01234567",
		CreatedBy:       &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}

	return v1RunFixture{
		Spec:    spec,
		Mig:     mig,
		MigRepo: migRepo,
		Wave:    wave,
		Run:     run,
	}
}

// TestSmokeWorkflow_EndToEnd validates a complete workflow combining multiple operations:
// 1. Create run (queued)
// 2. Create jobs (build, test, deploy)
// 3. Append logs across jobs
// 4. Generate diffs
// 5. Create events
// 6. Update run status to completed
// 7. Verify all data is correctly persisted and retrievable
//
// This test simulates the critical path through the system from run creation
// to completion, validating database operations, foreign key relationships,
// and query correctness.
//
