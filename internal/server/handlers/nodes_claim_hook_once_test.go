package handlers

import (
	"context"
	"os"
	"path/filepath"
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
	source := writeHookManifest(t, `id: pre-a
once: true
steps:
  - image: test:latest
`)
	spec := []byte(`{"steps":[{"image":"test:latest"}],"hooks":["` + source + `"]}`)
	st := &jobStore{}
	st.hasHookOnceLedger.val = false
	st.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: job.Attempt,
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
	}}

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
	sourceA := writeHookManifest(t, `id: post-a
once: false
steps:
  - image: test:latest
`)
	sourceB := writeHookManifest(t, `id: post-b
once: true
steps:
  - image: test:latest
`)
	spec := []byte(`{"steps":[{"image":"test:latest"}],"hooks":["` + sourceA + `","` + sourceB + `"]}`)
	st := &jobStore{}
	st.hasHookOnceLedger.val = true
	st.getHookOnceLedger.val = store.HooksOnce{
		RunID:             runID,
		RepoID:            repoID,
		HookHash:          strings.Repeat("a", 64),
		FirstSuccessJobID: &firstSuccessID,
		OnceSkipMarked:    false,
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: job.Attempt,
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
	}}

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

func TestResolveHookRuntimeDecision_OnceDisabledSkipsLedgerLookup(t *testing.T) {
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
	source := writeHookManifest(t, `id: no-once
once: false
steps:
  - image: test:latest
`)
	spec := []byte(`{"steps":[{"image":"test:latest"}],"hooks":["` + source + `"]}`)
	st := &jobStore{}
	st.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: job.Attempt,
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
	}}

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
	if st.hasHookOnceLedger.called {
		t.Fatal("did not expect HasHookOnceLedger() for once-disabled hook")
	}
	if st.getHookOnceLedger.called {
		t.Fatal("did not expect GetHookOnceLedger() for once-disabled hook")
	}
}

func TestResolveHookRuntimeDecision_CanonicalHashIgnoresSourcePath(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	firstSource := writeHookManifest(t, `id: canonical
once: false
steps:
  - image: test:latest
`)
	secondSource := writeHookManifest(t, `id: canonical
once: false
steps:
  - image: test:latest
`)
	baseJob := store.Job{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: 1,
		JobType: domaintypes.JobTypeHook,
		Name:    "pre-gate-hook-000",
	}
	st := &jobStore{}
	st.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: baseJob.Attempt,
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
	}}

	firstSpec := []byte(`{"steps":[{"image":"test:latest"}],"hooks":["` + firstSource + `"]}`)
	first, err := resolveHookRuntimeDecision(context.Background(), st, baseJob, firstSpec, domaintypes.JobTypeHook)
	if err != nil {
		t.Fatalf("resolve first hook runtime decision: %v", err)
	}

	secondSpec := []byte(`{"steps":[{"image":"test:latest"}],"hooks":["` + secondSource + `"]}`)
	second, err := resolveHookRuntimeDecision(context.Background(), st, baseJob, secondSpec, domaintypes.JobTypeHook)
	if err != nil {
		t.Fatalf("resolve second hook runtime decision: %v", err)
	}

	if first.HookHash == "" || second.HookHash == "" {
		t.Fatalf("hook hash should not be empty: first=%q second=%q", first.HookHash, second.HookHash)
	}
	if first.HookHash != second.HookHash {
		t.Fatalf("canonical hook hash mismatch across different source paths: %q vs %q", first.HookHash, second.HookHash)
	}
}

func TestResolveHookRuntimeDecision_MatcherShouldRunFalseSkipsLedger(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	job := store.Job{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: 1,
		JobType: domaintypes.JobTypeHook,
		Name:    "pre-gate-hook-000",
	}
	source := writeHookManifest(t, `id: sbom-remove-only
once: true
sbom:
  on_remove:
    - name: lib-a
steps:
  - image: test:latest
`)
	spec := []byte(`{"steps":[{"image":"test:latest"}],"hooks":["` + source + `"]}`)
	st := &jobStore{}
	st.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: 1,
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
	}}
	st.listSBOMRowsByJob.val = []store.Sbom{
		{JobID: domaintypes.NewJobID(), RepoID: repoID, Lib: "lib-a", Ver: "1.0.0"},
	}

	got, err := resolveHookRuntimeDecision(context.Background(), st, job, spec, domaintypes.JobTypeHook)
	if err != nil {
		t.Fatalf("resolveHookRuntimeDecision() error: %v", err)
	}
	if got == nil {
		t.Fatal("resolveHookRuntimeDecision() returned nil decision")
	}
	if got.HookShouldRun {
		t.Fatal("HookShouldRun=true, want false when matcher predicates do not match")
	}
	if st.hasHookOnceLedger.called || st.getHookOnceLedger.called {
		t.Fatal("did not expect hook-once ledger checks when once is not eligible")
	}
}

func TestResolveHookRuntimeDecision_RelativeSourceNotFoundReturnsError(t *testing.T) {
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
	spec := []byte(`{"steps":[{"image":"test:latest"}],"hooks":["./hooks/lint.yaml"]}`)
	st := &jobStore{}

	got, err := resolveHookRuntimeDecision(context.Background(), st, job, spec, domaintypes.JobTypeHook)
	if err == nil {
		t.Fatalf("resolveHookRuntimeDecision() error=nil, want not found error, got decision=%+v", got)
	}
	if st.hasHookOnceLedger.called || st.getHookOnceLedger.called {
		t.Fatal("did not expect hook-once ledger calls when hook source resolution fails")
	}
}

func writeHookManifest(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "hook.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write hook manifest %s: %v", path, err)
	}
	return path
}
