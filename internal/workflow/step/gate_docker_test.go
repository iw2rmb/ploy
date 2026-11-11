package step

import (
	"context"
	"testing"
)

func TestNewGateExecutor_DefaultLocalDocker(t *testing.T) {
	t.Parallel()

	mockRT := &testContainerRuntime{}

	executor := NewGateExecutor(mockRT)

	if executor == nil {
		t.Fatal("expected non-nil executor for empty mode")
	}

	// A nil spec yields nil metadata and no error.
	result, err := executor.Execute(context.Background(), nil, "/workspace", newTestGateJobMounts(t))
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for nil spec, got: %+v", result)
	}
}
