package step

import (
	"context"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Mock implementations for testing.

type mockWorkspaceHydrator struct {
	hydrateFn func(ctx context.Context, manifest contracts.StepManifest, workspace string) error
}

func (m *mockWorkspaceHydrator) Hydrate(ctx context.Context, manifest contracts.StepManifest, workspace string) error {
	if m.hydrateFn != nil {
		return m.hydrateFn(ctx, manifest, workspace)
	}
	return nil
}

type mockGateExecutor struct {
	executeFn func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error)
}

func (m *mockGateExecutor) Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, spec, workspace)
	}
	// Default: return a passing gate check
	return &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{
				Tool:   "default",
				Passed: true,
			},
		},
		LogFindings: []contracts.BuildGateLogFinding{},
	}, nil
}

// TestRunner_Run_BasicExecution verifies that a basic step execution completes
// successfully with proper exit code and timing capture when using minimal
// configuration with repo-based hydration.
func TestRunner_Run_BasicExecution(t *testing.T) {
	runner := Runner{
		Workspace: &mockWorkspaceHydrator{},
		Gate:      &mockGateExecutor{},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{
				Name:      "source",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadOnly,
				Hydration: &contracts.StepInputHydration{
					Repo: &contracts.RepoMaterialization{
						URL: "https://github.com/example/repo.git",
					},
				},
			},
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

	if result.ExitCode != 0 {
		t.Errorf("Run() ExitCode = %d, want 0", result.ExitCode)
	}

	// Verify timing fields are populated
	if result.Timings.TotalDuration == 0 {
		t.Errorf("Run() TotalDuration not captured")
	}
	if result.Timings.HydrationDuration < 0 {
		t.Errorf("Run() HydrationDuration invalid: %v", result.Timings.HydrationDuration)
	}
	if result.Timings.ExecutionDuration < 0 {
		t.Errorf("Run() ExecutionDuration invalid: %v", result.Timings.ExecutionDuration)
	}
	if result.Timings.BuildGateDuration < 0 {
		t.Errorf("Run() BuildGateDuration invalid: %v", result.Timings.BuildGateDuration)
	}
}

// TestRunner_Run_NilComponents verifies that the runner gracefully handles
// nil workspace and gate components without panicking, while still capturing
// basic timing information.
func TestRunner_Run_NilComponents(t *testing.T) {
	// Runner with nil workspace and gate should still work
	runner := Runner{
		Workspace: nil,
		Gate:      nil,
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

	result, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run() unexpected error with nil components: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Run() ExitCode = %d, want 0", result.ExitCode)
	}

	// Timing should still be captured even with nil components
	if result.Timings.TotalDuration == 0 {
		t.Errorf("Run() TotalDuration not captured with nil components")
	}
}
