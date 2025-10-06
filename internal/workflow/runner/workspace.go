package runner

import (
	"context"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// WorkspacePrepareRequest describes the context for preparing a stage workspace.
type WorkspacePrepareRequest struct {
	Ticket contracts.WorkflowTicket
	Path   string
}

// WorkspacePreparer prepares the workflow workspace before stage execution.
type WorkspacePreparer interface {
	Prepare(ctx context.Context, req WorkspacePrepareRequest) error
}
