package step

import (
	"context"
	"errors"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestRunner_Run_HydrationFailure verifies that hydration errors are properly
// propagated when workspace hydration fails during the hydration phase.
func TestRunner_Run_HydrationFailure(t *testing.T) {
	expectedErr := errors.New("hydration failed")
	runner := Runner{
		Workspace: &testWorkspaceHydrator{
			hydrateFn: func(ctx context.Context, manifest contracts.StepManifest, workspace string) error {
				return expectedErr
			},
		},
		Gate: &testGateExecutor{},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: types.CID("bafytest123"),
			},
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
	}

	_, err := runner.Run(context.Background(), req)
	if err == nil {
		t.Fatalf("Run() expected error, got nil")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("Run() error chain doesn't include hydration error: %v", err)
	}
}
