package step

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestNewDockerGateExecutor_DefaultLocalDocker(t *testing.T) {
	t.Parallel()

	mockRT := &testContainerRuntime{}

	executor := NewDockerGateExecutor(mockRT)

	if executor == nil {
		t.Fatal("expected non-nil executor for empty mode")
	}

	// Verify it's a dockerGateExecutor by executing and checking behavior.
	// dockerGateExecutor returns nil,nil for nil spec.
	result, err := executor.Execute(context.Background(), nil, "/workspace")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for nil spec, got: %+v", result)
	}
}

func TestNewDockerGateExecutor_NilRuntime(t *testing.T) {
	t.Parallel()

	executor := NewDockerGateExecutor(nil)

	if executor == nil {
		t.Fatal("expected non-nil executor even with nil runtime")
	}

	// dockerGateExecutor with nil runtime fails gate execution.
	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(context.Background(), spec, "/workspace")

	// With nil runtime, dockerGateExecutor must return an explicit error.
	if err == nil {
		t.Fatal("expected error with nil runtime, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result with nil runtime error, got: %+v", result)
	}
}
