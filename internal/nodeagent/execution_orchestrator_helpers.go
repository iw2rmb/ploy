// execution_orchestrator_helpers.go contains shared scaffolding helpers for mod/heal job execution.
//
// These helpers extract common patterns from executeModJob and executeHealingJob to:
//   - Reduce code duplication and risk of divergent behavior
//   - Make future changes cheaper by having a single implementation
//   - Keep job-specific behavior (e.g., /in hydration for heal) in respective functions
//
// Extracted patterns:
//   - jobExecutionContext: runtime init + logStreamer lifecycle
//   - withTempDir: temp directory creation + cleanup
//   - snapshotWorkspaceForNoIndexDiff: baseline snapshot for diff generation
package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// jobExecutionContext holds runtime components initialized for a mod/heal job.
// It bundles the runner, diff generator, and log streamer lifecycle together
// so callers can initialize once and defer cleanup consistently.
//
// Usage pattern:
//
//	ctx, cleanup, err := r.initJobExecutionContext(ctx, req.RunID, req.JobID)
//	if err != nil { /* handle error */ }
//	defer cleanup()
//	// use ctx.runner, ctx.diffGenerator
type jobExecutionContext struct {
	// runner is the step.Runner configured with workspace, container, diff, and gate components.
	runner step.Runner

	// diffGenerator generates diffs between workspace states. May be nil if unavailable.
	diffGenerator step.DiffGenerator

	// logStreamer streams logs to the server. Caller must call cleanup to close it.
	logStreamer *LogStreamer
}

// initJobExecutionContext initializes runtime components for job execution and returns
// a cleanup function that must be deferred by the caller.
//
// This extracts the common init + defer pattern from executeModJob/executeHealingJob:
//
//	runner, diffGenerator, logStreamer, err := r.initializeRuntime(ctx, runID, jobID)
//	if err != nil { ... }
//	defer func() { _ = logStreamer.Close() }()
//
// Parameters:
//   - ctx: context for initialization operations
//   - runID: run identifier for logging and telemetry
//   - jobID: job identifier for associating log chunks with specific jobs
//
// Returns:
//   - ctx: jobExecutionContext with runner, diffGenerator, logStreamer
//   - cleanup: function to close logStreamer (must be deferred)
//   - err: non-nil if runtime initialization fails
func (r *runController) initJobExecutionContext(ctx context.Context, runID types.RunID, jobID types.JobID) (jobExecutionContext, func(), error) {
	runner, diffGenerator, logStreamer, err := r.initializeRuntime(ctx, runID, jobID)
	if err != nil {
		return jobExecutionContext{}, nil, err
	}

	execCtx := jobExecutionContext{
		runner:        runner,
		diffGenerator: diffGenerator,
		logStreamer:   logStreamer,
	}

	// Cleanup function closes logStreamer.
	cleanup := func() {
		if err := logStreamer.Close(); err != nil {
			slog.Warn("failed to close log streamer", "run_id", runID, "job_id", jobID, "error", err)
		}
	}

	return execCtx, cleanup, nil
}

// withTempDir creates a temporary directory with the given prefix and calls fn with its path.
// The directory is automatically removed after fn returns (regardless of success/failure).
//
// This extracts the common pattern for /out directory handling:
//
//	outDir, err := os.MkdirTemp("", prefix)
//	if err != nil { /* handle */ }
//	defer func() { _ = os.RemoveAll(outDir) }()
//	// use outDir
//
// Parameters:
//   - prefix: temp directory prefix (e.g., "ploy-mod-out-*", "ploy-heal-out-*")
//   - fn: function to execute with the temp directory path
//
// Returns error from MkdirTemp or from fn; cleanup happens in either case.
func withTempDir(prefix string, fn func(path string) error) error {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return fmt.Errorf("create temp dir %s: %w", prefix, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("failed to remove temp dir", "path", dir, "error", err)
		}
	}()

	return fn(dir)
}

// snapshotResult holds the result of snapshotWorkspaceForNoIndexDiff.
type snapshotResult struct {
	// dir is the path to the snapshot directory containing the workspace copy.
	// Empty string if snapshotting failed or was skipped.
	dir string

	// cleanup removes the snapshot directory. Safe to call even if dir is empty.
	cleanup func()
}

