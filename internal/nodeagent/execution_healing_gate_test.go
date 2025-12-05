package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// TestRunGateWithHealing_NoWorkspaceChanges_SkipsReGateAndFails verifies that
// when healing mods do not produce any workspace changes (as measured by
// `git status --porcelain`), the node agent does not re-run the gate and
// returns a terminal ErrBuildGateFailed error.
func TestRunGateWithHealing_NoWorkspaceChanges_SkipsReGateAndFails(t *testing.T) {
	// Mock gate executor that always fails to force healing.
	gateCallCount := 0
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: false},
				},
				LogsText: "[ERROR] Build failure\n",
			}, nil
		},
	}

	// Healing container runs but does not modify the workspace.
	healingContainerCount := 0
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			healingContainerCount++
			return step.ContainerHandle{ID: "healer"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
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

	// Create workspace with a clean git repo so git status --porcelain is empty
	// both before and after healing.
	workspace, err := os.MkdirTemp("", "ploy-no-diff-ws-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace)

	// Reuse helper that initializes a git repo and then reset changes to ensure
	// a clean working tree.
	if err := setupGitRepoWithChange(workspace); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(workspace, "git", "checkout", "--", "."); err != nil {
		t.Fatal(err)
	}

	outDir, err := os.MkdirTemp("", "ploy-no-diff-out-*")
	if err != nil {
		t.Fatal(err)
	}
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
		RunID: types.RunID("test-no-diff-healing"),
		Options: map[string]any{
			"build_gate_healing": map[string]any{
				"retries": 1,
				"mods": []any{
					map[string]any{"image": "healer:latest"},
				},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:   types.StepID(req.RunID),
		Name: "Test",
		Gate: &contracts.StepGateSpec{
			Enabled: true,
			Profile: "java",
		},
	}

	_, _, gateErr := rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if !errors.Is(gateErr, step.ErrBuildGateFailed) {
		t.Fatalf("runGateWithHealing() error = %v, want ErrBuildGateFailed", gateErr)
	}
	if gateCallCount != 1 {
		t.Fatalf("expected 1 gate call (no re-gate), got %d", gateCallCount)
	}
	if healingContainerCount != 1 {
		t.Fatalf("expected 1 healing container run, got %d", healingContainerCount)
	}
}

// TestRunGateWithHealing_GatePassesImmediately verifies that runGateWithHealing returns
// immediately with the gate metadata when the initial gate passes without healing.
// This test validates the reusable helper for future pre-mod and post-mod gate phases.
func TestRunGateWithHealing_GatePassesImmediately(t *testing.T) {
	// Mock gate executor that passes immediately.
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: true},
				},
				LogsText: "[INFO] BUILD SUCCESS\n",
			}, nil
		},
	}

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

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: &mockContainerRuntime{},
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    "test-node",
		},
	}

	req := StartRunRequest{
		RunID: types.RunID("test-gate-pass"),
	}

	manifest := contracts.StepManifest{
		ID:   types.StepID(req.RunID),
		Name: "Test",
		Gate: &contracts.StepGateSpec{
			Enabled: true,
			Profile: "java",
		},
	}

	initialGate, reGates, gateErr := rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if gateErr != nil {
		t.Fatalf("runGateWithHealing() error = %v, want nil", gateErr)
	}
	if initialGate == nil || initialGate.Metadata == nil {
		t.Fatal("initialGate and metadata should be captured")
	}
	if len(initialGate.Metadata.StaticChecks) == 0 || !initialGate.Metadata.StaticChecks[0].Passed {
		t.Error("gate should have passed")
	}
	if len(reGates) != 0 {
		t.Errorf("len(reGates) = %d, want 0 (no healing needed)", len(reGates))
	}
	if inDir != "" {
		t.Errorf("inDir should remain empty when gate passes, got %q", inDir)
	}
}

