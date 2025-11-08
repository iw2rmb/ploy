package nodeagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// TestExecuteWithHealing_GatePassesAfterHealingMod verifies that when the initial gate fails
// but healing is configured, the healing mod executes and the gate is re-run.
func TestExecuteWithHealing_GatePassesAfterHealingMod(t *testing.T) {
	// Track call sequence to ensure proper orchestration.
	var callSequence []string

	// Mock gate executor that fails on first call, passes on second call.
	gateCallCount := 0
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			callSequence = append(callSequence, "gate")

			if gateCallCount == 1 {
				// First gate fails
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: false},
					},
					LogsText: "[ERROR] Compilation failure\n",
				}, nil
			}
			// Second gate passes after healing
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: true},
				},
				LogsText: "[INFO] BUILD SUCCESS\n",
			}, nil
		},
	}

	// Mock container runtime that tracks execution.
	containerCallCount := 0
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			containerCallCount++
			callSequence = append(callSequence, "container:"+spec.Image)
			return step.ContainerHandle{ID: "mock-container"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn: func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
			return []byte("container logs"), nil
		},
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
	}

	// Create temporary directories for workspace, out, and in.
	workspace, err := os.MkdirTemp("", "ploy-test-ws-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace)

	outDir, err := os.MkdirTemp("", "ploy-test-out-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outDir)

	inDir := ""

	// Create runner with mocked components.
	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	// Create runController with minimal config.
	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    "test-node",
		},
	}

	// Prepare request with healing configuration.
	req := StartRunRequest{
		RunID:     types.RunID("test-run-healing"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("test-branch"),
		Env:       map[string]string{},
		Options: map[string]any{
			"build_gate_healing": map[string]any{
				"retries": 1,
				"mods": []any{
					map[string]any{
						"image": "test/healer:latest",
						"env": map[string]any{
							"HEAL_TASK": "fix-build",
						},
					},
				},
			},
		},
	}

	// Build main manifest with gate enabled.
	manifest := contracts.StepManifest{
		ID:    types.StepID(req.RunID),
		Name:  "Main mod",
		Image: "test/main-mod:latest",
		Inputs: []contracts.StepInput{
			{
				Name:        "workspace",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadWrite,
				SnapshotCID: types.CID("bafy123"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
			Profile: "java",
		},
		Options: req.Options,
	}

	// Execute with healing.
	result, err := rc.executeWithHealing(context.Background(), runner, req, manifest, workspace, outDir, &inDir)

	// Should succeed after healing.
	if err != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil", err)
	}

	// Exit code should be 0 (main mod succeeded).
	if result.ExitCode != 0 {
		t.Errorf("executeWithHealing() exit code = %d, want 0", result.ExitCode)
	}

	// Verify call sequence: gate (fail) → healing container → gate (pass) → main container
	// After gate passes, we run the main mod without re-checking the gate.
	expectedSequence := []string{"gate", "container:test/healer:latest", "gate", "container:test/main-mod:latest"}
	if len(callSequence) != len(expectedSequence) {
		t.Fatalf("call sequence length = %d, want %d. Got: %v", len(callSequence), len(expectedSequence), callSequence)
	}
	for i, expected := range expectedSequence {
		if callSequence[i] != expected {
			t.Errorf("call sequence[%d] = %q, want %q", i, callSequence[i], expected)
		}
	}

	// Verify /in directory was created with build-gate.log.
	if inDir == "" {
		t.Errorf("inDir should be created for healing")
	} else {
		logPath := filepath.Join(inDir, "build-gate.log")
		if _, err := os.Stat(logPath); os.IsNotExist(err) {
			t.Errorf("build-gate.log should exist at %s", logPath)
		}
	}
}

// TestExecuteWithHealing_RetriesExhausted verifies that when healing retries are exhausted
// and the gate still fails, the function returns an appropriate error.
func TestExecuteWithHealing_RetriesExhausted(t *testing.T) {
	// Mock gate executor that always fails.
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: false},
				},
				LogsText: "[ERROR] Persistent build failure\n",
			}, nil
		},
	}

	// Mock container runtime.
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			return step.ContainerHandle{ID: "mock-container"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn: func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
			return []byte("healer logs"), nil
		},
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
	}

	workspace, _ := os.MkdirTemp("", "ploy-test-ws-*")
	defer os.RemoveAll(workspace)

	outDir, _ := os.MkdirTemp("", "ploy-test-out-*")
	defer os.RemoveAll(outDir)

	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    "test-node",
		},
	}

	req := StartRunRequest{
		RunID:     types.RunID("test-run-exhausted"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("test-branch"),
		Options: map[string]any{
			"build_gate_healing": map[string]any{
				"retries": 2, // Try twice
				"mods": []any{
					map[string]any{
						"image": "test/healer:latest",
					},
				},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.RunID),
		Name:  "Main mod",
		Image: "test/main-mod:latest",
		Inputs: []contracts.StepInput{
			{
				Name:        "workspace",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadWrite,
				SnapshotCID: types.CID("bafy123"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
			Profile: "java",
		},
		Options: req.Options,
	}

	_, err := rc.executeWithHealing(context.Background(), runner, req, manifest, workspace, outDir, &inDir)

	// Should return build gate failure error.
	if err == nil {
		t.Fatalf("executeWithHealing() expected error, got nil")
	}

	if !errors.Is(err, step.ErrBuildGateFailed) {
		t.Errorf("executeWithHealing() error should wrap ErrBuildGateFailed, got: %v", err)
	}

	// Error should mention retries exhausted.
	if err.Error() != "build gate failed: healing retries exhausted" {
		t.Errorf("executeWithHealing() error = %q, want 'build gate failed: healing retries exhausted'", err.Error())
	}
}

// Mock implementations for testing.

type mockWorkspaceHydrator struct{}

func (m *mockWorkspaceHydrator) Hydrate(ctx context.Context, manifest contracts.StepManifest, workspace string) error {
	return nil
}

type mockGateExecutor struct {
	executeFn func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error)
}

func (m *mockGateExecutor) Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, spec, workspace)
	}
	return &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "test", Passed: true}},
	}, nil
}

type mockContainerRuntime struct {
	createFn func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error)
	startFn  func(ctx context.Context, handle step.ContainerHandle) error
	waitFn   func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error)
	logsFn   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error)
	removeFn func(ctx context.Context, handle step.ContainerHandle) error
}

func (m *mockContainerRuntime) Create(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
	if m.createFn != nil {
		return m.createFn(ctx, spec)
	}
	return step.ContainerHandle{ID: "mock"}, nil
}

func (m *mockContainerRuntime) Start(ctx context.Context, handle step.ContainerHandle) error {
	if m.startFn != nil {
		return m.startFn(ctx, handle)
	}
	return nil
}

func (m *mockContainerRuntime) Wait(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
	if m.waitFn != nil {
		return m.waitFn(ctx, handle)
	}
	return step.ContainerResult{ExitCode: 0}, nil
}

func (m *mockContainerRuntime) Logs(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
	if m.logsFn != nil {
		return m.logsFn(ctx, handle)
	}
	return []byte("logs"), nil
}

func (m *mockContainerRuntime) Remove(ctx context.Context, handle step.ContainerHandle) error {
	if m.removeFn != nil {
		return m.removeFn(ctx, handle)
	}
	return nil
}
