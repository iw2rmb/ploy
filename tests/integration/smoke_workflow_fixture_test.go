package integration

import (
	"context"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type v1RunFixture struct {
	Spec    store.Spec
	Mod     store.Mig
	ModRepo store.MigRepo
	Run     store.Run
	RunRepo store.RunRepo
}

func newV1RunFixture(t *testing.T, ctx context.Context, db store.Store, repoURL, baseRef, targetRef string, specJSON []byte) v1RunFixture {
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

	modID := domaintypes.NewMigID()
	mig, err := db.CreateMig(ctx, store.CreateMigParams{
		ID:        modID,
		Name:      "smoke-" + modID.String(),
		SpecID:    &spec.ID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig() failed: %v", err)
	}

	modRepoID := domaintypes.NewMigRepoID()
	modRepo, err := db.CreateMigRepo(ctx, store.CreateMigRepoParams{
		ID:        modRepoID,
		MigID:     modID,
		Url:       repoURL,
		BaseRef:   baseRef,
		TargetRef: targetRef,
	})
	if err != nil {
		t.Fatalf("CreateMigRepo() failed: %v", err)
	}

	runID := domaintypes.NewRunID()
	run, err := db.CreateRun(ctx, store.CreateRunParams{
		ID:        runID,
		MigID:     modID,
		SpecID:    spec.ID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}

	runRepo, err := db.CreateRunRepo(ctx, store.CreateRunRepoParams{
		MigID:         modID,
		RunID:         run.ID,
		RepoID:        modRepo.RepoID,
		RepoBaseRef:   baseRef,
		RepoTargetRef: targetRef,
	})
	if err != nil {
		t.Fatalf("CreateRunRepo() failed: %v", err)
	}

	return v1RunFixture{
		Spec:    spec,
		Mod:     mig,
		ModRepo: modRepo,
		Run:     run,
		RunRepo: runRepo,
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
