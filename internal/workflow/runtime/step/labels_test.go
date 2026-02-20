package step

import (
	"context"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestRunner_Run_SetsRunIDLabel verifies that the container labels include
// LabelRunID when a RunID is provided in the Request. This enables telemetry
// and log aggregation systems to correlate containers with workflow runs.
func TestRunner_Run_SetsRunIDLabel(t *testing.T) {
	rt := &testContainerRuntime{}
	runner := Runner{Containers: rt}

	runID := types.RunID("run-123")

	manifest := contracts.StepManifest{
		ID:    types.StepID("step-xyz"),
		Name:  "Test Run",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{Name: "workspace", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite, Hydration: &contracts.StepInputHydration{}},
		},
	}

	// Pass RunID directly to step.Request for consistent labeling.
	req := Request{RunID: runID, Manifest: manifest, Workspace: "/tmp/workspace"}

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

// TestRunner_Run_OmitsRunIDLabelWhenEmpty verifies that container labels
// do not include LabelRunID when no RunID is provided in the Request.
// This avoids empty or misleading labels in telemetry systems.
func TestRunner_Run_OmitsRunIDLabelWhenEmpty(t *testing.T) {
	rt := &testContainerRuntime{}
	runner := Runner{Containers: rt}

	manifest := contracts.StepManifest{
		ID:    types.StepID("step-xyz"),
		Name:  "Test Run",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{Name: "workspace", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite, Hydration: &contracts.StepInputHydration{}},
		},
	}

	// No RunID provided — labels should be empty.
	req := Request{Manifest: manifest, Workspace: "/tmp/workspace"}

	if _, err := runner.Run(context.Background(), req); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if rt.captured.Labels != nil {
		if _, ok := rt.captured.Labels[types.LabelRunID]; ok {
			t.Fatalf("expected no %q label when RunID empty", types.LabelRunID)
		}
		if len(rt.captured.Labels) != 0 {
			t.Fatalf("expected labels to be empty when RunID empty, got %v", rt.captured.Labels)
		}
	}
}
