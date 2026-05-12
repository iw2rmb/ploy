package step

import (
	"context"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestRunner_Run exercises basic execution paths: a runner with default
// hydrator+gate components, and a runner with nil components. Both must
// complete successfully with timing fields populated.
func TestRunner_Run(t *testing.T) {
	tests := []struct {
		name   string
		runner Runner
		input  contracts.StepInput
	}{
		{
			name: "default components with repo hydration input",
			runner: Runner{
				Workspace: &testWorkspaceHydrator{},
				Gate:      &testGateExecutor{},
			},
			input: contracts.StepInput{
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
		{
			name:   "nil components are tolerated",
			runner: Runner{Workspace: nil, Gate: nil},
			input: contracts.StepInput{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: types.CID("bafytest123"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := contracts.StepManifest{
				ID:     types.StepID("test-step"),
				Name:   "Test Step",
				Image:  "test:latest",
				Inputs: []contracts.StepInput{tt.input},
			}
			req := Request{Manifest: manifest, Workspace: "/tmp/test-workspace"}

			result, err := tt.runner.Run(context.Background(), req)
			if err != nil {
				t.Fatalf("Run() unexpected error: %v", err)
			}
			if result.ExitCode != 0 {
				t.Errorf("Run() ExitCode = %d, want 0", result.ExitCode)
			}
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
		})
	}
}
