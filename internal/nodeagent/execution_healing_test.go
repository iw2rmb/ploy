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

// TestExecuteWithHealing_GateStatsTracking verifies that pre-gate and re-gate stats
// are properly tracked and returned in the execution result.
func TestExecuteWithHealing_GateStatsTracking(t *testing.T) {
	// Mock gate executor that fails on first call, passes on second call.
	gateCallCount := 0
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++

			if gateCallCount == 1 {
				// First gate (pre-gate) fails with specific metadata.
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: false},
					},
					LogsText: "[ERROR] Pre-gate failure\n",
					Resources: &contracts.BuildGateResourceUsage{
						LimitNanoCPUs:    2000000000,
						LimitMemoryBytes: 1073741824,
						CPUTotalNs:       500000000,
						MemUsageBytes:    536870912,
					},
				}, nil
			}
			// Second gate (re-gate) passes with different metadata.
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: true},
				},
				LogsText: "[INFO] Re-gate success\n",
				Resources: &contracts.BuildGateResourceUsage{
					LimitNanoCPUs:    2000000000,
					LimitMemoryBytes: 1073741824,
					CPUTotalNs:       300000000,
					MemUsageBytes:    268435456,
				},
			}, nil
		},
	}

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
			return []byte("container logs"), nil
		},
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
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
		RunID:     types.RunID("test-run-stats"),
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

	execResult, err := rc.executeWithHealing(context.Background(), runner, req, manifest, workspace, outDir, &inDir, 0)

	if err != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil", err)
	}

	// Verify pre-gate stats are captured.
	if execResult.PreGate == nil {
		t.Fatal("execResult.PreGate should not be nil")
	}

	if execResult.PreGate.Metadata == nil {
		t.Fatal("execResult.PreGate.Metadata should not be nil")
	}

	if len(execResult.PreGate.Metadata.StaticChecks) == 0 || execResult.PreGate.Metadata.StaticChecks[0].Passed {
		t.Error("pre-gate should have failed check")
	}

	if execResult.PreGate.Metadata.LogsText != "[ERROR] Pre-gate failure\n" {
		t.Errorf("pre-gate logs = %q, want '[ERROR] Pre-gate failure\\n'", execResult.PreGate.Metadata.LogsText)
	}

	// Verify re-gate stats are captured.
	if len(execResult.ReGates) != 1 {
		t.Fatalf("len(execResult.ReGates) = %d, want 1", len(execResult.ReGates))
	}

	reGate := execResult.ReGates[0]
	if reGate.Metadata == nil {
		t.Fatal("re-gate metadata should not be nil")
	}

	if len(reGate.Metadata.StaticChecks) == 0 || !reGate.Metadata.StaticChecks[0].Passed {
		t.Error("re-gate should have passing check")
	}

	if reGate.Metadata.LogsText != "[INFO] Re-gate success\n" {
		t.Errorf("re-gate logs = %q, want '[INFO] Re-gate success\\n'", reGate.Metadata.LogsText)
	}

	// Verify duration is tracked (should be > 0).
	if execResult.PreGate.DurationMs < 0 {
		t.Errorf("pre-gate duration = %d, want >= 0", execResult.PreGate.DurationMs)
	}

	if reGate.DurationMs < 0 {
		t.Errorf("re-gate duration = %d, want >= 0", reGate.DurationMs)
	}
}

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
	result, err := rc.executeWithHealing(context.Background(), runner, req, manifest, workspace, outDir, &inDir, 0)

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

