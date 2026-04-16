package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func applyHookOncePlanningDecision(
	ctx context.Context,
	st store.Store,
	job store.Job,
	decision hookPlanningDecision,
) (hookPlanningDecision, error) {
	if !decision.Match.Once.Enabled || !decision.Match.Once.Eligible {
		return decision, nil
	}
	hash := strings.TrimSpace(decision.Match.Once.PersistenceKey)
	if hash == "" {
		hash = strings.TrimSpace(decision.Match.HookHash)
	}
	if hash == "" {
		return hookPlanningDecision{}, fmt.Errorf("hook once persistence key is empty")
	}

	runtimeDecision, err := applyHookOnceLedgerDecision(ctx, st, job, &contracts.HookRuntimeDecision{
		HookHash:      hash,
		HookShouldRun: decision.Match.ShouldRun,
	})
	if err != nil {
		return hookPlanningDecision{}, err
	}
	decision.Match.ShouldRun = runtimeDecision.HookShouldRun
	return decision, nil
}
