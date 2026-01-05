package store

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

type v1Fixture struct {
	Mod     Mod
	Spec    Spec
	ModRepo ModRepo
	Run     Run
	RunRepo RunRepo
}

func newV1Fixture(t *testing.T, ctx context.Context, db Store, repoURL, baseRef, targetRef string, specJSON []byte) v1Fixture {
	t.Helper()

	createdBy := "test-user"

	specID := types.NewSpecID().String()
	spec, err := db.CreateSpec(ctx, CreateSpecParams{
		ID:        specID,
		Name:      "test-spec",
		Spec:      specJSON,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateSpec() failed: %v", err)
	}

	modID := types.NewModID().String()
	mod, err := db.CreateMod(ctx, CreateModParams{
		ID:        modID,
		Name:      "test-mod-" + modID,
		SpecID:    &specID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMod() failed: %v", err)
	}

	modRepoID := types.NewModRepoID().String()
	modRepo, err := db.CreateModRepo(ctx, CreateModRepoParams{
		ID:        modRepoID,
		ModID:     modID,
		RepoUrl:   repoURL,
		BaseRef:   baseRef,
		TargetRef: targetRef,
	})
	if err != nil {
		t.Fatalf("CreateModRepo() failed: %v", err)
	}

	runID := types.NewRunID().String()
	run, err := db.CreateRun(ctx, CreateRunParams{
		ID:        runID,
		ModID:     modID,
		SpecID:    specID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}

	runRepo, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
		ModID:         modID,
		RunID:         runID,
		RepoID:        modRepoID,
		RepoBaseRef:   baseRef,
		RepoTargetRef: targetRef,
	})
	if err != nil {
		t.Fatalf("CreateRunRepo() failed: %v", err)
	}

	return v1Fixture{
		Mod:     mod,
		Spec:    spec,
		ModRepo: modRepo,
		Run:     run,
		RunRepo: runRepo,
	}
}
