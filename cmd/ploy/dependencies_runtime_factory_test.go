package main

import (
	"context"
	"errors"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/runtime"
)

type fakeRuntimeAdapter struct {
	meta          runtime.AdapterMetadata
	grid          runner.RuntimeClient
	connectErr    error
	connectCalled int
}

func (a *fakeRuntimeAdapter) Metadata() runtime.AdapterMetadata {
	return a.meta
}

func (a *fakeRuntimeAdapter) Connect(_ context.Context) (runner.RuntimeClient, error) {
	a.connectCalled++
	if a.connectErr != nil {
		return nil, a.connectErr
	}
	return a.grid, nil
}

func TestDefaultRuntimeFactoryUsesSelectedRuntimeAdapter(t *testing.T) {
	prevRegistry := runtimeRegistry
	t.Cleanup(func() { runtimeRegistry = prevRegistry })
	runtimeRegistry = runtime.NewRegistry()

	adapter := &fakeRuntimeAdapter{
		meta: runtime.AdapterMetadata{
			Name: "fake",
		},
		grid: runner.NewInMemoryGrid(),
	}
	if err := runtimeRegistry.Register(adapter); err != nil {
		t.Fatalf("register adapter: %v", err)
	}

	t.Setenv(runtimeAdapterEnv, "fake")

	client, err := defaultRuntimeFactory()
	if err != nil {
		t.Fatalf("defaultRuntimeFactory: %v", err)
	}
	if adapter.connectCalled == 0 {
		t.Fatalf("expected adapter Connect to be called")
	}
	if client != adapter.grid {
		t.Fatalf("expected adapter runtime client to be returned")
	}
}

func TestDefaultRuntimeFactoryDefaultsToLocalStep(t *testing.T) {
	prevRegistry := runtimeRegistry
	t.Cleanup(func() { runtimeRegistry = prevRegistry })
	runtimeRegistry = runtime.NewRegistry()

	adapter := &fakeRuntimeAdapter{
		meta: runtime.AdapterMetadata{
			Name:    "local-step",
			Aliases: []string{"local"},
		},
		grid: runner.NewInMemoryGrid(),
	}
	if err := runtimeRegistry.Register(adapter); err != nil {
		t.Fatalf("register adapter: %v", err)
	}

	t.Setenv(runtimeAdapterEnv, "")
	client, err := defaultRuntimeFactory()
	if err != nil {
		t.Fatalf("defaultRuntimeFactory: %v", err)
	}
	if adapter.connectCalled != 1 {
		t.Fatalf("expected local-step adapter used, calls=%d", adapter.connectCalled)
	}
	if client != adapter.grid {
		t.Fatalf("expected local-step runtime client to be returned")
	}
}

func TestDefaultRuntimeFactoryErrorsWhenRuntimeAdapterMissing(t *testing.T) {
	prevRegistry := runtimeRegistry
	t.Cleanup(func() { runtimeRegistry = prevRegistry })
	runtimeRegistry = runtime.NewRegistry()
	// No adapters registered; resolution should fail.

	t.Setenv(runtimeAdapterEnv, "unknown")

	_, err := defaultRuntimeFactory()
	if err == nil {
		t.Fatalf("expected error when adapter missing")
	}
	if !errors.Is(err, runtime.ErrAdapterNotFound) {
		t.Fatalf("expected runtime.ErrAdapterNotFound, got %v", err)
	}
}
