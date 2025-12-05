package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// rehydrateWorkspaceForStep creates a fresh workspace for the given step by rehydrating
// from the base clone and applying ordered diffs from prior steps.
//
// For step 0: Creates base clone (or reuses cached base if available).
// For step k>0: Copies base clone + applies diffs from steps 0 through k-1.
//
// This function implements the core rehydration strategy that enables multi-node execution:
// each step can run on any node by reconstructing workspace state from base + diff chain.
//
// Parameters:
//   - ctx: Context for cancellation and deadlines.
//   - req: StartRunRequest containing repo URL, base_ref, and commit_sha.
//   - manifest: StepManifest for this step.
//   - stepIndex: Job step index for execution tracking.
//
// Returns:
//   - workspacePath: Path to the rehydrated workspace ready for execution.
//   - error: Non-nil if rehydration fails (clone, copy, or patch application error).
func (r *runController) rehydrateWorkspaceForStep(
	ctx context.Context,
	req StartRunRequest,
	manifest contracts.StepManifest,
	stepIndex types.StepIndex,
) (string, error) {
	runID := req.RunID.String()

	// Step 1: Ensure base clone exists (create on first use, reuse on subsequent calls).
	// Base clone path is deterministic per run and node.
	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	if baseRoot == "" {
		baseRoot = os.TempDir()
	}
	baseClone := filepath.Join(baseRoot, "ploy", "run", runID, "base")
	if err := os.MkdirAll(baseClone, 0o755); err != nil {
		return "", fmt.Errorf("create base clone dir: %w", err)
	}

	slog.Info("creating base clone for run", "run_id", runID, "path", baseClone)

	// Initialize git fetcher for repository hydration. The fetcher is responsible for
	// reusing cached clones when PLOYD_CACHE_HOME is configured.
	gitFetcher, err := r.createGitFetcher()
	if err != nil {
		return "", fmt.Errorf("create git fetcher: %w", err)
	}

	// Determine repo materialization:
	// - Prefer manifest inputs that already carry hydration.Repo (gate/mod jobs).
	// - Fallback to StartRunRequest repo fields (healing jobs and other callers).
	var repo *contracts.RepoMaterialization
	for _, input := range manifest.Inputs {
		if input.Hydration != nil && input.Hydration.Repo != nil {
			repo = input.Hydration.Repo
			break
		}
	}

	if repo == nil {
		// Derive repo materialization from StartRunRequest, mirroring
		// buildManifestFromRequest semantics.
		targetRef := strings.TrimSpace(req.TargetRef.String())
		if targetRef == "" && strings.TrimSpace(req.BaseRef.String()) != "" {
			targetRef = strings.TrimSpace(req.BaseRef.String())
		}

		tmp := contracts.RepoMaterialization{
			URL:       req.RepoURL,
			BaseRef:   req.BaseRef,
			TargetRef: types.GitRef(targetRef),
			Commit:    req.CommitSHA,
		}
		repo = &tmp
	}

	if err := gitFetcher.Fetch(ctx, repo, baseClone); err != nil {
		return "", fmt.Errorf("hydrate base clone: %w", err)
	}

	slog.Info("base clone created", "run_id", runID, "path", baseClone)

	// Step 2: Rehydrate workspace from base clone + ordered diffs.
	// C2: For ALL steps (including step 0), fetch diffs and apply them.
	// This ensures step 0 runs on the healed baseline if pre-mod healing occurred.
	workspacePath, err := createWorkspaceDir()
	if err != nil {
		return "", fmt.Errorf("create workspace dir: %w", err)
	}

	slog.Info("rehydrating workspace from base + diffs", "run_id", runID, "step_index", stepIndex)

	diffFetcher, err := NewDiffFetcher(r.cfg)
	if err != nil {
		_ = os.RemoveAll(workspacePath)
		return "", fmt.Errorf("create diff fetcher: %w", err)
	}

	// C2: Uniform rehydration query for ALL steps.
	// Fetch diffs where step_index < stepIndex (all diffs from previous jobs).
	// Jobs are ordered by step_index (e.g., 1000=pre-gate, 2000=mod-0, 3000=post-gate).
	gzippedDiffs, err := diffFetcher.FetchDiffsForStep(ctx, runID, stepIndex-1)
	if err != nil {
		_ = os.RemoveAll(workspacePath)
		return "", fmt.Errorf("fetch diffs for step: %w", err)
	}

	slog.Info("fetched diffs for rehydration", "run_id", runID, "step_index", stepIndex, "diff_count", len(gzippedDiffs))

	// Rehydrate workspace from base + diffs using the helper from execution.go.
	if err := RehydrateWorkspaceFromBaseAndDiffs(ctx, baseClone, workspacePath, gzippedDiffs); err != nil {
		_ = os.RemoveAll(workspacePath)
		return "", fmt.Errorf("rehydrate from base and diffs: %w", err)
	}

	slog.Info("workspace rehydrated successfully", "run_id", runID, "step_index", stepIndex, "workspace", workspacePath)

	// Create baseline commit after rehydration to enable incremental diffs.
	// C2: Now applies to ALL steps (including step 0) when diffs were applied.
	// This commit establishes a new HEAD so that "git diff HEAD" generates
	// only the changes from this step, not cumulative changes from prior steps.
	if len(gzippedDiffs) > 0 {
		if err := ensureBaselineCommitForRehydration(ctx, workspacePath, stepIndex); err != nil {
			_ = os.RemoveAll(workspacePath)
			return "", fmt.Errorf("create baseline commit for rehydration: %w", err)
		}
		slog.Info("baseline commit created for incremental diff", "run_id", runID, "step_index", stepIndex)
	}

	return workspacePath, nil
}

