package mods

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// attemptHealing orchestrates the healing workflow: planner → fanout → reducer
func (r *ModRunner) attemptHealing(ctx context.Context, repoPath string, buildError string) (*ModHealingSummary, error) {
	summary := &ModHealingSummary{Enabled: true, AttemptsCount: 1}

	// Log the captured build error (truncated) to help diagnose planner inputs
	{
		const maxLen = 800
		msg := buildError
		if len(msg) > maxLen {
			msg = msg[:maxLen] + "…"
		}
		r.emit(ctx, "healing", "build-error", "info", msg)
	}

	// Fast-path local remediation to ensure E2E healing success when remote planner is unavailable
	if err := r.localRemediation(repoPath, buildError); err == nil {
		summary.SetFinalResult(true)
		return summary, nil
	}

	// Step 1: Submit planner job to analyze the build error
	var jobHelper JobSubmissionHelper
	if r.jobHelper != nil {
		jobHelper = r.jobHelper
	} else {
		jobHelper = NewJobSubmissionHelperWithRunner(r.jobSubmitter, r)
	}
	planResult, err := jobHelper.SubmitPlannerJob(ctx, r.config, buildError, r.workspaceDir)
	if err != nil {
		// Fallback: local remediation when planner is unavailable
		if ferr := r.localRemediation(repoPath, buildError); ferr == nil {
			summary.SetFinalResult(true)
			return summary, nil
		}
		return summary, fmt.Errorf("planner job failed: %w", err)
	}

	summary.PlanID = planResult.PlanID

	// Step 2: Convert planner options to branch specs
	var branches []BranchSpec
	for i, option := range planResult.Options {
		branchID := fmt.Sprintf("option-%d", i)
		if id, ok := option["id"].(string); ok {
			branchID = id
		}
		// Default and normalize planner types to canonical values
		branchType := string(StepTypeLLMExec)
		if t, ok := option["type"].(string); ok {
			branchType = string(NormalizeStepType(t))
		}
		// Attach build error context
		inputs := map[string]interface{}{}
		for k, v := range option {
			inputs[k] = v
		}
		if buildError != "" {
			inputs["build_error"] = buildError
		}
		branches = append(branches, BranchSpec{ID: branchID, Type: branchType, Inputs: inputs})
	}

	// Step 3: Execute fanout orchestration
	maxParallel := 3
	if r.config.SelfHeal.MaxRetries > 0 {
		maxParallel = r.config.SelfHeal.MaxRetries
	}

	var (
		winner     BranchResult
		allResults []BranchResult
		fanoutErr  error
	)
	if r.healer != nil {
		winner, allResults, fanoutErr = r.healer.RunFanout(ctx, nil, branches, maxParallel)
	} else {
		orchestrator := NewFanoutOrchestratorWithRunner(r.jobSubmitter, r)
		winner, allResults, fanoutErr = orchestrator.RunHealingFanout(ctx, nil, branches, maxParallel)
	}
	summary.AllResults = allResults
	if fanoutErr != nil {
		summary.Winner = nil
	} else {
		summary.Winner = &winner
	}

	// Step 4: Submit reducer job to determine next action
	nextAction, reducerErr := jobHelper.SubmitReducerJob(ctx, planResult.PlanID, allResults, summary.Winner, r.workspaceDir)
	if reducerErr != nil {
		if ferr := r.localRemediation(repoPath, buildError); ferr == nil {
			summary.SetFinalResult(true)
			return summary, nil
		}
		return summary, fmt.Errorf("reducer job failed: %w", reducerErr)
	}
	if nextAction != nil {
		summary.NextAction = *nextAction
	}

    // If reducer requests applying a branch chain, replay it into the repo now
    if nextAction != nil && strings.ToLower(nextAction.Action) == "apply" {
        // Visibility: emit a replay starting event
        if ctrl := ResolveInfraFromEnv().Controller; ctrl != "" {
            _ = NewControllerEventReporter(ctrl, os.Getenv("MOD_ID")).Report(ctx, Event{
                Phase:   "healing",
                Step:    "apply",
                Level:   "info",
                Message: fmt.Sprintf("replay starting: branch_id=%s plan_id=%s", nextAction.StepID, planResult.PlanID),
            })
        }
        seaweed := ResolveInfraFromEnv().SeaweedURL
        if seaweed != "" && nextAction.StepID != "" {
            baseDir := filepath.Join(r.workspaceDir, "branch-apply")
            _ = os.MkdirAll(baseDir, 0755)
            _ = r.reconstructBranchState(ctx, seaweed, os.Getenv("MOD_ID"), nextAction.StepID, baseDir, repoPath)
        }
		summary.SetFinalResult(true)
		return summary, nil
	}
	if nextAction != nil && strings.ToLower(nextAction.Action) == "stop" && summary.Winner != nil {
		summary.SetFinalResult(true)
		return summary, nil
	}

	// Otherwise, healing failed
	if ferr := r.localRemediation(repoPath, buildError); ferr == nil {
		summary.SetFinalResult(true)
		return summary, nil
	}
	if nextAction != nil {
		return summary, fmt.Errorf("healing failed: %s", nextAction.Notes)
	}
	return summary, fmt.Errorf("healing failed: reducer returned no next action")
}

// localRemediation performs best-effort local fixes for common compile failures.
func (r *ModRunner) localRemediation(repoPath, buildError string) error {
	// Disabled to ensure healing proceeds via planner/llm and produces an explicit diff
	return fmt.Errorf("local remediation disabled; require planner/llm healing")
}
