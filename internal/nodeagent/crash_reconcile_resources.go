package nodeagent

import (
	"context"
	"log/slog"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// Docker labels survive a node crash even when the parent container and local
// runtime directories do not. Unknown control-plane state preserves resources.
func (c *ClaimManager) reconcileDockerJobResources(ctx context.Context) {
	if c.startupReconciler == nil || c.startupReconciler.resources == nil {
		return
	}
	docker := c.startupReconciler.resources
	owners, err := step.ListDockerJobOwners(ctx, docker)
	if err != nil {
		slog.Warn("job Docker resource discovery incomplete", "error", err)
	}
	if len(owners) == 0 {
		return
	}
	uploader, err := c.ensureUploader()
	if err != nil {
		slog.Warn("cannot verify Docker resource owners", "error", err)
		return
	}
	for _, owner := range owners {
		statusText, err := uploader.GetJobStatus(ctx, owner.JobID)
		if err != nil {
			slog.Warn("preserving Docker resources with unknown job status", "job_id", owner.JobID, "error", err)
			continue
		}
		status, err := types.ParseJobStatus(statusText)
		if err != nil || !isTerminalRuntimeStatus(status) {
			continue
		}
		if err := step.RemoveDockerJobResources(ctx, docker, owner); err != nil {
			slog.Warn("failed to reconcile job Docker resources", "job_id", owner.JobID, "resume_count", owner.ResumeCount, "error", err)
		}
	}
}
