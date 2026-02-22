package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
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
//   - req: StartRunRequest containing repo URL, base_ref, commit_sha, and job_name.
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
	repoID := req.RepoID
	if repoID.IsZero() {
		return "", fmt.Errorf("rehydrate workspace: repo_id is required for repo-scoped diffs listing")
	}

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
		if removeErr := os.RemoveAll(workspacePath); removeErr != nil {
			slog.Warn("failed to clean up workspace after error", "path", workspacePath, "error", removeErr)
		}
		return "", fmt.Errorf("create diff fetcher: %w", err)
	}

	// C2: Uniform rehydration query for ALL steps.
	// Fetch diffs where step_index < stepIndex (all diffs from previous jobs).
	// Jobs are ordered by step_index (e.g., 1000=pre-gate, 2000=mod-0, 3000=post-gate).
	gzippedDiffs, err := diffFetcher.FetchDiffsForStepRepo(ctx, req.RunID, repoID, stepIndex-1)
	if err != nil {
		if removeErr := os.RemoveAll(workspacePath); removeErr != nil {
			slog.Warn("failed to clean up workspace after error", "path", workspacePath, "error", removeErr)
		}
		return "", fmt.Errorf("fetch diffs for step: %w", err)
	}

	slog.Info("fetched diffs for rehydration", "run_id", runID, "step_index", stepIndex, "diff_count", len(gzippedDiffs))

	// Rehydrate workspace from base + diffs using the helper from execution.go.
	if err := RehydrateWorkspaceFromBaseAndDiffs(ctx, baseClone, workspacePath, gzippedDiffs); err != nil {
		if removeErr := os.RemoveAll(workspacePath); removeErr != nil {
			slog.Warn("failed to clean up workspace after error", "path", workspacePath, "error", removeErr)
		}
		return "", fmt.Errorf("rehydrate from base and diffs: %w", err)
	}

	slog.Info("workspace rehydrated successfully", "run_id", runID, "step_index", stepIndex, "workspace", workspacePath)

	// Create baseline commit after rehydration to enable incremental diffs.
	// C2: Now applies to ALL steps (including step 0) when diffs were applied.
	// This commit establishes a new HEAD so that "git diff HEAD" generates
	// only the changes from this step, not cumulative changes from prior steps.
	if len(gzippedDiffs) > 0 {
		if err := ensureBaselineCommitForRehydration(ctx, workspacePath, stepIndex); err != nil {
			if rmErr := os.RemoveAll(workspacePath); rmErr != nil {
				slog.Warn("failed to remove workspace after baseline commit error", "path", workspacePath, "error", rmErr)
			}
			return "", fmt.Errorf("create baseline commit for rehydration: %w", err)
		}
		slog.Info("baseline commit created for incremental diff", "run_id", runID, "step_index", stepIndex)
	}

	return workspacePath, nil
}

// uploadModDiffWithBaseline generates and uploads a diff for a mod job by comparing
// the pre-mod baseline snapshot with the post-mod workspace. This ensures that
// untracked files created by the mod are included in the patch (git diff --no-index
// semantics via GenerateBetween).
func (r *runController) uploadModDiffWithBaseline(
	ctx context.Context,
	runID types.RunID,
	jobID types.JobID,
	jobName string,
	diffGenerator step.DiffGenerator,
	baseDir string,
	workspace string,
	result step.Result,
	stepIndex types.StepIndex,
) {
	if diffGenerator == nil {
		return
	}

	// If no baseline snapshot is available, skip diff upload rather than
	// falling back to legacy baseline-less generation. Mod diffs must use
	// baseline-aware GenerateBetween semantics.
	if strings.TrimSpace(baseDir) == "" {
		slog.Warn("mod diff skipped: baseline snapshot missing", "run_id", runID, "job_id", jobID, "job_name", jobName, "step_index", stepIndex)
		return
	}

	// Generate diff between baseline snapshot and post-mod workspace so untracked
	// files are included in the patch (git diff --no-index semantics).
	diffBytes, err := diffGenerator.GenerateBetween(ctx, baseDir, workspace)
	if err != nil {
		slog.Error("failed to generate mod diff from baseline", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "error", err)
		return
	}

	if len(diffBytes) == 0 {
		// No changes between baseline and workspace; skip upload.
		slog.Info("no diff to upload for mod (no changes between baseline and workspace)", "run_id", runID, "job_id", jobID, "step_index", stepIndex)
		return
	}

	// Build diff summary with step metadata for database storage.
	// Uses typed builder to eliminate map[string]any construction.
	// Mod job diffs use DiffModTypeMod so they participate in the rehydration chain.
	summary := types.NewDiffSummaryBuilder().
		StepIndex(stepIndex).
		ModType(DiffModTypeMod.String()).
		ExitCode(result.ExitCode).
		Timings(
			time.Duration(result.Timings.HydrationDuration).Milliseconds(),
			time.Duration(result.Timings.ExecutionDuration).Milliseconds(),
			time.Duration(result.Timings.DiffDuration).Milliseconds(),
			time.Duration(result.Timings.TotalDuration).Milliseconds(),
		).
		MustBuild()

	// Ensure uploaders are initialized (lazy init for backward compatibility with tests).
	// In production, uploaders are pre-initialized at agent startup.
	if err := r.ensureUploaders(); err != nil {
		slog.Error("failed to initialize uploaders", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "error", err)
		return
	}

	if err := r.diffUploader.UploadDiff(ctx, runID, jobID, diffBytes, summary); err != nil {
		slog.Error("failed to upload mod diff", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "error", err)
		return
	}

	slog.Info("mod diff uploaded successfully", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "size", len(diffBytes))
}
