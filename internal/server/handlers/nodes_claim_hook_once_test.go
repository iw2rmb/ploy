package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
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

	const (
		hash     = "a1b2c3d4e5f6"
		bundleID = "bundle_hook_a"
	)
	st, bs := newHookBundleFixture(t, hash, bundleID, `id: pre-a
once: true
steps:
  - image: test:latest
`)
	st.hasHookOnceLedger.val = false
	st.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: job.Attempt,
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
	}}
	spec := specWithHooksAndBundleMap([]string{hash}, map[string]string{hash: bundleID})

	got, err := resolveHookRuntimeDecision(context.Background(), st, bs, job, spec, domaintypes.JobTypeHook)
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

	const (
		hashA     = "1111111aaaaa"
		hashB     = "2222222bbbbb"
		bundleIDB = "bundle_hook_b"
	)
	st, bs := newHookBundleFixture(t, hashB, bundleIDB, `id: post-b
once: true
steps:
  - image: test:latest
`)
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
	spec := specWithHooksAndBundleMap(
		[]string{hashA, hashB},
		map[string]string{hashA: "bundle_hook_a", hashB: bundleIDB},
	)

	got, err := resolveHookRuntimeDecision(context.Background(), st, bs, job, spec, domaintypes.JobTypeHook)
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

	const (
		hash     = "abcdef123456"
		bundleID = "bundle_no_once"
	)
	st, bs := newHookBundleFixture(t, hash, bundleID, `id: no-once
once: false
steps:
  - image: test:latest
`)
	st.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: job.Attempt,
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
	}}
	spec := specWithHooksAndBundleMap([]string{hash}, map[string]string{hash: bundleID})

	got, err := resolveHookRuntimeDecision(context.Background(), st, bs, job, spec, domaintypes.JobTypeHook)
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
	baseJob := store.Job{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: 1,
		JobType: domaintypes.JobTypeHook,
		Name:    "pre-gate-hook-000",
	}
	firstManifest := `id: canonical
once: false
steps:
  - image: test:latest
`
	secondManifest := `id: canonical
once: false
steps:
  - image: test:latest
`

	st1, bs1 := newHookBundleFixture(t, "abcabcabcabc", "bundle_one", firstManifest)
	st1.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: baseJob.Attempt,
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
	}}
	firstSpec := specWithHooksAndBundleMap([]string{"abcabcabcabc"}, map[string]string{"abcabcabcabc": "bundle_one"})
	first, err := resolveHookRuntimeDecision(context.Background(), st1, bs1, baseJob, firstSpec, domaintypes.JobTypeHook)
	if err != nil {
		t.Fatalf("resolve first hook runtime decision: %v", err)
	}

	st2, bs2 := newHookBundleFixture(t, "defdefdefdef", "bundle_two", secondManifest)
	st2.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: baseJob.Attempt,
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
	}}
	secondSpec := specWithHooksAndBundleMap([]string{"defdefdefdef"}, map[string]string{"defdefdefdef": "bundle_two"})
	second, err := resolveHookRuntimeDecision(context.Background(), st2, bs2, baseJob, secondSpec, domaintypes.JobTypeHook)
	if err != nil {
		t.Fatalf("resolve second hook runtime decision: %v", err)
	}

	if first.HookHash == "" || second.HookHash == "" {
		t.Fatalf("hook hash should not be empty: first=%q second=%q", first.HookHash, second.HookHash)
	}
	if first.HookHash != second.HookHash {
		t.Fatalf("canonical hook hash mismatch across different sources: %q vs %q", first.HookHash, second.HookHash)
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

	const (
		hash     = "feedbeef1234"
		bundleID = "bundle_remove_only"
	)
	st, bs := newHookBundleFixture(t, hash, bundleID, `id: sbom-remove-only
once: true
sbom:
  on_remove:
    - name: lib-a
steps:
  - image: test:latest
`)
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
	spec := specWithHooksAndBundleMap([]string{hash}, map[string]string{hash: bundleID})

	got, err := resolveHookRuntimeDecision(context.Background(), st, bs, job, spec, domaintypes.JobTypeHook)
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

func TestResolveHookRuntimeDecision_UsesResolvedHookSourceFromJobMeta(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	const (
		hash     = "89abcdef0123"
		bundleID = "bundle_meta_source"
	)
	st, bs := newHookBundleFixture(t, hash, bundleID, `id: meta-source
once: false
steps:
  - image: test:latest
`)
	metaBytes, err := contracts.MarshalJobMeta(&contracts.JobMeta{
		Kind:       contracts.JobKindMig,
		HookSource: hash,
	})
	if err != nil {
		t.Fatalf("marshal hook job meta: %v", err)
	}
	job := store.Job{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		JobType: domaintypes.JobTypeHook,
		Name:    "pre-gate-hook-000",
		Meta:    metaBytes,
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: job.Attempt,
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
	}}
	spec := specWithHooksAndBundleMap([]string{"https://hooks.example.com/v1/hook.yaml"}, map[string]string{hash: bundleID})

	got, err := resolveHookRuntimeDecision(context.Background(), st, bs, job, spec, domaintypes.JobTypeHook)
	if err != nil {
		t.Fatalf("resolveHookRuntimeDecision() unexpected error: %v", err)
	}
	if !got.HookShouldRun {
		t.Fatalf("HookShouldRun=false, want true; decision=%+v", got)
	}
}

