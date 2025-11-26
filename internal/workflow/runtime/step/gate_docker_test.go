package step

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// mockGateRuntimeMinimal implements the subset of ContainerRuntime used by
// dockerGateExecutor plus Remove, so we can verify cleanup behavior without
// depending on the real Docker client or runner mocks.
type mockGateRuntimeMinimal struct {
	createCalled bool
	startCalled  bool
	waitCalled   bool
	logsCalled   bool
	removeCalled bool
}

func (m *mockGateRuntimeMinimal) Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
	m.createCalled = true
	return ContainerHandle{ID: "mock-id"}, nil
}

func (m *mockGateRuntimeMinimal) Start(ctx context.Context, h ContainerHandle) error {
	m.startCalled = true
	return nil
}

func (m *mockGateRuntimeMinimal) Wait(ctx context.Context, h ContainerHandle) (ContainerResult, error) {
	m.waitCalled = true
	return ContainerResult{ExitCode: 0}, nil
}

func (m *mockGateRuntimeMinimal) Logs(ctx context.Context, h ContainerHandle) ([]byte, error) {
	m.logsCalled = true
	return []byte("ok"), nil
}

func (m *mockGateRuntimeMinimal) Remove(ctx context.Context, h ContainerHandle) error {
	m.removeCalled = true
	return nil
}

func TestDockerGateExecutor_RemovesContainerAfterExecution(t *testing.T) {
	rt := &mockGateRuntimeMinimal{}
	executor := NewDockerGateExecutor(rt)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		Profile: "java",
	}

	_, err := executor.Execute(context.Background(), spec, "/tmp/workspace")
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !rt.removeCalled {
		t.Fatalf("expected Remove to be called on container runtime after gate execution")
	}
	if !rt.createCalled || !rt.startCalled || !rt.waitCalled || !rt.logsCalled {
		t.Fatalf("expected create/start/wait/logs to be called before remove; got %+v", rt)
	}
}
