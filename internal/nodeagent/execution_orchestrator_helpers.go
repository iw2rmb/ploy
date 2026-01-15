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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

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

// standardJobConfig configures the execution of a standard container job (mod/heal).
type standardJobConfig struct {
	// Manifest is the step manifest to execute.
	Manifest contracts.StepManifest
	// DiffType specifies the type of diff to generate (Mod/Heal).
	DiffType DiffModType
	// OutDirPattern is the pattern for the temporary /out directory (e.g., "ploy-mod-out-*").
	OutDirPattern string
	// InDirPattern is the pattern for the temporary /in directory. Optional.
	InDirPattern string

	// PopulateInDir is an optional hook to populate the /in directory.
	PopulateInDir func(inDir string) error
	// InjectEnv is an optional hook to inject environment variables into the manifest.
	InjectEnv func(manifest *contracts.StepManifest, workspace string)
	// MountCerts is an optional hook to mount TLS certificates.
	MountCerts func(manifest *contracts.StepManifest)

	// CheckWorkspaceNoChange enables pre/post execution workspace status checks.
	// Used by healing jobs to detect "no change" failures.
	CheckWorkspaceNoChange bool
	// UploadConfiguredArtifacts enables uploading artifacts defined in RunOptions.
	UploadConfiguredArtifacts bool

	// UploadDiff is the strategy for uploading diffs.
	UploadDiff func(ctx context.Context, runID types.RunID, jobID types.JobID, jobName string, diffGen step.DiffGenerator, baselineDir, workspace string, result step.Result, stepIndex types.StepIndex)

	// StartTime is the time when the job execution started (including manifest build).
	// If zero, defaults to time.Now() inside executeStandardJob.
	StartTime time.Time
}

