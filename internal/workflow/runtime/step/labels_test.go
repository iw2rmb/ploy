package step

import (
	"context"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// mockRuntime captures the ContainerSpec passed to Create and simulates a run.
type mockRuntime struct {
	captured ContainerSpec
}

func (m *mockRuntime) Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
	m.captured = spec
	return ContainerHandle{ID: "mock"}, nil
}

func (m *mockRuntime) Start(ctx context.Context, handle ContainerHandle) error { return nil }
func (m *mockRuntime) Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error) {
	return ContainerResult{ExitCode: 0}, nil
}
func (m *mockRuntime) Logs(ctx context.Context, handle ContainerHandle) ([]byte, error) {
	return nil, nil
}
func (m *mockRuntime) Remove(ctx context.Context, handle ContainerHandle) error { return nil }

func TestRunner_Run_SetsRunIDLabel(t *testing.T) {
	rt := &mockRuntime{}
	runner := Runner{Containers: rt}

	runID := types.RunID("run-123")

	manifest := contracts.StepManifest{
		ID:    types.StepID(runID),
		Name:  "Test Run",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{Name: "workspace", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite, Hydration: &contracts.StepInputHydration{}},
		},
	}

	req := Request{Manifest: manifest, Workspace: "/tmp/workspace"}

	if _, err := runner.Run(context.Background(), req); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if rt.captured.Labels == nil {
		t.Fatalf("expected labels, got nil")
	}
	if got := rt.captured.Labels[types.LabelRunID]; got != runID.String() {
		t.Fatalf("label %q=%q, want %q", types.LabelRunID, got, runID.String())
	}
}
