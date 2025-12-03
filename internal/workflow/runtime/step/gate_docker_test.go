package step

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	lastSpec     ContainerSpec
}

func (m *mockGateRuntimeMinimal) Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
	m.createCalled = true
	m.lastSpec = spec
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

func TestDockerGateExecutor_GradleCommandOmitsFailFast(t *testing.T) {
	rt := &mockGateRuntimeMinimal{}
	executor := NewDockerGateExecutor(rt)

	// Use an explicit java-gradle profile; workspace contents are irrelevant for this path.
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte{}, 0o644); err != nil {
		t.Fatalf("failed to create dummy build.gradle: %v", err)
	}

	spec := &contracts.StepGateSpec{
		Enabled: true,
		Profile: "java-gradle",
	}

	if _, err := executor.Execute(context.Background(), spec, tmpDir); err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}
	if len(rt.lastSpec.Command) != 3 {
		t.Fatalf("expected 3-element command, got %v", rt.lastSpec.Command)
	}

	cmd := rt.lastSpec.Command[2]
	if !strings.Contains(cmd, "gradle -q --stacktrace") {
		t.Fatalf("expected gradle command with -q --stacktrace, got %q", cmd)
	}
	if strings.Contains(cmd, "--fail-fast") {
		t.Fatalf("expected gradle command not to contain --fail-fast, got %q", cmd)
	}
	if !strings.Contains(cmd, "test -p /workspace") {
		t.Fatalf("expected gradle command to run tests in /workspace, got %q", cmd)
	}
}
