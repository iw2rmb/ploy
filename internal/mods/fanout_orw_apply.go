package mods

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	modID := os.Getenv("MOD_ID")
	infra := ResolveInfraFromEnv()
	seaweedURL := infra.SeaweedURL
	runID := ORWRunID(branch.ID)
	diffKey := computeBranchDiffKey(modID, branch.ID, runID)
	vars := makeORWVars(baseDir, modID, diffKey, seaweedURL)

	// Prepare input tar from the repo and upload to SeaweedFS for task-side download
	repoRoot := filepath.Join(o.runner.GetWorkspaceDir(), "repo")
	inputTar := filepath.Join(baseDir, "input.tar")
	if err := createTarFromDir(repoRoot, inputTar); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to create ORW input tar: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}
	if modID == "" || seaweedURL == "" {
		result.Status = "failed"
		result.Notes = "missing MOD_ID or SeaweedFS URL for ORW branch"
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}
	if err := uploadInputTar(seaweedURL, modID, inputTar); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to upload ORW input tar: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}
	inputURL := strings.TrimRight(seaweedURL, "/") + "/artifacts/mods/" + modID + "/input.tar"
	available := false
	for i := 0; i < 10; i++ {
		if headURLFn(inputURL) {
			available = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !available {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("input.tar not yet available at %s", inputURL)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 2b: Substitute environment variables in HCL template
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