// TestRunGateWithHealing_GateFailsNoHealing verifies that runGateWithHealing returns
// ErrBuildGateFailed when the gate fails and no healing is configured.
func TestRunGateWithHealing_GateFailsNoHealing(t *testing.T) {
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: false},
				},
				LogsText: "[ERROR] Build failure\n",
			}, nil
		},
	}

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

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: &mockContainerRuntime{},
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    "test-node",
		},
	}

	req := StartRunRequest{
		RunID:   types.RunID("test-gate-fail-no-heal"),
		Options: map[string]any{},
	}

	manifest := contracts.StepManifest{
		ID:   types.StepID(req.RunID),
		Name: "Test",
		Gate: &contracts.StepGateSpec{
			Enabled: true,
			Profile: "java",
		},
	}

	initialGate, reGates, gateErr := rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if !errors.Is(gateErr, step.ErrBuildGateFailed) {
		t.Fatalf("runGateWithHealing() error = %v, want ErrBuildGateFailed", gateErr)
	}
	if initialGate == nil {
		t.Fatal("initialGate should be captured")
	}
	if len(reGates) != 0 {
		t.Errorf("len(reGates) = %d, want 0", len(reGates))
	}
}

// TestRunGateWithHealing_GateFailsHealingSucceeds verifies the gate+healing orchestration
// when the initial gate fails but healing succeeds on the first attempt.
func TestRunGateWithHealing_GateFailsHealingSucceeds(t *testing.T) {
	var callSequence []string
	gateCallCount := 0

	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			callSequence = append(callSequence, fmt.Sprintf("gate-%d", gateCallCount))

			if gateCallCount == 1 {
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}},
					LogsText:     "[ERROR] Initial failure\n",
				}, nil
			}
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: true}},
				LogsText:     "[INFO] Success after healing\n",
			}, nil
		},
	}

	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			callSequence = append(callSequence, "container:"+spec.Image)
			return step.ContainerHandle{ID: "mock"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn: func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
			return []byte("logs"), nil
		},
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

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
		RunID: types.RunID("test-gate-heal-success"),
		Options: map[string]any{
			"build_gate_healing": map[string]any{
				"retries": 1,
				"mods": []any{
					map[string]any{"image": "healer:latest"},
				},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:   types.StepID(req.RunID),
		Name: "Test",
		Gate: &contracts.StepGateSpec{
			Enabled: true,
			Profile: "java",
		},
		Options: req.Options,
	}

	initialGate, reGates, gateErr := rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if gateErr != nil {
		t.Fatalf("runGateWithHealing() error = %v, want nil", gateErr)
	}

	expectedSequence := []string{"gate-1", "container:healer:latest", "gate-2"}
	if len(callSequence) != len(expectedSequence) {
		t.Fatalf("call sequence = %v, want %v", callSequence, expectedSequence)
	}
	for i, expected := range expectedSequence {
		if callSequence[i] != expected {
			t.Errorf("callSequence[%d] = %q, want %q", i, callSequence[i], expected)
		}
	}

	if initialGate == nil || initialGate.Metadata == nil {
		t.Fatal("initialGate should be captured")
	}
	if initialGate.Metadata.StaticChecks[0].Passed {
		t.Error("initial gate should have failed")
	}

	if len(reGates) != 1 || !reGates[0].Metadata.StaticChecks[0].Passed {
		t.Fatalf("expected one successful re-gate, got: %#v", reGates)
	}
	if inDir == "" {
		t.Error("inDir should be created for healing")
	}
}

