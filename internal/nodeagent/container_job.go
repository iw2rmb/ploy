// container_job.go contains mig container execution and workspace lifecycle helpers.
package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	gitpkg "github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// executeMigJob runs a mig container job.
// Executes the container, uploads diff, and reports status.
//
// Stack-aware image selection: The job loads the persisted stack from the
// pre-gate phase and uses it for manifest building. This ensures mig steps
// use stack-specific images (e.g., java-maven, java-gradle) when configured.
func (r *runController) executeMigJob(ctx context.Context, req StartRunRequest, mounts step.JobMounts) {
	startTime := time.Now()

	// Load the persisted stack from the pre-gate phase for stack-aware image
	// selection. If no stack was persisted (e.g., gate skipped), defaults to
	// MigStackUnknown which falls back to "default" in stack maps.
	stack := resolveManifestStack(req, r.loadPersistedStack(req.RunID))

	stepIdx := 0
	if req.MigSpec != nil && len(req.MigSpec.Steps) > 0 {
		if req.MigContext != nil {
			stepIdx = req.MigContext.StepIndex
		} else {
			idx, err := migStepIndexFromJobName(req.JobName, len(req.MigSpec.Steps))
			if err != nil {
				err = fmt.Errorf("derive mig step index from job_name: %w", err)
				slog.Error("failed to derive mig step index", "run_id", req.RunID, "job_id", req.JobID, "error", err)
				r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
				return
			}
			stepIdx = idx
		}
		if stepIdx < 0 || stepIdx >= len(req.MigSpec.Steps) {
			err := fmt.Errorf("derived mig step index out of range: derived=%d steps_len=%d", stepIdx, len(req.MigSpec.Steps))
			slog.Error("derived mig step index out of range", "run_id", req.RunID, "job_id", req.JobID, "derived_index", stepIdx, "steps_len", len(req.MigSpec.Steps))
			r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
			return
		}
	}
	manifest, err := buildMigManifest(req, stepIdx, stack)
	if err != nil {
		slog.Error("failed to build manifest", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	// Log the stack-aware image selection for observability.
	slog.Info("mig job using stack-aware image",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"detected_stack", stack,
		"resolved_image", manifest.Image,
	)

	r.executeMigContainerJob(ctx, req, manifest, startTime, mounts)
}

type migJobOutcome struct {
	runErr     error
	result     step.Result
	repoSHAOut string
	duration   time.Duration
}

func (r *runController) executeMigContainerJob(ctx context.Context, req StartRunRequest, manifest contracts.StepManifest, startTime time.Time, mounts step.JobMounts) {
	outcome, execErr := r.executeMigContainerWithOutcome(ctx, req, manifest, startTime, mounts)
	if execErr == nil {
		if shouldUploadRepoArtifactsAfterMigJob(req, outcome) {
			r.uploadRepoArtifactsIfPresent(req.RunID, req.RepoID, req.JobID)
		}
		return
	}
	slog.Error("mig job execution failed", "run_id", req.RunID, "job_id", req.JobID, "error", execErr)
	r.uploadRepoArtifactsIfPresent(req.RunID, req.RepoID, req.JobID)
	r.uploadFailureStatus(ctx, req, execErr, time.Since(startTime))
}

func shouldUploadRepoArtifactsAfterMigJob(req StartRunRequest, outcome migJobOutcome) bool {
	if outcome.runErr != nil || outcome.result.ExitCode != 0 {
		return true
	}
	return req.MigSpec != nil && req.MigSpec.BuildGate != nil && req.MigSpec.BuildGate.Disabled &&
		(req.NextID == nil || req.NextID.IsZero())
}

func (r *runController) executeMigContainerWithOutcome(ctx context.Context, req StartRunRequest, manifest contracts.StepManifest, startTime time.Time, mounts step.JobMounts) (migJobOutcome, error) {
	var outcome migJobOutcome
	cleanupReportAccess, err := r.configureJobReportAccess(&manifest, mounts.Staging)
	if err != nil {
		return outcome, err
	}
	defer cleanupReportAccess()

	jobDirs := jobDirectories(req.RunID, req.JobID)

	execCtx, cleanup, err := r.initExecutionContext(ctx, req.RunID, req.JobID)
	if err != nil {
		return outcome, fmt.Errorf("initialize runtime: %w", err)
	}
	defer cleanup()
	artifactLogs, err := newArtifactLogWriter(execCtx.logStreamer, jobDirs)
	if err != nil {
		return outcome, fmt.Errorf("prepare job artifact logs: %w", err)
	}
	execCtx.runner.LogWriter = artifactLogs
	defer func() {
		if err := artifactLogs.Close(); err != nil {
			slog.Warn("failed to close job artifact logs", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		}
	}()

	hydrationStart := time.Now()
	wsResult, err := r.prepareStickyWorkspaceWithCleanup(ctx, req, manifest)
	if err != nil {
		return outcome, fmt.Errorf("prepare sticky workspace: %w", err)
	}
	hydrationDuration := time.Since(hydrationStart)
	defer wsResult.cleanup()
	workspace := wsResult.path

	stepOutcome, err := r.runMigContainerJob(ctx, req, manifest, execCtx, workspace, startTime, hydrationDuration, jobDirs, mounts)
	if err != nil {
		return outcome, err
	}
	outcome = stepOutcome
	return outcome, nil
}

func (r *runController) runMigContainerJob(
	ctx context.Context,
	req StartRunRequest,
	manifest contracts.StepManifest,
	execCtx executionContext,
	workspace string,
	startTime time.Time,
	hydrationDuration time.Duration,
	jobDirs JobDirectories,
	mounts step.JobMounts,
) (migJobOutcome, error) {
	outcome := migJobOutcome{}
	diffPath := jobDirs.Diff
	imageName := strings.TrimSpace(manifest.Image)
	if imageName == "" {
		return outcome, fmt.Errorf("resolved job image is empty")
	}
	if err := r.SaveJobImageName(ctx, req.JobID, imageName); err != nil {
		return outcome, fmt.Errorf("save job image name: %w", err)
	}

	result := step.Result{Timings: step.StageTiming{
		HydrationDuration: types.Duration(hydrationDuration),
	}}
	var runErr error

	preWorkspaceTree := ""
	if tree, treeErr := gitpkg.ComputeWorkspaceTreeSHA(ctx, workspace); treeErr != nil {
		return outcome, fmt.Errorf("compute pre-execution workspace tree: %w", treeErr)
	} else {
		preWorkspaceTree = tree
	}

	_, err := r.materializeJobResources(ctx, manifest, manifest.BundleMap, mounts.Staging)
	if err != nil {
		return outcome, err
	}

	// Materialized inputs and writable temporary state stay below the job root.
	result, runErr = execCtx.runner.Run(ctx, step.Request{
		RunID:     req.RunID,
		JobID:     req.JobID,
		Manifest:  manifest,
		Workspace: workspace,
		JobMounts: mounts,
	})
	result.Timings.HydrationDuration = types.Duration(hydrationDuration)
	duration := time.Since(startTime)
	result.Timings.TotalDuration = types.Duration(duration)
	if runErr != nil || result.ExitCode != 0 {
		persistContainerInspectArtifact(req, jobDirs, result)
	}

	diffUploaded, diffErr := r.uploadDiff(
		ctx,
		req.RunID,
		req.JobID,
		execCtx.diffGenerator,
		workspace,
		result,
		types.DiffJobTypeMig,
		diffPath,
	)
	if diffErr != nil && runErr == nil && result.ExitCode == 0 {
		runErr = fmt.Errorf("upload job diff: %w", diffErr)
	}

	if runErr == nil && result.ExitCode == 0 {
		if err := advanceWorkspaceBaseline(ctx, workspace, req.RunID, req.JobID, diffUploaded); err != nil {
			runErr = fmt.Errorf("advance workspace baseline: %w", err)
			slog.Error("failed to advance workspace baseline", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		}
	}

	repoSHAOut := ""
	if runErr == nil && result.ExitCode == 0 {
		var repoSHAErr error
		repoSHAOut, repoSHAErr = r.computeRepoSHAOut(ctx, req, workspace, preWorkspaceTree)
		if repoSHAErr != nil {
			runErr = repoSHAErr
			slog.Error("failed to compute repo_sha_out", "run_id", req.RunID, "job_id", req.JobID, "error", repoSHAErr)
		}
	}

	statsBuilder := types.NewRunStatsBuilder().
		ExitCode(result.ExitCode).
		DurationMs(duration.Milliseconds()).
		TimingsFromDurations(
			time.Duration(result.Timings.HydrationDuration).Milliseconds(),
			time.Duration(result.Timings.ExecutionDuration).Milliseconds(),
			0,
			time.Duration(result.Timings.TotalDuration).Milliseconds(),
		)
	if result.ContainerResources != nil {
		statsBuilder.JobResources(result.ContainerResources)
	}

	if runErr != nil {
		statsBuilder.Error(normalizedExecutionError(runErr))
	} else if result.ExitCode != 0 {
		statsBuilder.Error(deriveContainerExitError(req, result, jobDirs))
	}

	stats := statsBuilder.MustBuild()
	outcome = migJobOutcome{
		runErr:     runErr,
		result:     result,
		repoSHAOut: repoSHAOut,
		duration:   duration,
	}
	r.reportTerminalStatus(ctx, req, runErr, result, stats, repoSHAOut, duration)
	return outcome, nil
}

// materializeJobResources returns an empty staging path when the manifest has
// no Hydra resources, preserving the existing optional mount contract.
func (r *runController) materializeJobResources(ctx context.Context, manifest contracts.StepManifest, bundleMap map[string]string, stagingDir string) (string, error) {
	hashes := collectUniqueHashes(manifest)
	if len(hashes) == 0 {
		return "", nil
	}
	if err := r.materializeHydraResources(ctx, manifest, bundleMap, stagingDir); err != nil {
		return "", fmt.Errorf("materialize hydra resources: %w", err)
	}
	return stagingDir, nil
}

// tempResource holds a temporary path and its cleanup function.
// Used for workspace snapshots, sticky workspaces, and similar lifecycle-scoped directories.
type tempResource struct {
	path    string
	cleanup func()
}

// prepareStickyWorkspaceWithCleanup wraps prepareStickyWorkspace and returns a no-op
// cleanup for sticky run workspaces.
func (r *runController) prepareStickyWorkspaceWithCleanup(
	ctx context.Context,
	req StartRunRequest,
	manifest contracts.StepManifest,
) (tempResource, error) {
	workspace, err := r.prepareStickyWorkspace(ctx, req, manifest)
	if err != nil {
		return tempResource{}, err
	}

	return tempResource{
		path:    workspace,
		cleanup: func() {},
	}, nil
}