func TestResolveHookRuntimeDecision_InvalidHookSpecIsTerminalClaimError(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	const (
		hash     = "aa11bb22cc33"
		bundleID = "bundle_invalid_hook"
	)
	st, bs := newHookBundleFixture(t, hash, bundleID, `id: invalid-hook
steps:
  - image: test:latest
    unknown_key: true
`)
	job := store.Job{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		JobType: domaintypes.JobTypeHook,
		Name:    "pre-gate-hook-000",
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      domaintypes.NewJobID(),
		RunID:   runID,
		RepoID:  repoID,
		Attempt: job.Attempt,
		JobType: domaintypes.JobTypeSBOM,
		Status:  domaintypes.JobStatusSuccess,
	}}
	spec := specWithHooksAndBundleMap([]string{hash}, map[string]string{hash: bundleID})

	_, err := resolveHookRuntimeDecision(context.Background(), st, bs, job, spec, domaintypes.JobTypeHook)
	if err == nil {
		t.Fatal("expected resolveHookRuntimeDecision error")
	}
	var terminalErr *ClaimJobTerminalError
	if !errors.As(err, &terminalErr) {
		t.Fatalf("expected ClaimJobTerminalError, got %T (%v)", err, err)
	}
	if !strings.Contains(err.Error(), "unknown_key") {
		t.Fatalf("expected strict decode error in terminal message, got: %v", err)
	}
}

func newHookBundleFixture(t *testing.T, hash string, bundleID string, hookYAML string) (*jobStore, *bsmock.Store) {
	t.Helper()
	st := &jobStore{}
	bs := bsmock.New()
	objKey := "spec_bundles/" + bundleID + "/bundle.tar.gz"
	st.getSpecBundle.val = store.SpecBundle{
		ID:        bundleID,
		ObjectKey: &objKey,
	}
	if _, err := bs.Put(context.Background(), objKey, "application/gzip", makeDirectContentBundle(t, hookYAML)); err != nil {
		t.Fatalf("put hook bundle blob: %v", err)
	}
	_ = hash
	return st, bs
}

func specWithHooksAndBundleMap(hooks []string, bundleMap map[string]string) []byte {
	var hooksPart []string
	for _, h := range hooks {
		hooksPart = append(hooksPart, fmt.Sprintf("%q", h))
	}
	var bmParts []string
	for k, v := range bundleMap {
		bmParts = append(bmParts, fmt.Sprintf("%q:%q", k, v))
	}
	return []byte(fmt.Sprintf(
		`{"steps":[{"image":"test:latest"}],"hooks":[%s],"bundle_map":{%s}}`,
		strings.Join(hooksPart, ","),
		strings.Join(bmParts, ","),
	))
}

func makeDirectContentBundle(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	body := []byte(content)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "content",
		Typeflag: tar.TypeReg,
		Mode:     0o644,
		Size:     int64(len(body)),
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}