// uploadDiffForStep generates and uploads a diff for the given step with step_index metadata.
// This replaces the older uploadDiff method and tags each diff with its step index for
// ordered rehydration in multi-step/multi-node scenarios.
func (r *runController) uploadDiffForStep(
	ctx context.Context,
	runID types.RunID,
	jobID types.JobID,
	diffGenerator step.DiffGenerator,
	workspace string,
	result step.Result,
	stepIndex types.StepIndex,
) {
	if diffGenerator == nil {
		return
	}

	// Generate workspace diff for this step.
	diffBytes, err := diffGenerator.Generate(ctx, workspace)
	if err != nil {
		slog.Error("failed to generate step diff", "run_id", runID, "step_index", stepIndex, "error", err)
		return
	}

	if len(diffBytes) == 0 {
		// No changes from this step; skip upload.
		slog.Info("no diff to upload for step (no changes)", "run_id", runID, "step_index", stepIndex)
		return
	}

	// Build diff summary with step metadata for database storage.
	// C2: Every diff is tagged with step_index + mod_type for unified rehydration.
	// - step_index: Job step index for ordering and rehydration queries.
	// - mod_type: "mod" for main mod diffs (healing diffs use "healing" in execution_healing.go).
	summary := types.DiffSummary{
		"step_index": stepIndex,
		"mod_type":   "mod", // Identifies this diff as a main mod step diff.
		"exit_code":  result.ExitCode,
		"timings": map[string]interface{}{
			"hydration_duration_ms":  result.Timings.HydrationDuration.Milliseconds(),
			"execution_duration_ms":  result.Timings.ExecutionDuration.Milliseconds(),
			"build_gate_duration_ms": result.Timings.BuildGateDuration.Milliseconds(),
			"diff_duration_ms":       result.Timings.DiffDuration.Milliseconds(),
			"total_duration_ms":      result.Timings.TotalDuration.Milliseconds(),
		},
	}

	// Upload diff with step metadata to control plane.
	diffUploader, err := NewDiffUploader(r.cfg)
	if err != nil {
		slog.Error("failed to create diff uploader", "run_id", runID, "step_index", stepIndex, "error", err)
		return
	}

	// Upload diff to job-scoped endpoint. Step ordering is tracked in the summary metadata.
	if err := diffUploader.UploadDiff(ctx, runID, jobID, diffBytes, summary); err != nil {
		slog.Error("failed to upload step diff", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "error", err)
		return
	}

	slog.Info("step diff uploaded successfully", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "size", len(diffBytes))
}

// uploadBaselineDiff uploads a diff between two directories with the specified mod_type and step_index.
// Used by C2 to persist pre-mod or post-mod healing changes so subsequent steps run on the healed baseline.
//
// Parameters:
//   - baseDir: the reference directory (e.g., base clone before healing)
//   - modifiedDir: the modified directory (e.g., healed workspace)
//   - stepIndex: the step index to tag the diff with
//   - modType: the mod_type to tag the diff with (e.g., "pre_gate", "post_gate")
//
// Returns error if diff generation fails or diff is empty (healing claimed success but made no changes).
func (r *runController) uploadBaselineDiff(
	ctx context.Context,
	runID types.RunID,
	jobID types.JobID,
	diffGenerator step.DiffGenerator,
	baseDir string,
	modifiedDir string,
	stepIndex types.StepIndex,
	modType string,
) error {
	if diffGenerator == nil {
		return fmt.Errorf("diff generator nil, cannot upload baseline diff")
	}

	diffBytes, err := diffGenerator.GenerateBetween(ctx, baseDir, modifiedDir)
	if err != nil {
		return fmt.Errorf("failed to generate baseline diff: %w", err)
	}

	if len(diffBytes) == 0 {
		// Empty diff after healing passed means something is wrong:
		// either the gate is flaky or healing made no actual changes.
		// Fail the run to surface this inconsistency.
		return fmt.Errorf("healing produced empty diff but gate passed - possible flaky gate or healing made no changes")
	}

	summary := types.DiffSummary{
		"step_index": stepIndex,
		"mod_type":   modType,
	}

	diffUploader, err := NewDiffUploader(r.cfg)
	if err != nil {
		return fmt.Errorf("failed to create diff uploader: %w", err)
	}

	// Upload diff to job-scoped endpoint. Step ordering is tracked in the summary metadata.
	if err := diffUploader.UploadDiff(ctx, runID, jobID, diffBytes, summary); err != nil {
		return fmt.Errorf("failed to upload baseline diff: %w", err)
	}

	slog.Info("baseline diff uploaded successfully", "run_id", runID, "job_id", jobID, "mod_type", modType, "step_index", stepIndex, "size", len(diffBytes))
	return nil
}
