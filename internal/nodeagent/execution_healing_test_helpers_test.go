package nodeagent

import (
	"context"
	"os"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// healingDirs creates temporary workspace and output directories for healing tests.
// Both directories are removed when the test completes.
func healingDirs(t *testing.T) (workspace, outDir string) {
	t.Helper()
	workspace = t.TempDir()
	outDir = t.TempDir()
	return workspace, outDir
}

// failingGate returns a mock gate executor that always reports failure.
func failingGate() *mockGateExecutor {
	return &mockGateExecutor{executeFn: func(_ context.Context, _ *contracts.StepGateSpec, _ string) (*contracts.BuildGateStageMetadata, error) {
		return &contracts.BuildGateStageMetadata{
			StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}},
			LogsText:     "[ERROR] Build failure\n",
		}, nil
	}}
}

// passingGate returns a mock gate executor that always reports success.
func passingGate() *mockGateExecutor {
	return &mockGateExecutor{executeFn: func(_ context.Context, _ *contracts.StepGateSpec, _ string) (*contracts.BuildGateStageMetadata, error) {
		return &contracts.BuildGateStageMetadata{
			StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: true}},
			LogsText:     "[INFO] BUILD SUCCESS\n",
		}, nil
	}}
}

// noopContainer returns a mock container runtime where all operations succeed with no-op behavior.
func noopContainer() *mockContainerRuntime {
	return &mockContainerRuntime{
		createFn: func(_ context.Context, _ step.ContainerSpec) (step.ContainerHandle, error) {
			return step.ContainerHandle{ID: "mock"}, nil
		},
		startFn: func(_ context.Context, _ step.ContainerHandle) error { return nil },
		waitFn: func(_ context.Context, _ step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(_ context.Context, _ step.ContainerHandle) ([]byte, error) { return []byte(""), nil },
		removeFn: func(_ context.Context, _ step.ContainerHandle) error { return nil },
	}
}

// envCapturingContainer returns a mock container runtime that captures the environment
// from the first container creation. The returned map pointer is populated lazily.
func envCapturingContainer() (*mockContainerRuntime, *map[string]string) {
	var captured map[string]string
	mc := &mockContainerRuntime{
		createFn: func(_ context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			if captured == nil {
				captured = make(map[string]string, len(spec.Env))
				for k, v := range spec.Env {
					captured[k] = v
				}
			}
			return step.ContainerHandle{ID: "heal"}, nil
		},
		startFn: func(_ context.Context, _ step.ContainerHandle) error { return nil },
		waitFn: func(_ context.Context, _ step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(_ context.Context, _ step.ContainerHandle) ([]byte, error) { return []byte(""), nil },
		removeFn: func(_ context.Context, _ step.ContainerHandle) error { return nil },
	}
	return mc, &captured
}

// healingRunner creates a step.Runner with a mock workspace hydrator and the given gate/container mocks.
func healingRunner(gate *mockGateExecutor, container *mockContainerRuntime) step.Runner {
	return step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: container,
		Gate:       gate,
	}
}

// healingRC creates a runController with a default config suitable for healing tests.
func healingRC() *runController {
	return &runController{cfg: Config{ServerURL: "http://localhost:9999", NodeID: testNodeID}}
}

// healingRCWithConfig creates a runController with the given config.
func healingRCWithConfig(cfg Config) *runController {
	return &runController{cfg: cfg}
}

// healingRequest creates a StartRunRequest with healing configuration.
func healingRequest(runID, jobID string, retries int, modImage string) StartRunRequest {
	req := StartRunRequest{
		RunID:     types.RunID(runID),
		JobID:     types.JobID(jobID),
		RepoURL:   types.RepoURL("https://gitlab.com/acme/x.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("br"),
	}
	if modImage != "" {
		req.TypedOptions = RunOptions{
			Healing: &HealingConfig{
				Retries: retries,
				Mod:     HealingMod{Image: contracts.ModImage{Universal: modImage}},
			},
		}
	} else {
		req.TypedOptions = RunOptions{
			Healing: &HealingConfig{
				Retries: retries,
			},
		}
	}
	return req
}

// healingManifest creates a StepManifest with gate enabled and a standard workspace input.
func healingManifest(req StartRunRequest) contracts.StepManifest {
	return contracts.StepManifest{
		ID:     types.StepID(req.JobID),
		Image:  "main:latest",
		Inputs: []contracts.StepInput{{Name: "ws", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite}},
		Gate:   &contracts.StepGateSpec{Enabled: true},
	}
}

// assertFileExists checks that a file exists at the given path.
func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("expected file to exist: %s", path)
	}
}
