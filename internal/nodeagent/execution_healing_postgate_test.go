package nodeagent

import (
	"context"
	"fmt"
	"os"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

func testLogDigest(n int) types.Sha256Digest {
	return types.Sha256Digest(fmt.Sprintf("sha256:%064x", n))
}

// TestExecuteWithHealing_PostGate_PassesWithoutHealing verifies the post-mod gate path when:
// - Pre-mod gate passes immediately (no healing)
// - Main mod executes and exits 0
// - Post-mod gate passes immediately (no healing)
//
// This test validates that runGateWithHealing is invoked with gatePhase="post" and
// the post-mod gate metadata is correctly appended to ReGates and stored in BuildGate.
func TestExecuteWithHealing_PostGate_PassesWithoutHealing(t *testing.T) {
	// Track gate calls to distinguish pre-mod vs post-mod gates.
	var gateCalls []string
	gateCallCount := 0

	// Mock gate: all gates pass immediately.
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			// Gate is called twice by runner.Run (pre-mod) and once by post-mod gate.
			// runner.Run calls gate internally when Gate.Enabled=true.
			gateCalls = append(gateCalls, fmt.Sprintf("gate-%d", gateCallCount))
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: true},
				},
				LogsText:  fmt.Sprintf("[INFO] Gate %d success\n", gateCallCount),
				LogDigest: testLogDigest(gateCallCount),
			}, nil
		},
	}

	// Track container execution to verify main mod runs.
	containerCallCount := 0
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			containerCallCount++
			return step.ContainerHandle{ID: "mock"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn:   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) { return []byte("logs"), nil },
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error { return nil },
	}

	workspace, _ := os.MkdirTemp("", "ploy-postgate-pass-*")
	defer func() { _ = os.RemoveAll(workspace) }()
	outDir, _ := os.MkdirTemp("", "ploy-postgate-pass-out-*")
	defer func() { _ = os.RemoveAll(outDir) }()
	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{ServerURL: "http://localhost:9999", NodeID: testNodeID},
	}

	req := StartRunRequest{
		RunID:        types.RunID("test-postgate-pass"),
		TypedOptions: RunOptions{},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.RunID),
		Name:  "Main mod",
		Image: "main:latest",
		Inputs: []contracts.StepInput{
			{Name: "workspace", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite},
		},
		Gate: &contracts.StepGateSpec{Enabled: true},
	}

	execResult, err := rc.executeWithHealing(context.Background(), runner, req, manifest, workspace, outDir, &inDir, 0)

	// Should succeed without error.
	if err != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil", err)
	}

	// Main mod should exit 0.
	if execResult.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", execResult.ExitCode)
	}

	// PreGate should capture the initial pre-mod gate (from runner.Run).
	if execResult.PreGate == nil {
		t.Fatal("PreGate should be captured")
	}
	if !execResult.PreGate.Metadata.StaticChecks[0].Passed {
		t.Error("PreGate should have passed")
	}

	// ReGates should contain exactly one entry: the post-mod gate.
	// (Pre-mod gate passes immediately, so no pre-mod re-gates.)
	if len(execResult.ReGates) != 1 {
		t.Fatalf("len(ReGates) = %d, want 1 (post-mod gate only)", len(execResult.ReGates))
	}

	// Verify the post-mod gate metadata.
	postGate := execResult.ReGates[0]
	if postGate.Metadata == nil {
		t.Fatal("post-mod gate metadata should not be nil")
	}
	if !postGate.Metadata.StaticChecks[0].Passed {
		t.Error("post-mod gate should have passed")
	}

	// result.BuildGate should be updated to the post-mod gate result.
	if execResult.BuildGate == nil {
		t.Fatal("BuildGate should be set to post-mod gate result")
	}
	// The post-mod gate is the second gate call (after pre-mod from runner.Run).
	if execResult.BuildGate.LogDigest != testLogDigest(2) {
		t.Errorf("BuildGate.LogDigest = %q, want %q (post-mod gate)", execResult.BuildGate.LogDigest, testLogDigest(2))
	}

	// Verify gate call count: 1 pre-mod (from runner.Run) + 1 post-mod = 2.
	if gateCallCount != 2 {
		t.Errorf("gate call count = %d, want 2", gateCallCount)
	}

	// Verify main mod container ran.
	if containerCallCount != 1 {
		t.Errorf("container call count = %d, want 1 (main mod only)", containerCallCount)
	}
}

