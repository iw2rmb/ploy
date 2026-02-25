package nodeagent

import (
	"context"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func withGateExecutionLabels(ctx context.Context, req StartRunRequest) context.Context {
	labels := make(map[string]string, 2)
	if !req.RunID.IsZero() {
		labels[types.LabelRunID] = req.RunID.String()
	}
	if !req.JobID.IsZero() {
		labels[types.LabelJobID] = req.JobID.String()
	}
	return step.WithGateContainerLabels(ctx, labels)
}