// TestRunGateWithHealing_HealingRetriesExhausted verifies that runGateWithHealing returns
// ErrBuildGateFailed when healing retries are exhausted without the gate passing.
func TestRunGateWithHealing_HealingRetriesExhausted(t *testing.T) {
	gateCallCount := 0

	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}},
				LogsText:     fmt.Sprintf("[ERROR] Failure %d\n", gateCallCount),
			}, nil
		},
	}

	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			return step.ContainerHandle{ID: "mock"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return []byte("logs"), nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
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
		cfg: Config{ServerURL: "http://localhost:9999", NodeID: "test-node"},
	}

	req := StartRunRequest{
		RunID: types.RunID("test-heal-exhausted"),
		Options: map[string]any{
			"build_gate_healing": map[string]any{
				"retries": 2,
				"mods":    []any{map[string]any{"image": "healer:latest"}},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:      types.StepID(req.RunID),
		Name:    "Test",
		Gate:    &contracts.StepGateSpec{Enabled: true, Profile: "java"},
		Options: req.Options,
	}

	initialGate, reGates, gateErr := rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if !errors.Is(gateErr, step.ErrBuildGateFailed) {
		t.Fatalf("runGateWithHealing() error = %v, want ErrBuildGateFailed", gateErr)
	}
	if initialGate == nil {
		t.Fatal("initialGate should not be nil")
	}
	if len(reGates) != 2 {
		t.Fatalf("len(reGates) = %d, want 2", len(reGates))
	}
	if gateCallCount != 3 {
		t.Errorf("gate call count = %d, want 3", gateCallCount)
	}
}

// TestPreModGate_HealingFixesAndRunProceeds focuses on the pre-mod gate phase.
func TestPreModGate_HealingFixesAndRunProceeds(t *testing.T) {
	var callSequence []string
	gateCallCount := 0

	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			callSequence = append(callSequence, fmt.Sprintf("gate-%d", gateCallCount))

			if gateCallCount == 1 {
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: false},
					},
					LogsText: "[ERROR] Baseline compilation failure\n",
				}, nil
			}
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: true},
				},
				LogsText: "[INFO] BUILD SUCCESS\n",
			}, nil
		},
	}

	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			callSequence = append(callSequence, "container:"+spec.Image)
			return step.ContainerHandle{ID: "mock"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return []byte("logs"), nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	workspace, err := os.MkdirTemp("", "ploy-premod-gate-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace)

	outDir, err := os.MkdirTemp("", "ploy-premod-out-*")
	if err != nil {
		t.Fatal(err)
	}
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
		RunID:     types.RunID("test-premod-gate-heal"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature"),
		Options: map[string]any{
			"build_gate_healing": map[string]any{
				"retries": 1,
				"mods": []any{
					map[string]any{"image": "healer:latest"},
				},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.RunID),
		Name:  "Main mod",
		Image: "main-mod:latest",
		Inputs: []contracts.StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadWrite,
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
			Profile: "java",
		},
		Options: req.Options,
	}

	preGate, reGates, gateErr := rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if gateErr != nil {
		t.Fatalf("pre-mod gate should pass after healing, got error: %v", gateErr)
	}
	if preGate == nil || preGate.Metadata == nil {
		t.Fatal("preGate should be captured")
	}
	if preGate.Metadata.StaticChecks[0].Passed {
		t.Error("preGate should have failed initially")
	}
	if len(reGates) != 1 || !reGates[0].Metadata.StaticChecks[0].Passed {
		t.Fatalf("expected one successful re-gate, got %#v", reGates)
	}

	expectedSequence := []string{"gate-1", "container:healer:latest", "gate-2"}
	if len(callSequence) != len(expectedSequence) {
		t.Fatalf("call sequence = %v, want %v", callSequence, expectedSequence)
	}
	for i, expected := range expectedSequence {
		if callSequence[i] != expected {
			t.Errorf("callSequence[%d] = %q, want %q", i, callSequence[i], expected)
		}
	}
	if inDir == "" {
		t.Error("inDir should be created for healing")
	}
}

// TestPreModGate_HealingExhaustedNoMods verifies that when pre-mod healing
// is exhausted without success, no main mod containers run.
func TestPreModGate_HealingExhaustedNoMods(t *testing.T) {
	gateCallCount := 0
	healingContainerCount := 0
	mainModExecuted := false

	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: false},
				},
				LogsText: fmt.Sprintf("[ERROR] Persistent failure %d\n", gateCallCount),
			}, nil
		},
	}

	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			if spec.Image == "main-mod:latest" {
				mainModExecuted = true
				t.Error("main mod should NOT execute when pre-mod gate fails")
			} else {
				healingContainerCount++
			}
			return step.ContainerHandle{ID: "mock"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return []byte("logs"), nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	workspace, _ := os.MkdirTemp("", "ploy-premod-exhausted-*")
	defer os.RemoveAll(workspace)
	outDir, _ := os.MkdirTemp("", "ploy-premod-exhausted-out-*")
	defer os.RemoveAll(outDir)
	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{ServerURL: "http://localhost:9999", NodeID: "test-node"},
	}

	req := StartRunRequest{
		RunID:     types.RunID("test-premod-exhausted"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature"),
		Options: map[string]any{
			"build_gate_healing": map[string]any{
				"retries": 2,
				"mods": []any{
					map[string]any{"image": "healer:latest"},
				},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.RunID),
		Name:  "Main mod",
		Image: "main-mod:latest",
		Inputs: []contracts.StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadWrite,
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
			Profile: "java",
		},
		Options: req.Options,
	}

	preGate, reGates, gateErr := rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if !errors.Is(gateErr, step.ErrBuildGateFailed) {
		t.Fatalf("expected ErrBuildGateFailed, got: %v", gateErr)
	}
	if preGate == nil {
		t.Fatal("preGate should be captured")
	}
	if len(reGates) != 2 {
		t.Fatalf("len(reGates) = %d, want 2", len(reGates))
	}
	if gateCallCount != 3 {
		t.Errorf("gate call count = %d, want 3", gateCallCount)
	}
	if healingContainerCount != 2 {
		t.Errorf("healing container count = %d, want 2", healingContainerCount)
	}
	if mainModExecuted {
		t.Error("main mod should NOT execute when pre-mod gate fails")
	}
}

