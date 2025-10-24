package executor

import (
	"context"
	"errors"
	"strings"

	"github.com/iw2rmb/ploy/internal/api/controlplane"
	workflowruntime "github.com/iw2rmb/ploy/internal/workflow/runtime"
)

// Options configure the assignment executor.
type Options struct {
	Registry       *workflowruntime.Registry
	DefaultAdapter string
}

// Executor resolves runtime adapters to process assignments.
type Executor struct {
	registry       *workflowruntime.Registry
	defaultAdapter string
}

// New constructs an Executor instance.
func New(opts Options) *Executor {
	adapter := strings.ToLower(strings.TrimSpace(opts.DefaultAdapter))
	return &Executor{registry: opts.Registry, defaultAdapter: adapter}
}

// Execute resolves the target runtime and establishes a connection.
func (e *Executor) Execute(ctx context.Context, assignment controlplane.Assignment) error {
	if e == nil || e.registry == nil {
		return errors.New("executor: registry not initialised")
	}
	name := strings.ToLower(strings.TrimSpace(assignment.Runtime))
	if name == "" {
		name = e.defaultAdapter
	}
	adapter, _, err := e.registry.Resolve(name)
	if err != nil {
		return err
	}
	_, err = adapter.Connect(ctx)
	return err
}
