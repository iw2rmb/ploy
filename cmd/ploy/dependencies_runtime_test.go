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
	grid          runner.GridClient
	connectErr    error
	connectCalled int
}

func (a *fakeRuntimeAdapter) Metadata() runtime.AdapterMetadata {
	return a.meta
}

func (a *fakeRuntimeAdapter) Connect(_ context.Context) (runner.GridClient, error) {
	a.connectCalled++
	if a.connectErr != nil {
		return nil, a.connectErr
	}
	return a.grid, nil
}

func TestDefaultGridFactoryUsesSelectedRuntimeAdapter(t *testing.T) {
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
	t.Setenv(gridIDEnv, "")
	t.Setenv(gridIDFallbackEnv, "")
	t.Setenv(gridAPIKeyEnv, "")
	t.Setenv(gridAPIKeyFallbackEnv, "")

	client, err := defaultGridFactory()
	if err != nil {
		t.Fatalf("defaultGridFactory: %v", err)
	}
	if adapter.connectCalled == 0 {
		t.Fatalf("expected adapter Connect to be called")
	}
	if client != adapter.grid {
		t.Fatalf("expected adapter grid client to be returned")
	}
}

func TestDefaultGridFactoryErrorsWhenRuntimeAdapterMissing(t *testing.T) {
	prevRegistry := runtimeRegistry
	t.Cleanup(func() { runtimeRegistry = prevRegistry })
	runtimeRegistry = runtime.NewRegistry()
	// No adapters registered; resolution should fail.

	t.Setenv(runtimeAdapterEnv, "unknown")
	t.Setenv(gridIDEnv, "")
	t.Setenv(gridIDFallbackEnv, "")
	t.Setenv(gridAPIKeyEnv, "")
	t.Setenv(gridAPIKeyFallbackEnv, "")

	_, err := defaultGridFactory()
	if err == nil {
		t.Fatalf("expected error when adapter missing")
	}
	if !errors.Is(err, runtime.ErrAdapterNotFound) {
		t.Fatalf("expected runtime.ErrAdapterNotFound, got %v", err)
	}
}