// snapshotWorkspaceForNoIndexDiff creates a snapshot of the workspace for baseline comparison.
// This enables "git diff --no-index" semantics via GenerateBetween, which captures untracked
// files in the diff (unlike HEAD-based git diff).
//
// This extracts the common pre-mod/pre-heal baseline snapshot pattern:
//
//	var baselineDir string
//	if diffGenerator != nil {
//	    snapshotDir, snapErr := os.MkdirTemp("", prefix)
//	    if snapErr != nil { /* warn */ }
//	    else if err := copyGitClone(workspace, snapshotDir); err != nil { /* warn, cleanup */ }
//	    else { baselineDir = snapshotDir; defer os.RemoveAll }
//	}
//
// Parameters:
//   - runID: run identifier for logging
//   - jobID: job identifier for logging
//   - diffType: DiffModTypeMod or DiffModTypeHealing for log message context
//   - workspace: source workspace to snapshot
//
// Returns snapshotResult with:
//   - dir: path to snapshot, or empty if failed/skipped
//   - cleanup: function to remove snapshot (safe to call even if empty)
//
// Note: Caller must defer cleanup() to avoid leaking temp directories.
func snapshotWorkspaceForNoIndexDiff(runID types.RunID, jobID types.JobID, diffType DiffModType, workspace string) snapshotResult {
	jobTypeStr := diffType.String()
	prefix := fmt.Sprintf("ploy-%s-base-*", jobTypeStr)
	snapshotDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s: failed to create baseline snapshot directory", jobTypeStr),
			"run_id", runID, "job_id", jobID, "error", err)
		return snapshotResult{dir: "", cleanup: func() {}}
	}

	if err := copyGitClone(workspace, snapshotDir); err != nil {
		slog.Warn(fmt.Sprintf("%s: failed to snapshot baseline workspace", jobTypeStr),
			"run_id", runID, "job_id", jobID, "error", err)
		if rmErr := os.RemoveAll(snapshotDir); rmErr != nil {
			slog.Warn("failed to remove snapshot dir after copy failure", "path", snapshotDir, "error", rmErr)
		}
		return snapshotResult{dir: "", cleanup: func() {}}
	}

	return snapshotResult{
		dir: snapshotDir,
		cleanup: func() {
			if err := os.RemoveAll(snapshotDir); err != nil {
				slog.Warn("failed to remove snapshot dir", "path", snapshotDir, "error", err)
			}
		},
	}
}

// workspaceRehydrationResult holds the result of workspace rehydration.
type workspaceRehydrationResult struct {
	// workspace is the path to the rehydrated workspace.
	workspace string

	// cleanup removes the workspace directory. Safe to call even if workspace is empty.
	cleanup func()
}

// rehydrateWorkspaceWithCleanup is a wrapper around rehydrateWorkspaceForStep that bundles
// the workspace path with its cleanup function.
//
// This extracts the common pattern:
//
//	workspace, err := r.rehydrateWorkspaceForStep(ctx, req, manifest, stepIndex)
//	if err != nil { /* handle */ }
//	defer func() { _ = os.RemoveAll(workspace) }()
//
// Returns workspaceRehydrationResult with:
//   - workspace: path to rehydrated workspace
//   - cleanup: function to remove workspace (must be deferred)
//
// Note: Caller must defer cleanup() to avoid leaking workspaces.
func (r *runController) rehydrateWorkspaceWithCleanup(
	ctx context.Context,
	req StartRunRequest,
	manifest contracts.StepManifest,
	stepIndex types.StepIndex,
) (workspaceRehydrationResult, error) {
	workspace, err := r.rehydrateWorkspaceForStep(ctx, req, manifest, stepIndex)
	if err != nil {
		return workspaceRehydrationResult{}, err
	}

	return workspaceRehydrationResult{
		workspace: workspace,
		cleanup: func() {
			if err := os.RemoveAll(workspace); err != nil {
				slog.Warn("failed to remove workspace", "path", workspace, "error", err)
			}
		},
	}, nil
}

// clearManifestHydration removes hydration config from manifest inputs.
// This is used when workspace is already hydrated, to prevent double-hydration.
//
// This extracts the common pattern from executeModJob/executeHealingJob:
//
//	if len(manifest.Inputs) > 0 {
//	    inputs := make([]contracts.StepInput, len(manifest.Inputs))
//	    copy(inputs, manifest.Inputs)
//	    for i := range inputs {
//	        inputs[i].Hydration = nil
//	    }
//	    manifest.Inputs = inputs
//	}
func clearManifestHydration(manifest *contracts.StepManifest) {
	if len(manifest.Inputs) == 0 {
		return
	}
	inputs := make([]contracts.StepInput, len(manifest.Inputs))
	copy(inputs, manifest.Inputs)
	for i := range inputs {
		inputs[i].Hydration = nil
	}
	manifest.Inputs = inputs
}

// disableManifestGate sets Gate.Enabled=false on the manifest.
// Mod and healing jobs don't run gates; gates are handled by dedicated gate jobs.
func disableManifestGate(manifest *contracts.StepManifest) {
	manifest.Gate = &contracts.StepGateSpec{Enabled: false}
}
