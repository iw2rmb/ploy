package nodeagent

import (
	"context"
	"log/slog"
	"os"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// executeMRJob runs a post-run MR creation job.
// It rehydrates the final workspace state for the run and invokes createMR
// using GitLab options from the manifest. MR jobs are best-effort: failures
// are surfaced via logs and job status but must not affect the run's
// terminal status (handled by the control plane).
func (r *runController) executeMRJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	// Parse options and build a minimal manifest so that GitLab options
	// (gitlab_pat, gitlab_domain, mr_on_success/mr_on_fail) are available.
	typedOpts := parseRunOptions(req.Options)
	manifest, err := buildManifestFromRequest(req, typedOpts, 0, contracts.ModStackUnknown)
	if err != nil {
		slog.Error("failed to build manifest for MR job", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	// Rehydrate workspace from base + diffs at the MR job's step index.
	workspace, err := r.rehydrateWorkspaceForStep(ctx, req, manifest, req.StepIndex)
	if err != nil {
		slog.Error("failed to rehydrate workspace for MR job", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer func() { _ = os.RemoveAll(workspace) }()

	slog.Info("starting MR job execution",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"step_index", req.StepIndex,
	)

	// Attempt MR creation using the hydrated workspace.
	mrURL, mrErr := r.createMR(ctx, req, manifest, workspace)
	duration := time.Since(startTime)

	stats := types.RunStats{
		"duration_ms": duration.Milliseconds(),
	}
	if mrURL != "" {
		stats["metadata"] = map[string]any{"mr_url": mrURL}
	}

	if mrErr != nil {
		// MR jobs are best-effort; surface failure via job status and logs.
		stats["error"] = mrErr.Error()
		var exitCode int32 = -1
		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "failed", &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
			slog.Error("failed to upload MR job failure status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
		}
		slog.Warn("MR job failed", "run_id", req.RunID, "job_id", req.JobID, "error", mrErr, "duration", duration)
		return
	}

	var exitCodeZero int32 = 0
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "succeeded", &exitCodeZero, stats, req.StepIndex, req.JobID); uploadErr != nil {
		slog.Error("failed to upload MR job success status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	}
	slog.Info("MR job succeeded", "run_id", req.RunID, "job_id", req.JobID, "mr_url", mrURL, "duration", duration)
}
