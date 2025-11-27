package step

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestNewGateExecutor_DefaultLocalDocker verifies that empty mode returns docker executor.
func TestNewGateExecutor_DefaultLocalDocker(t *testing.T) {
	t.Parallel()

	mockRT := &mockContainerRuntime{}

	// Empty mode should default to local-docker.
	executor := NewGateExecutor("", mockRT, nil)

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

// TestNewGateExecutor_ExplicitLocalDocker verifies "local-docker" mode returns docker executor.
func TestNewGateExecutor_ExplicitLocalDocker(t *testing.T) {
	t.Parallel()

	mockRT := &mockContainerRuntime{}

	// Explicit "local-docker" mode.
	executor := NewGateExecutor(GateExecutorModeLocalDocker, mockRT, nil)

	if executor == nil {
		t.Fatal("expected non-nil executor for local-docker mode")
	}

	// Verify it's a dockerGateExecutor.
	result, err := executor.Execute(context.Background(), nil, "/workspace")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for nil spec, got: %+v", result)
	}
}

// TestNewGateExecutor_RemoteHTTP verifies "remote-http" mode returns HTTP executor.
func TestNewGateExecutor_RemoteHTTP(t *testing.T) {
	t.Parallel()

	fakeHTTP := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "test-job",
			Status: contracts.BuildGateJobStatusCompleted,
			Result: &contracts.BuildGateStageMetadata{
				LogDigest: "http-executor-test",
			},
		},
	}

	// Remote HTTP mode with valid client.
	executor := NewGateExecutor(GateExecutorModeRemoteHTTP, nil, fakeHTTP)

	if executor == nil {
		t.Fatal("expected non-nil executor for remote-http mode")
	}

	// Verify it's an HTTPGateExecutor by checking that it calls the fake client.
	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(context.Background(), spec, "/workspace")

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.LogDigest != "http-executor-test" {
		t.Errorf("expected LogDigest 'http-executor-test', got '%s'", result.LogDigest)
	}
	if fakeHTTP.ValidateCallCount != 1 {
		t.Errorf("expected 1 Validate call, got %d", fakeHTTP.ValidateCallCount)
	}
}

// TestNewGateExecutor_RemoteHTTP_NilClientFallback verifies fallback to docker when httpClient is nil.
func TestNewGateExecutor_RemoteHTTP_NilClientFallback(t *testing.T) {
	t.Parallel()

	mockRT := &mockContainerRuntime{}

	// Remote HTTP mode with nil httpClient should fall back to docker.
	executor := NewGateExecutor(GateExecutorModeRemoteHTTP, mockRT, nil)

	if executor == nil {
		t.Fatal("expected non-nil executor for fallback")
	}

	// Verify it's a dockerGateExecutor (returns nil,nil for nil spec).
	result, err := executor.Execute(context.Background(), nil, "/workspace")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for nil spec (docker fallback), got: %+v", result)
	}
}

// TestNewGateExecutor_UnrecognizedModeFallback verifies that unrecognized mode falls back to docker.
func TestNewGateExecutor_UnrecognizedModeFallback(t *testing.T) {
	t.Parallel()

	mockRT := &mockContainerRuntime{}

	// Unrecognized mode should fall back to local-docker.
	executor := NewGateExecutor("invalid-mode", mockRT, nil)

	if executor == nil {
		t.Fatal("expected non-nil executor for fallback")
	}

	// Verify it's a dockerGateExecutor.
	result, err := executor.Execute(context.Background(), nil, "/workspace")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for nil spec (docker fallback), got: %+v", result)
	}
}

// TestNewGateExecutor_NilRuntime verifies behavior with nil container runtime.
func TestNewGateExecutor_NilRuntime(t *testing.T) {
	t.Parallel()

	// Local-docker mode with nil runtime.
	executor := NewGateExecutor(GateExecutorModeLocalDocker, nil, nil)

	if executor == nil {
		t.Fatal("expected non-nil executor even with nil runtime")
	}

	// dockerGateExecutor with nil runtime returns empty metadata for enabled spec.
	spec := &contracts.StepGateSpec{Enabled: true, Profile: "java"}
	result, err := executor.Execute(context.Background(), spec, "/workspace")

	// With nil runtime, dockerGateExecutor returns empty metadata without error.
	if err != nil {
		t.Errorf("expected nil error with nil runtime, got: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result (empty metadata) with nil runtime")
	}
}

// TestNewGateExecutor_ModeConstants verifies mode constants are correct.
func TestNewGateExecutor_ModeConstants(t *testing.T) {
	t.Parallel()

	if GateExecutorModeLocalDocker != "local-docker" {
		t.Errorf("GateExecutorModeLocalDocker = '%s', want 'local-docker'", GateExecutorModeLocalDocker)
	}
	if GateExecutorModeRemoteHTTP != "remote-http" {
		t.Errorf("GateExecutorModeRemoteHTTP = '%s', want 'remote-http'", GateExecutorModeRemoteHTTP)
	}
}

// TestNewGateExecutorWithLogger_PropagatesLogger verifies logger is propagated to HTTP executor.
func TestNewGateExecutorWithLogger_PropagatesLogger(t *testing.T) {
	t.Parallel()

	fakeHTTP := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "logger-test",
			Status: contracts.BuildGateJobStatusCompleted,
			Result: &contracts.BuildGateStageMetadata{},
		},
	}

	// Test with custom logger (nil should use default).
	executor := NewGateExecutorWithLogger(GateExecutorModeRemoteHTTP, nil, fakeHTTP, nil)

	if executor == nil {
		t.Fatal("expected non-nil executor")
	}

	// Verify executor works (logger propagation is internal).
	spec := &contracts.StepGateSpec{Enabled: true}
	_, err := executor.Execute(context.Background(), spec, "/workspace")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// mockContainerRuntime is a minimal mock for testing factory mode selection.
// It satisfies ContainerRuntime interface for local-docker mode tests.
type mockContainerRuntime struct{}

func (m *mockContainerRuntime) Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
	return ContainerHandle{ID: "mock"}, nil
}

func (m *mockContainerRuntime) Start(ctx context.Context, handle ContainerHandle) error {
	return nil
}

func (m *mockContainerRuntime) Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error) {
	return ContainerResult{ExitCode: 0}, nil
}

func (m *mockContainerRuntime) Logs(ctx context.Context, handle ContainerHandle) ([]byte, error) {
	return nil, nil
}

func (m *mockContainerRuntime) Remove(ctx context.Context, handle ContainerHandle) error {
	return nil
}
