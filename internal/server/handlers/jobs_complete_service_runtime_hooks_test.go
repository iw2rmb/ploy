package handlers

import (
	"context"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/hook"
)

func TestRuntimeHookChainStartSHA_NormalizesAndValidates(t *testing.T) {
	t.Parallel()

	got := normalizeRepoSHA(" 0123456789ABCDEF0123456789ABCDEF01234567 ")
	if got != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("normalizeRepoSHA() = %q, want normalized sha", got)
	}
}

func TestEffectiveCompletedRepoSHAOut_NonChangingFallbackToIn(t *testing.T) {
	t.Parallel()

	job := store.Job{
		JobType:   domaintypes.JobTypeSBOM,
		RepoShaIn: "0123456789abcdef0123456789abcdef01234567",
	}
	if got := effectiveCompletedRepoSHAOut(job, ""); got != job.RepoShaIn {
		t.Fatalf("effectiveCompletedRepoSHAOut() = %q, want %q", got, job.RepoShaIn)
	}
}

func TestApplyInsertedHeadRepoSHA_SeedsAndClearsForChangingJob(t *testing.T) {
	t.Parallel()

	st := &jobStore{}
	nextID := domaintypes.NewJobID()
	head := store.Job{
		ID:      domaintypes.NewJobID(),
		RunID:   domaintypes.NewRunID(),
		RepoID:  domaintypes.NewRepoID(),
		Attempt: 3,
		JobType: domaintypes.JobTypeHook,
		NextID:  &nextID,
	}
	if err := applyInsertedHeadRepoSHA(context.Background(), st, head, " 0123456789ABCDEF0123456789ABCDEF01234567 "); err != nil {
		t.Fatalf("applyInsertedHeadRepoSHA() error: %v", err)
	}
	if !st.updateJobRepoSHAIn.called {
		t.Fatal("expected UpdateJobRepoSHAIn to be called")
	}
	if got := st.updateJobRepoSHAIn.params.RepoShaIn; got != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("update repo_sha_in = %q, want normalized sha", got)
	}
	if !st.clearRepoSHAChainFromJob.called {
		t.Fatal("expected ClearRepoSHAChainFromJob to be called")
	}
	if got := st.clearRepoSHAChainFromJob.params; got.ID != nextID || got.RunID != head.RunID || got.RepoID != head.RepoID || got.Attempt != head.Attempt {
		t.Fatalf("unexpected ClearRepoSHAChainFromJob params: %+v", got)
	}
}

func TestApplyHookOncePlanningDecision_SkipsWhenLedgerAlreadyHasSuccess(t *testing.T) {
	t.Parallel()

	st := &jobStore{}
	st.hasHookOnceLedger.val = true
	successJobID := domaintypes.NewJobID()
	st.getHookOnceLedger.val = store.HooksOnce{
		RunID:             domaintypes.NewRunID(),
		RepoID:            domaintypes.NewRepoID(),
		HookHash:          strings.Repeat("a", 64),
		FirstSuccessJobID: &successJobID,
	}

	job := store.Job{
		RunID:  st.getHookOnceLedger.val.RunID,
		RepoID: st.getHookOnceLedger.val.RepoID,
	}
	decision := hookPlanningDecision{
		Evaluated: true,
		Match: hook.MatchDecision{
			ShouldRun: true,
			HookHash:  strings.Repeat("a", 64),
			Once: hook.OnceEligibility{
				Enabled:        true,
				Eligible:       true,
				PersistenceKey: strings.Repeat("a", 64),
			},
		},
	}

	got, err := applyHookOncePlanningDecision(context.Background(), st, job, decision)
	if err != nil {
		t.Fatalf("applyHookOncePlanningDecision() error: %v", err)
	}
	if got.Match.ShouldRun {
		t.Fatal("Match.ShouldRun=true, want false when hook-once ledger already succeeded")
	}
	if !st.hasHookOnceLedger.called {
		t.Fatal("expected HasHookOnceLedger to be called")
	}
	if !st.getHookOnceLedger.called {
		t.Fatal("expected GetHookOnceLedger to be called")
	}
}
