package workflow

import (
	"context"
	"errors"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

var errMissingClient = errors.New("workflow: grid client required")

// CancelOptions captures the inputs for dispatching a workflow cancellation request.
type CancelOptions struct {
	Tenant     string
	RunID      string
	WorkflowID string
	Reason     string
}

// CancelCommand issues workflow cancellations through a Grid client.
type CancelCommand struct {
	Client runner.GridClient
}

// Run submits the cancellation request to the backing Grid client.
func (c CancelCommand) Run(ctx context.Context, opts CancelOptions) (runner.CancelResult, error) {
	if c.Client == nil {
		return runner.CancelResult{}, errMissingClient
	}
	request := runner.CancelRequest{
		Tenant:     strings.TrimSpace(opts.Tenant),
		RunID:      strings.TrimSpace(opts.RunID),
		WorkflowID: strings.TrimSpace(opts.WorkflowID),
		Reason:     strings.TrimSpace(opts.Reason),
	}
	return c.Client.CancelWorkflow(contextOrBackground(ctx), request)
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}
