package step

import (
	"context"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestRunner_Run_TimingCapture verifies that all phase timings (hydration,
// execution, build gate, diff, publish) are accurately measured and that
// total duration reflects the sum of all individual phase durations.
func TestRunner_Run_TimingCapture(t *testing.T) {
	hydrationDelay := 10 * time.Millisecond
	gateDelay := 5 * time.Millisecond

	runner := Runner{
		Workspace: &mockWorkspaceHydrator{
			hydrateFn: func(ctx context.Context, manifest contracts.StepManifest, workspace string) error {
				time.Sleep(hydrationDelay)
				return nil
			},
		},
		Gate: &mockGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				time.Sleep(gateDelay)
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "test", Passed: true},
					},
				}, nil
			},
		},
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
		Gate: &contracts.StepGateSpec{
			Enabled: true,
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
	}

	result, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	// Verify timing measurements are reasonable
	if time.Duration(result.Timings.HydrationDuration) < hydrationDelay {
		t.Errorf("Run() HydrationDuration = %v, expected >= %v", result.Timings.HydrationDuration, hydrationDelay)
	}

	if time.Duration(result.Timings.BuildGateDuration) < gateDelay {
		t.Errorf("Run() BuildGateDuration = %v, expected >= %v", result.Timings.BuildGateDuration, gateDelay)
	}

	// Total duration should be sum of all stages (with some tolerance)
	minExpected := result.Timings.HydrationDuration +
		result.Timings.ExecutionDuration +
		result.Timings.BuildGateDuration +
		result.Timings.DiffDuration +
		result.Timings.PublishDuration

	if result.Timings.TotalDuration < minExpected {
		t.Errorf("Run() TotalDuration = %v, expected >= %v", result.Timings.TotalDuration, minExpected)
	}
}
