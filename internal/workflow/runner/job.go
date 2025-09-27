package runner

import (
	"context"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// JobComposer builds stage job specifications ahead of Grid submission.
type JobComposer interface {
	Compose(ctx context.Context, req JobComposeRequest) (StageJobSpec, error)
}

// JobComposeRequest captures the context required to build a job spec for a stage.
type JobComposeRequest struct {
	Stage  Stage
	Ticket contracts.WorkflowTicket
}
