package store

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

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

	modID := types.NewMigID()
	mig, err := db.CreateMig(ctx, CreateMigParams{
		ID:        modID,
		Name:      "test-mig-" + modID.String(),
		SpecID:    &specID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig() failed: %v", err)
	}

	modRepoID := types.NewMigRepoID()
	modRepo, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:        modRepoID,
		MigID:     modID,
		RepoUrl:   repoURL,
		BaseRef:   baseRef,
		TargetRef: targetRef,
	})
	if err != nil {
		t.Fatalf("CreateMigRepo() failed: %v", err)
	}

	runID := types.NewRunID()
	run, err := db.CreateRun(ctx, CreateRunParams{
		ID:        runID,
		MigID:     modID,
		SpecID:    specID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}

	runRepo, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
		MigID:         modID,
		RunID:         runID,
		RepoID:        modRepoID,
		RepoBaseRef:   baseRef,
		RepoTargetRef: targetRef,
	})
	if err != nil {
		t.Fatalf("CreateRunRepo() failed: %v", err)
	}

	return v1Fixture{
		Mig:     mig,
		Spec:    spec,
		MigRepo: modRepo,
		Run:     run,
		RunRepo: runRepo,
	}
}