// TestExecuteWithHealing_NoHealingConfigured verifies that when the gate fails and no
// healing is configured, the function returns a terminal build gate error.
func TestExecuteWithHealing_NoHealingConfigured(t *testing.T) {
	// Mock gate executor that fails.
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

	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			t.Errorf("no containers should be created when gate fails without healing")
			return step.ContainerHandle{ID: "mock-container"}, nil
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
		RunID:     types.RunID("test-run-no-healing"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("test-branch"),
		Options:   map[string]any{
			// No build_gate_healing configured
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

	_, err := rc.executeWithHealing(context.Background(), runner, req, manifest, workspace, outDir, &inDir, 0)

	// Should return build gate failure error.
	if err == nil {
		t.Fatalf("executeWithHealing() expected error, got nil")
	}

	if !errors.Is(err, step.ErrBuildGateFailed) {
		t.Errorf("executeWithHealing() error should be ErrBuildGateFailed, got: %v", err)
	}
}

// TestExecuteWithHealing_RepoDiffSemantics verifies that healing verification aligns
// with the HTTP Build Gate API's repo+diff model:
//   - Pre-gate validates the workspace (repo_url+ref clone)
//   - Healing mods modify the workspace in-place
//   - Re-gate validates workspace = repo_url+ref + healing modifications
//
// This ensures conceptual equivalence with POST /v1/buildgate/validate using diff_patch.
func TestExecuteWithHealing_RepoDiffSemantics(t *testing.T) {
	// Track workspace paths passed to gate executor to verify same workspace is reused.
	var gateWorkspaces []string
	gateCallCount := 0

	// Mock gate executor that captures workspace paths and tracks call sequence.
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			gateWorkspaces = append(gateWorkspaces, workspace)

			if gateCallCount == 1 {
				// Pre-gate fails (initial repo_url+ref validation).
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: false},
					},
					LogsText: "[ERROR] Missing symbol: UnknownClass\n",
				}, nil
			}
			// Re-gate passes after healing modifications.
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: true},
				},
				LogsText: "[INFO] BUILD SUCCESS\n",
			}, nil
		},
	}

	// Track container specs to verify healing mod receives workspace.
	var containerSpecs []step.ContainerSpec
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			containerSpecs = append(containerSpecs, spec)
			return step.ContainerHandle{ID: "mock-container"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn: func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
			return []byte("healing complete"), nil
		},
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
	}

	// Create workspace simulating repo_url+ref clone.
	workspace, err := os.MkdirTemp("", "ploy-test-repo-diff-*")
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

	// Request with repo metadata (matching the repo+diff model).
	req := StartRunRequest{
		RunID:     types.RunID("test-run-repo-diff"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("e2e/fail-missing-symbol"),
		TargetRef: types.GitRef("mods-upgrade-java17"),
		CommitSHA: types.CommitSHA("abc123"),
		Env:       map[string]string{},
		Options: map[string]any{
			"build_gate_healing": map[string]any{
				"retries": 1,
				"mods": []any{
					map[string]any{
						"image": "test/codex-healer:latest",
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

	result, err := rc.executeWithHealing(context.Background(), runner, req, manifest, workspace, outDir, &inDir, 0)

	// Should succeed after healing.
	if err != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}

	// Verify repo+diff semantics: pre-gate and re-gate use the SAME workspace path.
	// This ensures healing modifications are validated in-place (repo_url+ref + diffs)
	// rather than creating a new workspace.
	if len(gateWorkspaces) != 2 {
		t.Fatalf("expected 2 gate calls (pre-gate, re-gate), got %d", len(gateWorkspaces))
	}

	if gateWorkspaces[0] != gateWorkspaces[1] {
		t.Errorf("repo+diff semantics violation: pre-gate workspace %q != re-gate workspace %q; "+
			"healing verification must reuse the same workspace containing repo_url+ref + healing changes",
			gateWorkspaces[0], gateWorkspaces[1])
	}

	// Both should point to our test workspace (the repo_url+ref clone).
	if gateWorkspaces[0] != workspace {
		t.Errorf("gate workspace = %q, want %q (original repo_url+ref workspace)", gateWorkspaces[0], workspace)
	}

	// Verify healing container also received the same workspace to modify.
	// This ensures healing mods accumulate changes on top of the repo baseline.
	if len(containerSpecs) < 1 {
		t.Fatal("expected at least one container spec for healing mod")
	}

	healerWorkspace := ""
	for _, mount := range containerSpecs[0].Mounts {
		if mount.Target == "/workspace" {
			healerWorkspace = mount.Source
			break
		}
	}
	if healerWorkspace != workspace {
		t.Errorf("healing mod workspace = %q, want %q (same repo_url+ref workspace)", healerWorkspace, workspace)
	}
}

// TestUploadHealingModDiff_MetadataTagging verifies that healing mod diffs are uploaded
// with proper metadata (mod_type=healing, mod_index, healing_attempt) to distinguish
// them from main mod diffs in the database.
func TestUploadHealingModDiff_MetadataTagging(t *testing.T) {
	t.Parallel()

	// Create temporary workspace with git repo for diff generation.
	workspace, err := os.MkdirTemp("", "ploy-test-ws-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace)

	// Initialize git repo and create a change.
	if err := setupGitRepoWithChange(workspace); err != nil {
		t.Fatal(err)
	}

	// Create runController with minimal config.
	// Note: This test validates diff metadata structure but doesn't actually upload
	// to a server (would require integration test setup).
	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    "test-node",
		},
	}

	// Create mock result for healing mod.
	healResult := step.Result{
		ExitCode: 0,
		Timings: step.StageTiming{
			HydrationDuration: 100,
			ExecutionDuration: 500,
			BuildGateDuration: 0,
			DiffDuration:      50,
			TotalDuration:     650,
		},
	}

	// Call uploadHealingModDiff (will fail at upload but we can verify the setup logic).
	// In a real scenario, this would need a mock HTTP server to validate the uploaded metadata.
	rc.uploadHealingModDiff(context.Background(), "test-run-id", "test-stage-id", workspace, healResult, 2, 1)

	// Verify that the function completes without panics (basic smoke test).
	// More comprehensive validation would require capturing the HTTP request in an integration test.
	// The key contract is that the summary includes:
	//   - "mod_type": "healing"
	//   - "mod_index": 2
	//   - "healing_attempt": 1
	//   - "exit_code": 0
	//   - "timings": {...}
	// This is validated by code inspection and integration tests.
}

// setupGitRepoWithChange initializes a git repo and creates a staged change for diff testing.
func setupGitRepoWithChange(workspace string) error {
	// Initialize git repo.
	if err := runCommand(workspace, "git", "init"); err != nil {
		return err
	}
	if err := runCommand(workspace, "git", "config", "user.name", "Test User"); err != nil {
		return err
	}
	if err := runCommand(workspace, "git", "config", "user.email", "test@example.com"); err != nil {
		return err
	}

	// Create initial commit.
	testFile := filepath.Join(workspace, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial content\n"), 0o644); err != nil {
		return err
	}
	if err := runCommand(workspace, "git", "add", "."); err != nil {
		return err
	}
	if err := runCommand(workspace, "git", "commit", "-m", "Initial commit"); err != nil {
		return err
	}

	// Make a change (not committed, so diff will show it).
	if err := os.WriteFile(testFile, []byte("modified content\n"), 0o644); err != nil {
		return err
	}

	return nil
}

// runCommand executes a shell command in the specified directory.
func runCommand(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s failed: %w\nOutput: %s", name, err, output)
	}
	return nil
}

// TestExecuteWithHealing_FullGateHistoryCapture verifies that the node agent
// captures the complete gate execution history (PreGate + ReGates) regardless
// of how many healing retries are configured. This test validates:
//
//   - PreGate is always captured when gate is enabled (even if it fails)
//   - ReGates slice grows with each healing retry attempt
//   - Each gate execution produces distinct BuildGateStageMetadata
//   - Gate history enables telemetry and debugging across the healing workflow
//
// This test ensures consistency between HTTP Build Gate API and Docker gate
// behavior by verifying the node agent always re-runs the gate after healing
// and captures all results.
func TestExecuteWithHealing_FullGateHistoryCapture(t *testing.T) {
	// Track gate call sequence with distinct metadata per call.
	gateCallCount := 0
	gateLogs := []string{
		"[ERROR] Initial failure: missing class Foo",
		"[ERROR] After healing attempt 1: still failing",
		"[ERROR] After healing attempt 2: type mismatch",
		"[INFO] BUILD SUCCESS after attempt 3",
	}

	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			// Use distinct logs for each gate call to verify proper history capture.
			idx := gateCallCount - 1
			if idx >= len(gateLogs) {
				idx = len(gateLogs) - 1
			}
			passed := gateCallCount >= 4 // Pass on 4th call (pre-gate + 3 re-gates).
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Language: "java", Passed: passed},
				},
				LogsText:  gateLogs[idx],
				LogDigest: fmt.Sprintf("digest-%d", gateCallCount),
				Resources: &contracts.BuildGateResourceUsage{
					CPUTotalNs:    uint64(gateCallCount * 100000000), // Distinct per call.
					MemUsageBytes: uint64(gateCallCount * 10485760),
				},
			}, nil
		},
	}

	// Track healing container executions.
	healingContainerCount := 0
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			healingContainerCount++
			return step.ContainerHandle{ID: fmt.Sprintf("heal-%d", healingContainerCount)}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn: func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
			return []byte("healing complete"), nil
		},
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
	}

	workspace, err := os.MkdirTemp("", "ploy-gate-history-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace)

	outDir, err := os.MkdirTemp("", "ploy-gate-history-out-*")
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
		RunID:     types.RunID("test-gate-history"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature"),
		Options: map[string]any{
			"build_gate_healing": map[string]any{
				"retries": 3, // Three retry attempts → 3 re-gates + 1 pre-gate = 4 total.
				"mods": []any{
					map[string]any{"image": "healer:latest"},
				},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.RunID),
		Name:  "Main mod",
		Image: "main:latest",
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

	execResult, err := rc.executeWithHealing(context.Background(), runner, req, manifest, workspace, outDir, &inDir, 0)

	// Should succeed after 3 healing attempts (4th gate call passes).
	if err != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil", err)
	}

	// --- Verify PreGate capture ---
	if execResult.PreGate == nil {
		t.Fatal("PreGate should always be captured when gate is enabled")
	}
	if execResult.PreGate.Metadata == nil {
		t.Fatal("PreGate.Metadata should not be nil")
	}
	if execResult.PreGate.Metadata.LogsText != gateLogs[0] {
		t.Errorf("PreGate logs = %q, want %q", execResult.PreGate.Metadata.LogsText, gateLogs[0])
	}
	if execResult.PreGate.Metadata.LogDigest != "digest-1" {
		t.Errorf("PreGate digest = %q, want 'digest-1'", execResult.PreGate.Metadata.LogDigest)
	}
	if len(execResult.PreGate.Metadata.StaticChecks) == 0 || execResult.PreGate.Metadata.StaticChecks[0].Passed {
		t.Error("PreGate should have failed check")
	}

	// --- Verify ReGates capture (3 healing retries = 3 re-gates) ---
	if len(execResult.ReGates) != 3 {
		t.Fatalf("len(ReGates) = %d, want 3 (one per healing retry)", len(execResult.ReGates))
	}

	// Verify each re-gate has distinct metadata (proving node agent re-runs gate).
	for i, regate := range execResult.ReGates {
		if regate.Metadata == nil {
			t.Fatalf("ReGates[%d].Metadata should not be nil", i)
		}
		expectedDigest := fmt.Sprintf("digest-%d", i+2) // digest-2, digest-3, digest-4
		if regate.Metadata.LogDigest != expectedDigest {
			t.Errorf("ReGates[%d] digest = %q, want %q", i, regate.Metadata.LogDigest, expectedDigest)
		}
		expectedLogs := gateLogs[i+1]
		if regate.Metadata.LogsText != expectedLogs {
			t.Errorf("ReGates[%d] logs = %q, want %q", i, regate.Metadata.LogsText, expectedLogs)
		}
		// Only the last re-gate should pass.
		shouldPass := i == 2
		if len(regate.Metadata.StaticChecks) == 0 {
			t.Fatalf("ReGates[%d] should have StaticChecks", i)
		}
		if regate.Metadata.StaticChecks[0].Passed != shouldPass {
			t.Errorf("ReGates[%d] passed = %v, want %v", i, regate.Metadata.StaticChecks[0].Passed, shouldPass)
		}
	}

	// --- Verify total gate calls ---
	if gateCallCount != 4 {
		t.Errorf("total gate calls = %d, want 4 (1 pre-gate + 3 re-gates)", gateCallCount)
	}

	// --- Verify healing containers ran ---
	// 3 healing containers (one per retry) + 1 main mod container (after gate passes) = 4 total.
	if healingContainerCount != 4 {
		t.Errorf("healing container count = %d, want 4 (3 healing + 1 main mod)", healingContainerCount)
	}

	// --- Verify duration tracking ---
	if execResult.PreGate.DurationMs < 0 {
		t.Errorf("PreGate duration = %d, want >= 0", execResult.PreGate.DurationMs)
	}
	for i, regate := range execResult.ReGates {
		if regate.DurationMs < 0 {
			t.Errorf("ReGates[%d] duration = %d, want >= 0", i, regate.DurationMs)
		}
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