// executeStandardJob orchestrates the common lifecycle of a container job (mod/heal).
// It handles runtime init, rehydration, snapshots, directory prep, execution, and uploading.
func (r *runController) executeStandardJob(ctx context.Context, req StartRunRequest, cfg standardJobConfig) {
	startTime := cfg.StartTime
	if startTime.IsZero() {
		startTime = time.Now()
	}

	// Initialize runtime components using shared helper.
	execCtx, cleanup, err := r.initJobExecutionContext(ctx, req.RunID, req.JobID)
	if err != nil {
		slog.Error("failed to initialize runtime", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer cleanup()

	// Rehydrate workspace.
	wsResult, err := r.rehydrateWorkspaceWithCleanup(ctx, req, cfg.Manifest, req.StepIndex)
	if err != nil {
		slog.Error("failed to rehydrate workspace", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer wsResult.cleanup()
	workspace := wsResult.workspace

	// Snapshot baseline for diffs.
	var baselineDir string
	if execCtx.diffGenerator != nil {
		snapshot := snapshotWorkspaceForNoIndexDiff(req.RunID, req.JobID, cfg.DiffType, workspace)
		defer snapshot.cleanup()
		baselineDir = snapshot.dir
	}

	// Define the execution body to be wrapped in temp dir context.
	executeBody := func(outDir, inDir string) error {
		// Prepare manifest copy for this execution.
		manifest := cfg.Manifest
		disableManifestGate(&manifest)
		clearManifestHydration(&manifest)

		// Apply hooks.
		if cfg.InjectEnv != nil {
			cfg.InjectEnv(&manifest, workspace)
		}
		if cfg.MountCerts != nil {
			cfg.MountCerts(&manifest)
		}

		// Pre-run workspace status check (for healing no-change detection).
		var preStatus string
		var preStatusErr error
		if cfg.CheckWorkspaceNoChange {
			preStatus, preStatusErr = workspaceStatus(ctx, workspace)
			if preStatusErr != nil {
				slog.Warn("failed to compute workspace status before execution", "run_id", req.RunID, "error", preStatusErr)
			}
		}

		// Run container.
		result, runErr := execCtx.runner.Run(ctx, step.Request{
			RunID:     req.RunID,
			Manifest:  manifest,
			Workspace: workspace,
			OutDir:    outDir,
			InDir:     inDir,
		})
		duration := time.Since(startTime)

		// Upload diff.
		if cfg.UploadDiff != nil {
			cfg.UploadDiff(ctx, req.RunID, req.JobID, req.JobName, execCtx.diffGenerator, baselineDir, workspace, result, req.StepIndex)
		}

		// Upload /out artifacts.
		if err := r.uploadOutDir(ctx, req.RunID, req.JobID, outDir); err != nil {
			slog.Warn("/out artifact upload failed", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "error", err)
		}

		// Upload configured artifacts (Mod only).
		if cfg.UploadConfiguredArtifacts {
			r.uploadConfiguredArtifacts(ctx, req, req.TypedOptions, manifest, workspace)
		}

		// Post-run workspace status check (for healing).
		if cfg.CheckWorkspaceNoChange && runErr == nil && result.ExitCode == 0 && preStatusErr == nil {
			postStatus, postErr := workspaceStatus(ctx, workspace)
			if postErr == nil && postStatus == preStatus {
				// No change failure.
				r.uploadHealingNoWorkspaceChangesFailure(ctx, req, types.NewRunStatsBuilder().ExitCode(1).DurationMs(duration.Milliseconds()).HealingWarning("no_workspace_changes").MustBuild(), duration)
				return nil
			}
		}

		// Build stats.
		statsBuilder := types.NewRunStatsBuilder().
			ExitCode(result.ExitCode).
			DurationMs(duration.Milliseconds()).
			TimingsFromDurations(
				time.Duration(result.Timings.HydrationDuration).Milliseconds(),
				time.Duration(result.Timings.ExecutionDuration).Milliseconds(),
				time.Duration(result.Timings.DiffDuration).Milliseconds(),
				time.Duration(result.Timings.TotalDuration).Milliseconds(),
			)
		stats := statsBuilder.MustBuild()

		// Handle terminal status.
		if runErr != nil {
			status := JobStatusFail
			var exitCode *int32
			if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
				status = JobStatusCancelled
			} else {
				var runtimeExitCode int32 = -1
				exitCode = &runtimeExitCode
			}
			if uploadErr := r.uploadStatus(ctx, req.RunID.String(), status.String(), exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
				slog.Error("failed to upload terminal status", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "error", uploadErr)
			}
			slog.Info("job terminated", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "status", status, "duration", duration, "error", runErr)
			return nil
		}

		if result.ExitCode != 0 {
			exitCode := int32(result.ExitCode)
			if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusFail.String(), &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
				slog.Error("failed to upload failure status", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "error", uploadErr)
			}
			slog.Info("job failed", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "exit_code", result.ExitCode, "duration", duration)
			return nil
		}

		var exitCodeZero int32 = 0
		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusSuccess.String(), &exitCodeZero, stats, req.StepIndex, req.JobID); uploadErr != nil {
			slog.Error("failed to upload success status", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "error", uploadErr)
		}
		slog.Info("job succeeded", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "duration", duration)
		return nil
	}

	// Wrapper logic for optional nested temp dirs.
	outDirErr := withTempDir(cfg.OutDirPattern, func(outDir string) error {
		if cfg.InDirPattern != "" {
			return withTempDir(cfg.InDirPattern, func(inDir string) error {
				if cfg.PopulateInDir != nil {
					if err := cfg.PopulateInDir(inDir); err != nil {
						slog.Warn("failed to populate in dir", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "error", err)
					}
				}
				return executeBody(outDir, inDir)
			})
		}
		return executeBody(outDir, "")
	})

	if outDirErr != nil {
		slog.Error("failed to create temp directories", "run_id", req.RunID, "error", outDirErr)
		r.uploadFailureStatus(ctx, req, outDirErr, time.Since(startTime))
	}
}
