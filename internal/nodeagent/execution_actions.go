package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func (r *runController) executeAction(ctx context.Context, req StartActionRequest) {
	defer func() {
		r.mu.Lock()
		delete(r.jobs, req.ActionID)
		r.mu.Unlock()
		r.ReleaseSlot()
	}()

	switch req.ActionType {
	case types.RunRepoActionTypeMRCreate.String():
		r.executeMRCreateAction(ctx, req)
	default:
		err := fmt.Errorf("unsupported action_type %q", req.ActionType)
		_ = r.statusUploader.UploadActionStatus(ctx, req.ActionID, types.JobStatusError.String(), types.NewRunStatsBuilder().Error(err.Error()).MustBuild())
	}
}

func (r *runController) executeMRCreateAction(ctx context.Context, req StartActionRequest) {
	startTime := time.Now()

	jobReq := StartRunRequest{
		RunID:        req.RunID,
		JobID:        req.ActionID,
		RepoID:       req.RepoID,
		RepoURL:      req.RepoURL,
		BaseRef:      req.BaseRef,
		TargetRef:    req.TargetRef,
		TypedOptions: req.TypedOptions,
		Env:          req.Env,
	}

	manifest, err := buildManifestFromRequest(jobReq, req.TypedOptions, 0, resolveManifestStack(jobReq, contracts.MigStackUnknown))
	if err != nil {
		stats := types.NewRunStatsBuilder().DurationMs(time.Since(startTime).Milliseconds()).Error(err.Error()).MustBuild()
		_ = r.statusUploader.UploadActionStatus(ctx, req.ActionID, types.JobStatusError.String(), stats)
		return
	}

	workspace, err := r.rehydrateWorkspaceForStep(ctx, jobReq, manifest)
	if err != nil {
		stats := types.NewRunStatsBuilder().DurationMs(time.Since(startTime).Milliseconds()).Error(err.Error()).MustBuild()
		_ = r.statusUploader.UploadActionStatus(ctx, req.ActionID, lifecycle.JobStatusFromRunError(err).String(), stats)
		return
	}

	mrURL, mrErr := r.createMR(ctx, jobReq, manifest, workspace)
	builder := types.NewRunStatsBuilder().DurationMs(time.Since(startTime).Milliseconds())
	if mrURL != "" {
		builder.MetadataEntry("mr_url", mrURL)
	}
	if mrErr != nil {
		builder.Error(mrErr.Error())
		status := lifecycle.JobStatusFromRunError(mrErr).String()
		if uploadErr := r.statusUploader.UploadActionStatus(ctx, req.ActionID, status, builder.MustBuild()); uploadErr != nil {
			slog.Error("failed to upload action error status", "action_id", req.ActionID, "error", uploadErr)
		}
		return
	}
	if uploadErr := r.statusUploader.UploadActionStatus(ctx, req.ActionID, types.JobStatusSuccess.String(), builder.MustBuild()); uploadErr != nil {
		slog.Error("failed to upload action success status", "action_id", req.ActionID, "error", uploadErr)
	}
}
