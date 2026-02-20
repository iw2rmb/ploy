package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// jobExecutionContext holds runtime components initialized for a mod/heal job.
type jobExecutionContext struct {
	runner        step.Runner
	diffGenerator step.DiffGenerator
	logStreamer   *LogStreamer
}

// initJobExecutionContext initializes runtime components and returns a cleanup function
// that closes the logStreamer (must be deferred).
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

	cleanup := func() {
		if err := logStreamer.Close(); err != nil {
			slog.Warn("failed to close log streamer", "run_id", runID, "job_id", jobID, "error", err)
		}
	}

	return execCtx, cleanup, nil
}

// withTempDir creates a temporary directory, calls fn, then removes the directory.
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

// snapshotResult holds a workspace snapshot path and its cleanup function.
type snapshotResult struct {
	dir     string
	cleanup func()
}

// snapshotWorkspaceForNoIndexDiff creates a snapshot of the workspace for baseline comparison.
// This enables "git diff --no-index" semantics via GenerateBetween, which captures untracked files.
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

// workspaceRehydrationResult holds a rehydrated workspace path and its cleanup function.
type workspaceRehydrationResult struct {
	workspace string
	cleanup   func()
}

// rehydrateWorkspaceWithCleanup wraps rehydrateWorkspaceForStep with automatic cleanup.
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

// clearManifestHydration removes hydration config from manifest inputs to prevent double-hydration.
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
func disableManifestGate(manifest *contracts.StepManifest) {
	manifest.Gate = &contracts.StepGateSpec{Enabled: false}
}

// standardJobConfig configures the execution of a standard container job (mod/heal).
type standardJobConfig struct {
	Manifest      contracts.StepManifest
	DiffType      DiffModType
	OutDirPattern string
	InDirPattern  string

	PopulateInDir func(inDir string) error
	InjectEnv     func(manifest *contracts.StepManifest, workspace string)
	MountCerts    func(manifest *contracts.StepManifest)

	CheckWorkspaceNoChange    bool
	UploadConfiguredArtifacts bool

	UploadDiff   func(ctx context.Context, runID types.RunID, jobID types.JobID, jobName string, diffGen step.DiffGenerator, baselineDir, workspace string, result step.Result, stepIndex types.StepIndex)
	BuildJobMeta func(outDir string) json.RawMessage

	StartTime time.Time
}

// executeStandardJob orchestrates the common lifecycle of a container job (mod/heal):
// runtime init, rehydration, snapshots, directory prep, execution, and uploading.
func (r *runController) executeStandardJob(ctx context.Context, req StartRunRequest, cfg standardJobConfig) {
	startTime := cfg.StartTime
	if startTime.IsZero() {
		startTime = time.Now()
	}

	execCtx, cleanup, err := r.initJobExecutionContext(ctx, req.RunID, req.JobID)
	if err != nil {
		slog.Error("failed to initialize runtime", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer cleanup()

	wsResult, err := r.rehydrateWorkspaceWithCleanup(ctx, req, cfg.Manifest, req.StepIndex)
	if err != nil {
		slog.Error("failed to rehydrate workspace", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer wsResult.cleanup()
	workspace := wsResult.workspace

	var baselineDir string
	if execCtx.diffGenerator != nil {
		snapshot := snapshotWorkspaceForNoIndexDiff(req.RunID, req.JobID, cfg.DiffType, workspace)
		defer snapshot.cleanup()
		baselineDir = snapshot.dir
	}

	executeBody := func(outDir, inDir string) error {
		manifest := cfg.Manifest
		disableManifestGate(&manifest)
		clearManifestHydration(&manifest)

		if cfg.InjectEnv != nil {
			cfg.InjectEnv(&manifest, workspace)
		}
		if cfg.MountCerts != nil {
			cfg.MountCerts(&manifest)
		}

		imageName := strings.TrimSpace(manifest.Image)
		if imageName == "" {
			return fmt.Errorf("resolved job image is empty")
		}
		if err := r.SaveJobImageName(ctx, req.JobID, imageName); err != nil {
			return fmt.Errorf("save job image name: %w", err)
		}

		var preStatus string
		var preStatusErr error
		if cfg.CheckWorkspaceNoChange {
			preStatus, preStatusErr = workspaceStatus(ctx, workspace)
			if preStatusErr != nil {
				slog.Warn("failed to compute workspace status before execution", "run_id", req.RunID, "error", preStatusErr)
			}
		}

		result, runErr := execCtx.runner.Run(ctx, step.Request{
			RunID:     req.RunID,
			Manifest:  manifest,
			Workspace: workspace,
			OutDir:    outDir,
			InDir:     inDir,
		})
		duration := time.Since(startTime)

		if cfg.UploadDiff != nil {
			cfg.UploadDiff(ctx, req.RunID, req.JobID, req.JobName, execCtx.diffGenerator, baselineDir, workspace, result, req.StepIndex)
		}

		if err := r.uploadOutDir(ctx, req.RunID, req.JobID, outDir); err != nil {
			slog.Warn("/out artifact upload failed", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex, "error", err)
		}

		if cfg.UploadConfiguredArtifacts {
			r.uploadConfiguredArtifacts(ctx, req, req.TypedOptions, manifest, workspace)
		}

		if cfg.CheckWorkspaceNoChange && runErr == nil && result.ExitCode == 0 && preStatusErr == nil {
			postStatus, postErr := workspaceStatus(ctx, workspace)
			if postErr == nil && postStatus == preStatus {
				r.uploadHealingNoWorkspaceChangesFailure(ctx, req, types.NewRunStatsBuilder().ExitCode(1).DurationMs(duration.Milliseconds()).HealingWarning("no_workspace_changes").MustBuild(), duration)
				return nil
			}
		}

		statsBuilder := types.NewRunStatsBuilder().
			ExitCode(result.ExitCode).
			DurationMs(duration.Milliseconds()).
			TimingsFromDurations(
				time.Duration(result.Timings.HydrationDuration).Milliseconds(),
				time.Duration(result.Timings.ExecutionDuration).Milliseconds(),
				time.Duration(result.Timings.DiffDuration).Milliseconds(),
				time.Duration(result.Timings.TotalDuration).Milliseconds(),
			)

		if cfg.BuildJobMeta != nil {
			if meta := cfg.BuildJobMeta(outDir); len(meta) > 0 {
				statsBuilder.JobMeta(meta)
			}
		}

		stats := statsBuilder.MustBuild()

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
