package runtime

import (
	"context"

	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	workflowruntime "github.com/iw2rmb/ploy/internal/workflow/runtime"
)

// RegisterDefaultFactories installs built-in runtime adapters.
func RegisterDefaultFactories(loader *Loader) {
	if loader == nil {
		return
	}
	loader.RegisterFactory("local", localFactory{})
}

type localFactory struct{}

func (localFactory) Name() string { return "local" }

func (localFactory) Build(context.Context, config.RuntimePluginConfig) (workflowruntime.Adapter, error) {
	return localAdapter{}, nil
}

type localAdapter struct{}

func (localAdapter) Metadata() workflowruntime.AdapterMetadata {
	return workflowruntime.AdapterMetadata{Name: "local"}
}

func (localAdapter) Connect(context.Context) (runner.RuntimeClient, error) {
	return &noopGridClient{}, nil
}

type noopGridClient struct{}

func (noopGridClient) ExecuteStage(context.Context, contracts.WorkflowTicket, runner.Stage, string) (runner.StageOutcome, error) {
	return runner.StageOutcome{}, nil
}

func (noopGridClient) CancelWorkflow(context.Context, runner.CancelRequest) (runner.CancelResult, error) {
	return runner.CancelResult{}, runner.ErrCancellationUnsupported
}