// TestPreModGate_GatePassesNoHealing verifies that when the pre-mod gate passes
// immediately, no healing is triggered.
func TestPreModGate_GatePassesNoHealing(t *testing.T) {
	gateCallCount := 0
	healingContainerCount := 0

	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: true},
				},
				LogsText: "[INFO] BUILD SUCCESS\n",
			}, nil
		},
	}

	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			if spec.Image == "healer:latest" {
				healingContainerCount++
				t.Error("healing container should NOT run when gate passes")
			}
			return step.ContainerHandle{ID: "mock"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return nil, nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	workspace, _ := os.MkdirTemp("", "ploy-premod-pass-*")
	defer os.RemoveAll(workspace)
	outDir, _ := os.MkdirTemp("", "ploy-premod-pass-out-*")
	defer os.RemoveAll(outDir)
	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{ServerURL: "http://localhost:9999", NodeID: "test-node"},
	}

	req := StartRunRequest{
		RunID: types.RunID("test-premod-pass"),
		Options: map[string]any{
			"build_gate_healing": map[string]any{
				"retries": 1,
				"mods":    []any{map[string]any{"image": "healer:latest"}},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:      types.StepID(req.RunID),
		Name:    "Test",
		Gate:    &contracts.StepGateSpec{Enabled: true, Profile: "java"},
		Options: req.Options,
	}

	preGate, reGates, gateErr := rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if gateErr != nil {
		t.Fatalf("pre-mod gate should pass, got error: %v", gateErr)
	}
	if preGate == nil || !preGate.Metadata.StaticChecks[0].Passed {
		t.Error("preGate should be captured with passing check")
	}
	if len(reGates) != 0 {
		t.Errorf("len(reGates) = %d, want 0 (no healing needed)", len(reGates))
	}
	if gateCallCount != 1 {
		t.Errorf("gate call count = %d, want 1", gateCallCount)
	}
	if healingContainerCount != 0 {
		t.Errorf("healing container count = %d, want 0", healingContainerCount)
	}
	if inDir != "" {
		t.Errorf("inDir should remain empty when gate passes, got %q", inDir)
	}
}

// TestRunGateWithHealing_GateDisabled verifies that runGateWithHealing returns
// immediately when the gate is disabled or no executor is available.
func TestRunGateWithHealing_GateDisabled(t *testing.T) {
	workspace := t.TempDir()
	outDir := t.TempDir()
	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: &mockContainerRuntime{},
		// Gate is nil to simulate disabled executor.
	}

	rc := &runController{
		cfg: Config{ServerURL: "http://localhost:9999", NodeID: "test-node"},
	}

	req := StartRunRequest{
		RunID: types.RunID("test-gate-disabled"),
	}

	manifest := contracts.StepManifest{
		ID:   types.StepID(req.RunID),
		Name: "Test",
		// Gate nil or disabled.
	}

	initialGate, reGates, gateErr := rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if gateErr != nil {
		t.Fatalf("expected nil error when gate disabled, got %v", gateErr)
	}
	if initialGate != nil {
		t.Fatalf("expected initialGate nil when gate disabled, got %#v", initialGate)
	}
	if len(reGates) != 0 {
		t.Fatalf("expected no re-gates when gate disabled, got %d", len(reGates))
	}
}

