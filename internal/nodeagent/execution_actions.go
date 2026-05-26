package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func (r *runController) executeAction(ctx context.Context, req StartActionRequest) {
	started := time.Now()
	defer func() {
		r.mu.Lock()
		delete(r.jobs, req.ActionID)
		r.mu.Unlock()
		r.ReleaseSlot()
	}()

	output, err := executeNodeMaintenanceAction(ctx, strings.TrimSpace(req.ActionType))
	status := types.JobStatusSuccess
	builder := types.NewRunStatsBuilder().DurationMs(time.Since(started).Milliseconds())
	if strings.TrimSpace(output) != "" {
		builder.MetadataEntry("output", clipActionOutput(output))
	}
	if err != nil {
		status = types.JobStatusError
		builder.Error(err.Error())
	}
	if uploadErr := r.statusUploader.UploadActionStatus(ctx, req.ActionID, status.String(), builder.MustBuild()); uploadErr != nil {
		slog.Error("failed to upload action status", "action_id", req.ActionID, "action_type", req.ActionType, "status", status, "error", uploadErr)
	}
}

func executeNodeMaintenanceAction(ctx context.Context, actionType string) (string, error) {
	_ = ctx
	return "", fmt.Errorf("unsupported action_type %q", actionType)
}

func clipActionOutput(output string) string {
	const limit = 4000
	output = strings.TrimSpace(output)
	if len(output) <= limit {
		return output
	}
	return output[:limit] + "...<truncated>"
}