// TestExecuteWithHealing_PostGate_FailsOnceHealsThenPasses verifies the post-mod gate path when:
// - Pre-mod gate passes immediately (no healing)
// - Main mod executes and exits 0
// - Post-mod gate fails on first check
// - Healing mod executes
// - Post-mod re-gate passes
//
// This test validates that post-mod gates use runGateWithHealing for consistent healing behavior.
func TestExecuteWithHealing_PostGate_FailsOnceHealsThenPasses(t *testing.T) {
	// Track call sequence to verify orchestration order.
	var callSequence []string
	gateCallCount := 0

	// Mock gate: pre-mod passes, first post-mod fails, second post-mod (re-gate) passes.
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			callSequence = append(callSequence, fmt.Sprintf("gate-%d", gateCallCount))

			switch gateCallCount {
			case 1:
				// Pre-mod gate passes (from runner.Run).
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: true}},
					LogsText:     "[INFO] Pre-mod success\n",
					LogDigest:    testLogDigest(1),
				}, nil
			case 2:
				// Post-mod gate fails (triggers healing).
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}},
					LogsText:     "[ERROR] Post-mod failure\n",
					LogDigest:    testLogDigest(2),
				}, nil
			default:
				// Post-mod re-gate passes after healing.
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: true}},
					LogsText:     "[INFO] Post-mod success after healing\n",
					LogDigest:    testLogDigest(3),
				}, nil
			}
		},
	}

	// Track container execution: main mod + healing mod.
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

	workspace, _ := os.MkdirTemp("", "ploy-postgate-heal-*")
	defer func() { _ = os.RemoveAll(workspace) }()
	outDir, _ := os.MkdirTemp("", "ploy-postgate-heal-out-*")
	defer func() { _ = os.RemoveAll(outDir) }()
	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{ServerURL: "http://localhost:9999", NodeID: testNodeID},
	}

	req := StartRunRequest{
		RunID: types.RunID("test-postgate-heal"),
		JobID: types.JobID("test-job-postgate-heal"),
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod:     HealingMod{Image: contracts.ModImage{Universal: "healer:latest"}},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.JobID),
		Name:  "Main mod",
		Image: "main:latest",
		Inputs: []contracts.StepInput{
			{Name: "workspace", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite},
		},
		Gate: &contracts.StepGateSpec{Enabled: true},
	}

	execResult, err := rc.executeWithHealing(context.Background(), runner, req, manifest, workspace, outDir, &inDir, 0)

	// Should succeed after post-mod healing.
	if err != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil", err)
	}

	// Main mod should exit 0.
	if execResult.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", execResult.ExitCode)
	}

	// Verify call sequence:
	// 1. gate-1 (pre-mod, passes)
	// 2. container:main:latest (main mod)
	// 3. gate-2 (post-mod, fails)
	// 4. container:healer:latest (healing mod)
	// 5. gate-3 (post-mod re-gate, passes)
	expectedSequence := []string{
		"gate-1",
		"container:main:latest",
		"gate-2",
		"container:healer:latest",
		"gate-3",
	}
	if len(callSequence) != len(expectedSequence) {
		t.Fatalf("call sequence = %v, want %v", callSequence, expectedSequence)
	}
	for i, expected := range expectedSequence {
		if callSequence[i] != expected {
			t.Errorf("call sequence[%d] = %q, want %q", i, callSequence[i], expected)
		}
	}

	// PreGate should capture the pre-mod gate.
	if execResult.PreGate == nil {
		t.Fatal("PreGate should be captured")
	}
	if execResult.PreGate.Metadata.LogDigest != testLogDigest(1) {
		t.Errorf("PreGate.LogDigest = %q, want %q", execResult.PreGate.Metadata.LogDigest, testLogDigest(1))
	}

	// ReGates should contain 2 entries: initial post-mod gate (failed) + post-mod re-gate (passed).
	if len(execResult.ReGates) != 2 {
		t.Fatalf("len(ReGates) = %d, want 2 (post-mod + post-mod re-gate)", len(execResult.ReGates))
	}

	// First entry: initial post-mod gate (failed).
	if execResult.ReGates[0].Metadata.LogDigest != testLogDigest(2) {
		t.Errorf("ReGates[0].LogDigest = %q, want %q", execResult.ReGates[0].Metadata.LogDigest, testLogDigest(2))
	}
	if execResult.ReGates[0].Metadata.StaticChecks[0].Passed {
		t.Error("ReGates[0] should have failed")
	}

	// Second entry: post-mod re-gate (passed after healing).
	if execResult.ReGates[1].Metadata.LogDigest != testLogDigest(3) {
		t.Errorf("ReGates[1].LogDigest = %q, want %q", execResult.ReGates[1].Metadata.LogDigest, testLogDigest(3))
	}
	if !execResult.ReGates[1].Metadata.StaticChecks[0].Passed {
		t.Error("ReGates[1] should have passed")
	}

	// result.BuildGate should be updated to the final post-mod re-gate result.
	if execResult.BuildGate == nil {
		t.Fatal("BuildGate should be set")
	}
	if execResult.BuildGate.LogDigest != testLogDigest(3) {
		t.Errorf("BuildGate.LogDigest = %q, want %q", execResult.BuildGate.LogDigest, testLogDigest(3))
	}

	// Gate call count: 1 pre-mod + 1 post-mod + 1 post-mod re-gate = 3.
	if gateCallCount != 3 {
		t.Errorf("gate call count = %d, want 3", gateCallCount)
	}
}