// TestRunGateWithHealing_HTTPModePassesDiffPatch verifies that when healing mods modify
// the workspace, the re-gate execution populates DiffPatch with the accumulated workspace
// changes so remote HTTP gates can validate them.
func TestRunGateWithHealing_HTTPModePassesDiffPatch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping test")
	}

	var gateSpecs []*contracts.StepGateSpec
	gateCallCount := 0

	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			specCopy := &contracts.StepGateSpec{
				Enabled:   spec.Enabled,
				Profile:   spec.Profile,
				RepoURL:   spec.RepoURL,
				Ref:       spec.Ref,
				DiffPatch: append([]byte(nil), spec.DiffPatch...),
			}
			gateSpecs = append(gateSpecs, specCopy)

			if gateCallCount == 1 {
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: false},
					},
					LogsText: "[ERROR] Build failure\n",
				}, nil
			}
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: true},
				},
				LogsText: "[INFO] BUILD SUCCESS\n",
			}, nil
		},
	}

	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			return step.ContainerHandle{ID: "healer"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn: func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
			return []byte("healer logs"), nil
		},
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	workspace := t.TempDir()

	cmd := exec.Command("git", "init", workspace)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	cmd = exec.Command("git", "-C", workspace, "config", "user.email", "test@test.com")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config email failed: %v", err)
	}
	cmd = exec.Command("git", "-C", workspace, "config", "user.name", "Test User")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config name failed: %v", err)
	}

	initialFile := filepath.Join(workspace, "Main.java")
	if err := os.WriteFile(initialFile, []byte("public class Main {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "-C", workspace, "add", ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	cmd = exec.Command("git", "-C", workspace, "commit", "-m", "Initial commit")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	healerModifiedContent := []byte("public class Main { void heal() {} }\n")
	healingSimulator := func() {
		if err := os.WriteFile(initialFile, healerModifiedContent, 0o644); err != nil {
			t.Fatalf("failed to simulate healing modification: %v", err)
		}
	}

	wrappedContainer := &mockContainerRuntime{
		createFn: mockContainer.createFn,
		startFn:  mockContainer.startFn,
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			healingSimulator()
			return mockContainer.waitFn(ctx, handle)
		},
		logsFn:   mockContainer.logsFn,
		removeFn: mockContainer.removeFn,
	}

	outDir := t.TempDir()
	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: wrappedContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{ServerURL: "http://localhost:9999", NodeID: "test-node"},
	}

	req := StartRunRequest{
		RunID: types.RunID("test-http-regate"),
		Options: map[string]any{
			"build_gate_healing": map[string]any{
				"retries": 1,
				"mods": []any{
					map[string]any{"image": "healer:latest"},
				},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:      types.StepID(req.RunID),
		Name:    "Test",
		Gate:    &contracts.StepGateSpec{Enabled: true, Profile: "java"},
		Options: req.Options,
	}

	_, _, gateErr := rc.runGateWithHealing(
		context.Background(), runner, req, manifest, workspace, outDir, &inDir, "pre", 0,
	)

	if gateErr != nil {
		t.Fatalf("runGateWithHealing() error = %v, want nil", gateErr)
	}
	if gateCallCount != 2 {
		t.Fatalf("expected 2 gate calls (pre + re-gate), got %d", gateCallCount)
	}
	if len(gateSpecs) != 2 {
		t.Fatalf("expected 2 captured gate specs, got %d", len(gateSpecs))
	}
	if len(gateSpecs[1].DiffPatch) == 0 {
		t.Fatalf("expected non-empty DiffPatch on re-gate spec")
	}
}
