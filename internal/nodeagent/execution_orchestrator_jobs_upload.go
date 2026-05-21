// execution_orchestrator_jobs_upload.go contains upload, status reporting,
// diff generation, and artifact helpers used by job executors.
package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	gitpkg "github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// uploadStatus uploads terminal status and execution statistics to the control plane.
// Uses a detached context to ensure reporting even if the run context is cancelled.
func (r *runController) uploadStatus(ctx context.Context, runID, status string, exitCode *int32, stats types.RunStats, jobID types.JobID, repoSHAOut ...string) error {
	var loggedExitCode any
	if exitCode != nil {
		loggedExitCode = *exitCode
	}
	loggedRepoSHAOut := ""
	if len(repoSHAOut) > 0 {
		loggedRepoSHAOut = strings.TrimSpace(repoSHAOut[0])
	}

	statusCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if uploadErr := r.statusUploader.UploadJobStatus(statusCtx, jobID, status, exitCode, stats, loggedRepoSHAOut); uploadErr != nil {
		return fmt.Errorf("upload job status: %w", uploadErr)
	}

	slog.Info("terminal status uploaded successfully", "run_id", runID, "job_id", jobID, "status", status, "exit_code", loggedExitCode, "repo_sha_out", loggedRepoSHAOut)
	return nil
}

// reportTerminalStatus uploads the final job status based on execution outcome.
// Handles runtime errors and maps process exit code to terminal status.
func (r *runController) reportTerminalStatus(
	ctx context.Context,
	req StartRunRequest,
	runErr error,
	result step.Result,
	stats types.RunStats,
	repoSHAOut string,
	duration time.Duration,
) {
	var status types.JobStatus
	var exitCode *int32

	if runErr != nil {
		status = lifecycle.JobStatusFromRunError(runErr)
		if status == types.JobStatusError {
			v := int32(-1)
			exitCode = &v
		}
		r.emitRunException(req, "node runtime execution error", runErr, map[string]any{
			"component": "run_controller", "status": status.String(), "duration_ms": duration.Milliseconds(),
		})
	} else {
		status = lifecycle.JobStatusFromExitCodeForJobType(req.JobType, result.ExitCode)
		ec := int32(result.ExitCode)
		exitCode = &ec
	}

	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), status.String(), exitCode, stats, req.JobID, repoSHAOut); uploadErr != nil {
		slog.Error("failed to upload terminal status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	} else {
		r.cleanupRunRepoShareOnTerminalSuccess(req, status)
	}
	slog.Info("job terminated", "run_id", req.RunID, "job_id", req.JobID, "status", status,
		"exit_code", result.ExitCode, "duration", duration)
}

func (r *runController) cleanupRunRepoShareOnTerminalSuccess(req StartRunRequest, status types.JobStatus) {
	_ = req
	_ = status
}

func (r *runController) computeRepoSHAOut(ctx context.Context, req StartRunRequest, workspace string, inputTree string) (string, error) {
	repoSHAIn := strings.TrimSpace(req.RepoSHAIn.String())
	if repoSHAIn == "" {
		return "", fmt.Errorf("repo_sha_in missing on claimed job")
	}
	preTree := strings.TrimSpace(inputTree)
	repoSHAOut, err := gitpkg.ComputeRepoSHAV1(ctx, workspace, repoSHAIn, preTree)
	if err != nil {
		return "", fmt.Errorf("compute repo_sha_out: %w", err)
	}
	return repoSHAOut, nil
}

// uploadJobDiff is the shared implementation for generating, summarizing, and uploading
// a diff for the current workspace state.
func (r *runController) uploadJobDiff(
	ctx context.Context,
	runID types.RunID,
	jobID types.JobID,
	diffGenerator step.DiffGenerator,
	workspace string,
	result step.Result,
	diffType types.DiffJobType,
	diffPath string,
) (bool, error) {
	if diffGenerator == nil {
		return false, nil
	}

	label := diffType.String()

	diffBytes, err := diffGenerator.Generate(ctx, workspace)
	if err != nil {
		slog.Error("failed to generate "+label+" diff", "run_id", runID, "job_id", jobID, "error", err)
		return false, err
	}
	if len(diffBytes) == 0 {
		slog.Info("no diff to upload (no workspace changes)", "run_id", runID, "job_id", jobID, "diff_type", label)
		if strings.TrimSpace(diffPath) != "" {
			if err := os.Remove(diffPath); err != nil && !os.IsNotExist(err) {
				slog.Warn("failed to remove empty diff artifact", "run_id", runID, "job_id", jobID, "path", diffPath, "error", err)
			}
		}
		return false, nil
	}
	if strings.TrimSpace(diffPath) != "" {
		if err := os.WriteFile(diffPath, diffBytes, 0o600); err != nil {
			return false, fmt.Errorf("write diff artifact %s: %w", diffPath, err)
		}
	}

	patchStats := step.CountPatchStats(diffBytes)
	summary := types.NewDiffSummaryBuilder().
		JobType(label).
		ExitCode(result.ExitCode).
		FilesChanged(patchStats.FilesChanged).
		LinesAdded(patchStats.LinesAdded).
		LinesRemoved(patchStats.LinesRemoved).
		Timings(
			time.Duration(result.Timings.HydrationDuration).Milliseconds(),
			time.Duration(result.Timings.ExecutionDuration).Milliseconds(),
			time.Duration(result.Timings.DiffDuration).Milliseconds(),
			time.Duration(result.Timings.TotalDuration).Milliseconds(),
		).
		MustBuild()

	if err := r.diffUploader.UploadDiff(ctx, runID, jobID, diffBytes, summary); err != nil {
		slog.Error("failed to upload "+label+" diff", "run_id", runID, "job_id", jobID, "error", err)
		return false, err
	}

	slog.Info(label+" diff uploaded successfully", "run_id", runID, "job_id", jobID, "size", len(diffBytes))
	return true, nil
}
