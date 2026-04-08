package handlers

import (
	"context"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/hook"
)

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
