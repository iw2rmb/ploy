package mods

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

// executeORWGenBranch executes an OpenRewrite recipe generation and application branch
func (o *fanoutOrchestrator) executeORWGenBranch(ctx context.Context, branch BranchSpec, result BranchResult) BranchResult {
	// Step 1: Render ORW apply assets
	hclPath, err := o.runner.RenderORWApplyAssets(branch.ID)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to render ORW apply assets: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 2: Pre-substitute recipe variables and prepare env
	rclass, rcoords, rtimeout := buildORWRecipeConfig(branch.Inputs)
	prePath, err := orwPreSubstitute(hclPath, rclass, rcoords, rtimeout)
	if err != nil {
		result.Status = "failed"
		result.Notes = err.Error()
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Provide host directories for bind mounts (no global env)
	baseDir := filepath.Dir(hclPath)
	vars := orwMakeVars(baseDir)

	// Step 2b: Substitute environment variables in HCL template
	runID := ORWRunID(branch.ID)
	renderedHCLPath, err := substituteORWTemplateVars(prePath, runID, vars)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to substitute ORW HCL template: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 3: Report job metadata asynchronously (job name == runID)
	var rep2 EventReporter
	if o.runner != nil {
		rep2 = o.runner.GetEventReporter()
	}
	reportJobSubmittedAsync(ctx, rep2, runID, "apply", string(StepTypeORWApply))

	// Step 4: Preflight validate HCL, then submit job to Nomad and wait for completion
	if err := orwValidateAndSubmit(ctx, o.hcl, renderedHCLPath, ResolveDefaultsFromEnv().AllowPartialORW); err != nil {
		result.Status = "failed"
		result.Notes = err.Error()
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		if o.runner != nil && o.runner.GetEventReporter() != nil {
			_ = o.runner.GetEventReporter().Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: "error", Message: fmt.Sprintf("branch %s failed: %s", branch.ID, result.Notes), Time: time.Now()})
		}
		return result
	}

	// Step 5: Check for generated diff.patch artifact
	// Step 5: Finalize
	if o.runner != nil && o.runner.GetEventReporter() != nil {
		_ = o.runner.GetEventReporter().Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: "info", Message: fmt.Sprintf("branch %s completed", branch.ID), Time: time.Now()})
	}
	orwFinalize(&result, renderedHCLPath, branch.ID)
	return result
}
