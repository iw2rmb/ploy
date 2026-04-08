package handlers

import (
	"context"
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type runtimeHookPlan struct {
	sourceIndex int
	source      string
	decision    hookPlanningDecision
}

func (s *CompleteJobService) planAndInsertCycleHookJobs(ctx context.Context, state *completeJobState) (*domaintypes.JobID, error) {
	if state == nil || state.serviceType != completeJobServiceTypeSBOM || state.job.NextID == nil {
		return nil, nil
	}
	cycleName, ok := cycleNameFromSBOMJobName(state.job.Name)
	if !ok {
		return nil, nil
	}

	run, runOK := s.loadRunForPostCompletion(ctx, state, "runtime hook planning")
	if !runOK {
		return nil, fmt.Errorf("load run for runtime hook planning")
	}
	specRow, err := s.store.GetSpec(ctx, run.SpecID)
	if err != nil {
		return nil, fmt.Errorf("load run spec %s: %w", run.SpecID, err)
	}
	migSpec, err := contracts.ParseMigSpecJSON(specRow.Spec)
	if err != nil {
		return nil, fmt.Errorf("parse run spec %s: %w", run.SpecID, err)
	}

	resolvedHooks, err := resolveHookManifestSources(*migSpec)
	if err != nil {
		return nil, fmt.Errorf("resolve hook sources: %w", err)
	}
	if len(resolvedHooks) == 0 {
		return nil, nil
	}

	matchInput, err := buildHookMatchInput(ctx, s.store, state.job)
	if err != nil {
		return nil, fmt.Errorf("build hook match input: %w", err)
	}
	matchInput.Stack = mergeHookRuntimeStackWithFallback(matchInput.Stack, hookRuntimeFallbackStack(migSpec, fallbackCycleName(cycleName)))

	plans := make([]runtimeHookPlan, 0, len(resolvedHooks))
	for i, source := range resolvedHooks {
		decision, decisionErr := resolvePlannableHookDecision(ctx, s.store, migSpec, source, matchInput, blobStoreForPlanning(s.blobpersist))
		if decisionErr != nil {
			return nil, fmt.Errorf("evaluate cycle hook source[%d] %q: %w", i, source, decisionErr)
		}
		if !decision.ShouldRun() {
			continue
		}
		plans = append(plans, runtimeHookPlan{
			sourceIndex: i,
			source:      source,
			decision:    decision,
		})
	}
	if len(plans) == 0 {
		return nil, nil
	}

	hookIDs := make([]domaintypes.JobID, len(plans))
	for i := range plans {
		hookIDs[i] = domaintypes.NewJobID()
	}
	for i := len(plans) - 1; i >= 0; i-- {
		nextID := *state.job.NextID
		if i+1 < len(plans) {
			nextID = hookIDs[i+1]
		}
		hookMeta := contracts.NewMigJobMeta()
		hookMeta.HookSource = plans[i].source
		hookMeta.ActionSummary = summarizeHookPlanningDecision(plans[i].decision)
		hookMetaBytes, hookMetaErr := contracts.MarshalJobMeta(hookMeta)
		if hookMetaErr != nil {
			return nil, fmt.Errorf("marshal hook meta %d: %w", i, hookMetaErr)
		}
		_, err = s.store.CreateJob(ctx, store.CreateJobParams{
			ID:          hookIDs[i],
			RunID:       state.job.RunID,
			RepoID:      state.job.RepoID,
			RepoBaseRef: state.job.RepoBaseRef,
			Attempt:     state.job.Attempt,
			Name:        fmt.Sprintf("%s-hook-%03d", cycleName, plans[i].sourceIndex),
			JobType:     domaintypes.JobTypeHook,
			Status:      domaintypes.JobStatusCreated,
			NextID:      &nextID,
			Meta:        hookMetaBytes,
		})
		if err != nil {
			return nil, fmt.Errorf("create hook job %d: %w", i, err)
		}
	}

	firstHookID := hookIDs[0]
	if err := s.store.UpdateJobNextID(ctx, store.UpdateJobNextIDParams{ID: state.job.ID, NextID: &firstHookID}); err != nil {
		return nil, fmt.Errorf("rewire sbom next_id to first hook: %w", err)
	}
	state.job.NextID = &firstHookID
	return &firstHookID, nil
}

func cycleNameFromSBOMJobName(jobName string) (string, bool) {
	name := strings.TrimSpace(jobName)
	if !strings.HasSuffix(name, "-sbom") {
		return "", false
	}
	cycleName := strings.TrimSpace(strings.TrimSuffix(name, "-sbom"))
	if cycleName == "" {
		return "", false
	}
	return cycleName, true
}

func fallbackCycleName(cycleName string) string {
	cycleName = strings.TrimSpace(cycleName)
	if strings.HasPrefix(cycleName, "re-gate") {
		return "re-gate"
	}
	return cycleName
}
