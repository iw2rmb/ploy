package mods

import (
	"context"
	"fmt"
	"time"
)

// executeHumanStepBranch handles human intervention branches with Git-based manual intervention workflow
func (o *fanoutOrchestrator) executeHumanStepBranch(ctx context.Context, branch BranchSpec, result BranchResult) BranchResult {
	// Parse timeout from branch inputs
	timeout := humanStepTimeout(branch, ResolveDefaultsFromEnv().BuildApplyTimeout)

	// Create timeout context for this branch
	branchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Get build error context
	buildError := humanStepBuildError(branch.Inputs)

	gp, bc, err := humanStepPreflight(o)
	if err != nil {
		result.Status = "failed"
		result.Notes = err.Error()
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 1: Create intervention branch name (derived in MR config)

	// Step 2: Create MR for human intervention
	mrConfig := humanStepMakeMRConfig(o.runner.GetTargetRepo(), branch.ID, buildError)
	mrResult, err := gp.CreateOrUpdateMR(branchCtx, mrConfig)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("Failed to create human intervention MR: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 3: Poll for manual commits and validate build
	if _, err := humanStepPollForFix(branchCtx, bc, branch.ID, 30*time.Second); err == nil {
		result.Status = "completed"
		result.Notes = fmt.Sprintf("Human intervention successful via MR %s - build now passes", mrResult.MRURL)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}
	// Timeout reached
	result.Status = "timeout"
	result.Notes = fmt.Sprintf("Human intervention timed out after %v", timeout)
	result.FinishedAt = time.Now()
	result.Duration = time.Since(result.StartedAt)
	return result
}
