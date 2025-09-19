package mods

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runORWApplyStep encapsulates the ORW apply step within the Mods workflow.
// It renders assets, prepares input, submits the job, fetches the diff, and runs apply+build.
func (r *ModRunner) runORWApplyStep(ctx context.Context, repoPath string, step ModStep, stepStart time.Time) (StepResult, error) {
	// Render ORW apply HCL assets (prefer transformation executor)
	var renderedPath string
	var err error
	if r.transformExec != nil {
		renderedPath, err = r.transformExec.RenderORWAssets(step.ID)
	} else {
		renderedPath, err = r.RenderORWApplyAssets(step.ID)
	}
	if err != nil {
		return StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to render ORW assets: %v", err)}, fmt.Errorf("failed to render orw-apply assets: %w", err)
	}

	// Guard: ensure repository contains a supported build file before creating input tar
	{
		hasPom, hasGradle, hasKts := checkBuildFiles(repoPath)
		r.emit(ctx, "apply", "guard-build-file", "info", fmt.Sprintf("repo=%s pom=%v gradle=%v kts=%v", repoPath, hasPom, hasGradle, hasKts))
		if err := ensureBuildFile(repoPath); err != nil {
			r.emit(ctx, "apply", string(StepTypeORWApply), "error", "no build file in repo (pom.xml/build.gradle)")
			return StepResult{StepID: step.ID, Success: false, Message: ErrNoBuildFile.Error()}, ErrNoBuildFile
		}
	}

	// Prepare input tar from repository
	inputTar := filepath.Join(filepath.Dir(renderedPath), "input.tar")
	if err := createTarFromDir(repoPath, inputTar); err != nil {
		return StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to create input tar: %v", err)}, fmt.Errorf("failed to create input tar: %w", err)
	}
	// Preview tar contents for diagnostics via reporter
	if r.eventReporter != nil {
		logPreviewTarWithReporter(r.eventReporter, "apply", "input-preview", inputTar, 20)
	} else {
		logPreviewTar(inputTar, 20)
	}

	// Pre-substitute recipe class and input tar host path into template
	rclass := ""
	if len(step.Recipes) > 0 {
		rclass = step.Recipes[0]
	}
	// Determine coordinates strictly from YAML (no discovery)
	rgroup, rartifact, rversion := step.RecipeGroup, step.RecipeArtifact, step.RecipeVersion
	if err := validateRecipeCoords(rgroup, rartifact, rversion, step.ID); err != nil {
		return StepResult{StepID: step.ID, Success: false, Message: err.Error()}, err
	}
	// Optional Maven plugin version (prefer YAML, then env; runner defaults internally if unset)
	pluginVersion := step.MavenPluginVersion
	if pluginVersion == "" {
		pluginVersion = os.Getenv("MODS_MAVEN_PLUGIN_VERSION")
	}
	// Create run ID for this submission and then substitute it
	runID := ORWRunID(step.ID)
	prePath, err := writeORWPreHCL(renderedPath, ORWRecipeParams{Class: rclass, Group: rgroup, Artifact: rartifact, Version: rversion, PluginVersion: pluginVersion}, inputTar, runID)
	if err != nil {
		return StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to write pre-HCL: %v", err)}, fmt.Errorf("failed to write pre-substituted HCL: %w", err)
	}

	// Prepare env and substitute final template
	baseDir := filepath.Dir(renderedPath)

	// Prepare branch-scoped step id and DIFF_KEY using MOD_ID only
	modID := os.Getenv("MOD_ID")
	branchID := step.ID
	bs := NewBranchStep(modID, branchID)
	curStepID := bs.ID
	diffKey := bs.DiffKey

	// Prepare input tar from the cloned repository and upload to SeaweedFS for task-side download
	modID = os.Getenv("MOD_ID")
	seaweed := ResolveInfraFromEnv().SeaweedURL
	// Upload best-effort to artifacts/mods/<id>/input.tar using HTTP client
	if err := uploadInputTar(seaweed, modID, inputTar); err != nil {
		r.emit(ctx, "apply", "input-upload", "warn", fmt.Sprintf("input.tar upload failed: %v", err))
	}
	// Substitute HCL with explicit variables to avoid global env writes
	vars := makeORWVars(baseDir, modID, diffKey, seaweed)
	submittedPath, err := substituteORWTemplateVars(prePath, runID, vars)
	if err != nil {
		return StepResult{StepID: step.ID, Success: false, Message: fmt.Sprintf("Failed to substitute ORW HCL: %v", err)}, fmt.Errorf("failed to substitute ORW HCL: %w", err)
	}

	// Persist a copy of the submitted HCL for post-mortem inspection
	if modID := os.Getenv("MOD_ID"); modID != "" {
		persistDir := filepath.Join("/tmp/mods-submitted", modID, step.ID)
		_ = os.MkdirAll(persistDir, 0755)
		dest := filepath.Join(persistDir, "orw_apply.submitted.hcl")
		if b, e := os.ReadFile(submittedPath); e == nil {
			_ = os.WriteFile(dest, b, 0644)
			r.emit(ctx, "apply", string(StepTypeORWApply), "info", fmt.Sprintf("Saved submitted HCL to %s", dest))
		}
	}

	// Debug: log env block from submitted HCL for verification (INPUT_URL, SEAWEEDFS_URL, etc.)
	if b, e := os.ReadFile(submittedPath); e == nil {
		s := string(b)
		start := strings.Index(s, "env = {")
		if start >= 0 {
			end := strings.Index(s[start:], "}")
			if end > 0 {
				block := s[start : start+end+1]
				_ = block // currently unused; reserved for local checks
			}
		}
	}
	// Prepare diff path for later fetch and processing
	diffPath := filepath.Join(baseDir, "out", "diff.patch")
	_ = os.MkdirAll(filepath.Dir(diffPath), 0755)
	r.emit(ctx, "apply", string(StepTypeORWApply), "info", "Submitting orw-apply job")
	// Submit job and fetch diff via executor/helper
	orwTimeout := ResolveDefaultsFromEnv().ORWApplyTimeout
	if r.transformExec != nil {
		params := ORWSubmitParams{
			SeaweedURL:       seaweed,
			ModID:            os.Getenv("MOD_ID"),
			BranchID:         branchID,
			StepID:           curStepID,
			RunID:            runID,
			SubmittedHCLPath: submittedPath,
			DiffPath:         diffPath,
			Timeout:          orwTimeout,
		}
		if _, err := r.transformExec.SubmitORWAndFetchDiff(ctx, params); err != nil {
			r.emit(ctx, "apply", string(StepTypeORWApply), "error", err.Error())
			return StepResult{StepID: step.ID, Success: false, Message: err.Error()}, err
		}
	} else if err := submitORWJobAndFetchDiff(ctx,
		func(p string) error {
			if r.hcl != nil {
				return r.hcl.Validate(p)
			}
			return validateJob(p)
		},
		func(p string, t time.Duration) error {
			if r.hcl != nil {
				return r.hcl.Submit(p, t)
			}
			return submitAndWaitTerminal(p, t)
		},
		r.reportLastJobAsync,
		seaweed,
		os.Getenv("MOD_ID"), branchID, curStepID, runID,
		submittedPath, diffPath, orwTimeout); err != nil {
		r.emit(ctx, "apply", string(StepTypeORWApply), "error", err.Error())
		return StepResult{StepID: step.ID, Success: false, Message: err.Error()}, err
	}
	// Successful wait and fetch implies job completed
	r.emit(ctx, "apply", string(StepTypeORWApply), "info", "orw-apply job completed")

	// Reconstruct branch state: apply all prior diffs from chain HEAD → root
	_ = r.reconstructBranchState(ctx, seaweed, modID, step.ID, baseDir, repoPath)

	if fi, err := os.Stat(diffPath); err == nil {
		r.emit(ctx, "apply", "diff-found", "info", fmt.Sprintf("diff ready (%d bytes)", fi.Size()))
		if fi.Size() == 0 {
			// Treat empty diff as no-op: skip apply/build and continue pipeline
			msg := "No changes produced by orw-apply; skipping apply/build"
			r.emit(ctx, "apply", "diff-empty", "info", msg)
			return StepResult{StepID: step.ID, Success: true, Message: msg, Duration: time.Since(stepStart)}, nil
		}
	} else {
		r.emit(ctx, "apply", "diff-stat", "warn", fmt.Sprintf("diff stat failed: %v", err))
	}

	// Apply diff (build gate will be executed later in the workflow)
	sr, err := runApplyDiffWithEvents(ctx, r, repoPath, diffPath, step.ID, stepStart, r.ApplyDiffOnly)
	if err != nil {
		return sr, err
	}

	// Record chain metadata for this branch (option_id = step.ID)
	{
		branchID := step.ID
		branchDiffKey := computeBranchDiffKey(modID, branchID, curStepID)
		_ = writeBranchChainStepMeta(seaweed, modID, branchID, curStepID, branchDiffKey)
	}

	return sr, nil
}
