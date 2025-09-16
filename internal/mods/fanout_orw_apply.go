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

	// Step 2: Read HCL template and substitute recipe-specific variables
	hclBytes, err := os.ReadFile(hclPath)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to read ORW HCL template: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Get recipe configuration from branch inputs
	rclass := ""
	rcoords := ""
	rtimeout := "10m"

	if inputs, ok := branch.Inputs["recipe_config"].(map[string]interface{}); ok {
		if class, ok := inputs["class"].(string); ok {
			rclass = class
		}
		if coords, ok := inputs["coords"].(string); ok {
			rcoords = coords
		}
		if timeout, ok := inputs["timeout"].(string); ok {
			rtimeout = timeout
		}
	}

	// Perform recipe-specific substitution first, then apply environment/template substitution
	prePath := strings.ReplaceAll(hclPath, ".rendered.hcl", ".pre.hcl")
	preContent := strings.NewReplacer(
		"${RECIPE_CLASS}", rclass,
		"${RECIPE_COORDS}", rcoords,
		"${RECIPE_TIMEOUT}", rtimeout,
	).Replace(string(hclBytes))
	if err := os.WriteFile(prePath, []byte(preContent), 0644); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to write pre-substituted ORW HCL: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Provide host directories for bind mounts (no global env)
	baseDir := filepath.Dir(hclPath)
	_ = os.MkdirAll(filepath.Join(baseDir, "out"), 0755)
	imgs := ResolveImagesFromEnv()
	infra := ResolveInfraFromEnv()
	// Resolve MOD_ID for ORW apply branch
	modID := os.Getenv("MOD_ID")
	vars := map[string]string{
		"MODS_CONTEXT_DIR":     baseDir,
		"MODS_OUT_DIR":         filepath.Join(baseDir, "out"),
		"PLOY_CONTROLLER":      infra.Controller,
		"MOD_ID":               modID,
		"PLOY_SEAWEEDFS_URL":   infra.SeaweedURL,
		"MODS_DIFF_KEY":        os.Getenv("MODS_DIFF_KEY"),
		"MODS_ORW_APPLY_IMAGE": imgs.ORWApply,
		"MODS_REGISTRY":        imgs.Registry,
		"NOMAD_DC":             infra.DC,
	}

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
	var orwVErr error
	if o.hcl != nil {
		orwVErr = o.hcl.Validate(renderedHCLPath)
	} else {
		orwVErr = nil
	}
	if orwVErr != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("ORW apply HCL validation failed: %v", orwVErr)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}
	timeout := ResolveDefaultsFromEnv().ORWApplyTimeout
	var orwSErr error
	if o.hcl != nil {
		orwSErr = o.hcl.SubmitCtx(ctx, renderedHCLPath, timeout)
	} else {
		orwSErr = fmt.Errorf("no HCL submitter in test mode")
	}
	if orwSErr != nil {
		diffPath := filepath.Join(filepath.Dir(renderedHCLPath), "out", "diff.patch")
		if ResolveDefaultsFromEnv().AllowPartialORW {
			if fi, statErr := os.Stat(diffPath); statErr == nil && fi.Size() > 0 {
				// proceed (partial allowed)
			} else {
				result.Status = "failed"
				result.Notes = fmt.Sprintf("ORW apply job failed: %v", orwSErr)
				result.FinishedAt = time.Now()
				result.Duration = time.Since(result.StartedAt)
				return result
			}
		} else {
			result.Status = "failed"
			result.Notes = fmt.Sprintf("ORW apply job failed: %v", orwSErr)
			result.FinishedAt = time.Now()
			result.Duration = time.Since(result.StartedAt)
			if o.runner != nil && o.runner.GetEventReporter() != nil {
				_ = o.runner.GetEventReporter().Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: "error", Message: fmt.Sprintf("branch %s failed: %s", branch.ID, result.Notes), Time: time.Now()})
			}
			return result
		}
	}

	// Step 5: Check for generated diff.patch artifact
	diffPath := filepath.Join(filepath.Dir(renderedHCLPath), "out", "diff.patch")
	if _, err := os.Stat(diffPath); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("ORW apply job completed but no diff.patch found: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		if o.runner != nil && o.runner.GetEventReporter() != nil {
			_ = o.runner.GetEventReporter().Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: "info", Message: fmt.Sprintf("branch %s completed", branch.ID), Time: time.Now()})
		}
		return result
	}

	result.Status = "completed"
	result.Notes = fmt.Sprintf("ORW apply job completed successfully, diff.patch at: %s", diffPath)
	result.FinishedAt = time.Now()
	result.Duration = time.Since(result.StartedAt)
	return result
}
