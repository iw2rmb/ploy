package nodeagent

import (
	"context"
	"fmt"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func (r *runController) executeAction(ctx context.Context, req StartActionRequest) {
	defer func() {
		r.mu.Lock()
		delete(r.jobs, req.ActionID)
		r.mu.Unlock()
		r.ReleaseSlot()
	}()

	err := fmt.Errorf("unsupported action_type %q", req.ActionType)
	_ = r.statusUploader.UploadActionStatus(ctx, req.ActionID, types.JobStatusError.String(), types.NewRunStatsBuilder().Error(err.Error()).MustBuild())
}
