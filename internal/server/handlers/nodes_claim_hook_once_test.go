package handlers

import (
	"context"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestResolveHookRuntimeDecision_NoLedgerRecordRunsHook(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	job := store.Job{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		JobType: domaintypes.JobTypeHook,
		Name:    "pre-gate-hook-000",
	}
	spec := []byte(`{"steps":[{"image":"test:latest"}],"hooks":["https://hooks.example.com/a.yaml"]}`)
	st := &jobStore{}
	st.hasHookOnceLedger.val = false

	got, err := resolveHookRuntimeDecision(context.Background(), st, job, spec, domaintypes.JobTypeHook)
	if err != nil {
		t.Fatalf("resolveHookRuntimeDecision() error: %v", err)
	}
	if got == nil {
		t.Fatal("resolveHookRuntimeDecision() returned nil decision")
	}
	if !got.HookShouldRun {
		t.Fatal("HookShouldRun=false, want true")
	}
	if got.HookOnceSkipMarked {
		t.Fatal("HookOnceSkipMarked=true, want false")
	}
	if len(got.HookHash) != 64 {
		t.Fatalf("HookHash length=%d, want 64", len(got.HookHash))
	}
	if !st.hasHookOnceLedger.called {
		t.Fatal("expected HasHookOnceLedger() to be called")
	}
	if st.getHookOnceLedger.called {
		t.Fatal("did not expect GetHookOnceLedger() when no ledger row exists")
	}
}

func TestResolveHookRuntimeDecision_LedgerSuccessSkipsAndMarksOnce(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	firstSuccessID := domaintypes.NewJobID()
	job := store.Job{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		JobType: domaintypes.JobTypeHook,
		Name:    "post-gate-hook-001",
	}
	spec := []byte(`{"steps":[{"image":"test:latest"}],"hooks":["https://hooks.example.com/a.yaml","https://hooks.example.com/b.yaml"]}`)
	st := &jobStore{}
	st.hasHookOnceLedger.val = true
	st.getHookOnceLedger.val = store.HooksOnce{
		RunID:             runID,
		RepoID:            repoID,
		HookHash:          strings.Repeat("a", 64),
		FirstSuccessJobID: &firstSuccessID,
		OnceSkipMarked:    false,
	}

	got, err := resolveHookRuntimeDecision(context.Background(), st, job, spec, domaintypes.JobTypeHook)
	if err != nil {
		t.Fatalf("resolveHookRuntimeDecision() error: %v", err)
	}
	if got == nil {
		t.Fatal("resolveHookRuntimeDecision() returned nil decision")
	}
	if got.HookShouldRun {
		t.Fatal("HookShouldRun=true, want false")
	}
	if !got.HookOnceSkipMarked {
		t.Fatal("HookOnceSkipMarked=false, want true")
	}
	if !st.getHookOnceLedger.called {
		t.Fatal("expected GetHookOnceLedger() to be called")
	}
}
