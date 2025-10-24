package executor_test

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/api/controlplane"
	"github.com/iw2rmb/ploy/internal/api/executor"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	workflowruntime "github.com/iw2rmb/ploy/internal/workflow/runtime"
)

func TestExecutorResolvesRuntime(t *testing.T) {
	registry := workflowruntime.NewRegistry()
	if err := registry.Register(stubAdapter{name: "local"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	exec := executor.New(executor.Options{Registry: registry, DefaultAdapter: "local"})
	assignment := controlplane.Assignment{ID: "a1", Runtime: "local"}
	if err := exec.Execute(context.Background(), assignment); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecutorRequiresAdapter(t *testing.T) {
	registry := workflowruntime.NewRegistry()
	exec := executor.New(executor.Options{Registry: registry, DefaultAdapter: "local"})
	if err := exec.Execute(context.Background(), controlplane.Assignment{ID: "a1"}); err == nil {
		t.Fatal("expected error for missing adapter")
	}
}

type stubAdapter struct {
	name string
}

func (s stubAdapter) Metadata() workflowruntime.AdapterMetadata {
	return workflowruntime.AdapterMetadata{Name: s.name}
}

func (stubAdapter) Connect(context.Context) (runner.GridClient, error) {
	return nil, nil
}
