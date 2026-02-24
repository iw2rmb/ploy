package nodeagent

import (
	"context"
	"fmt"
	"os"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func testLogDigest(n int) types.Sha256Digest {
	return types.Sha256Digest(fmt.Sprintf("sha256:%064x", n))
}

// TestExecuteWithHealing_PostGate_PassesWithoutHealing verifies the post-mig gate path when:
// - Pre-mig gate passes immediately (no healing)
// - Main mig executes and exits 0
// - Post-mig gate passes immediately (no healing)
//
// This test validates that runGateWithHealing is invoked with gatePhase="post" and
// the post-mig gate metadata is correctly appended to ReGates and stored in BuildGate.
func TestExecuteWithHealing_PostGate_PassesWithoutHealing(t *testing.T) {
	// Track gate calls to distinguish pre-mig vs post-mig gates.
	var gateCalls []string
	gateCallCount := 0

	// Mock gate: all gates pass immediately.
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			// Gate is called twice by runner.Run (pre-mig) and once by post-mig gate.
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

	// Track container execution to verify main mig runs.
	containerCallCount := 0
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			containerCallCount++
			return step.ContainerHandle("mock"), nil
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
		Name:  "Main mig",
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

	// Main mig should exit 0.
	if execResult.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", execResult.ExitCode)
	}

	// PreGate should capture the initial pre-mig gate (from runner.Run).
	if execResult.PreGate == nil {
		t.Fatal("PreGate should be captured")
	}
	if !execResult.PreGate.Metadata.StaticChecks[0].Passed {
		t.Error("PreGate should have passed")
	}

	// ReGates should contain exactly one entry: the post-mig gate.
	// (Pre-mig gate passes immediately, so no pre-mig re-gates.)
	if len(execResult.ReGates) != 1 {
		t.Fatalf("len(ReGates) = %d, want 1 (post-mig gate only)", len(execResult.ReGates))
	}

	// Verify the post-mig gate metadata.
	postGate := execResult.ReGates[0]
	if postGate.Metadata == nil {
		t.Fatal("post-mig gate metadata should not be nil")
	}
	if !postGate.Metadata.StaticChecks[0].Passed {
		t.Error("post-mig gate should have passed")
	}

	// result.BuildGate should be updated to the post-mig gate result.
	if execResult.BuildGate == nil {
		t.Fatal("BuildGate should be set to post-mig gate result")
	}
	// The post-mig gate is the second gate call (after pre-mig from runner.Run).
	if execResult.BuildGate.LogDigest != testLogDigest(2) {
		t.Errorf("BuildGate.LogDigest = %q, want %q (post-mig gate)", execResult.BuildGate.LogDigest, testLogDigest(2))
	}

	// Verify gate call count: 1 pre-mig (from runner.Run) + 1 post-mig = 2.
	if gateCallCount != 2 {
		t.Errorf("gate call count = %d, want 2", gateCallCount)
	}

	// Verify main mig container ran.
	if containerCallCount != 1 {
		t.Errorf("container call count = %d, want 1 (main mig only)", containerCallCount)
	}
}

// TestExecuteWithHealing_PostGate_FailsOnceHealsThenPasses verifies the post-mig gate path when:
// - Pre-mig gate passes immediately (no healing)
// - Main mig executes and exits 0
// - Post-mig gate fails on first check
// - Healing mig executes
// - Post-mig re-gate passes
//
// This test validates that post-mig gates use runGateWithHealing for consistent healing behavior.
func TestExecuteWithHealing_PostGate_FailsOnceHealsThenPasses(t *testing.T) {
	// Track call sequence to verify orchestration order.
	var callSequence []string
	gateCallCount := 0

	// Mock gate: pre-mig passes, first post-mig fails, second post-mig (re-gate) passes.
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			callSequence = append(callSequence, fmt.Sprintf("gate-%d", gateCallCount))

			switch gateCallCount {
			case 1:
				// Pre-mig gate passes (from runner.Run).
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: true}},
					LogsText:     "[INFO] Pre-mig success\n",
					LogDigest:    testLogDigest(1),
				}, nil
			case 2:
				// Post-mig gate fails (triggers healing).
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}},
					LogsText:     "[ERROR] Post-mig failure\n",
					LogDigest:    testLogDigest(2),
				}, nil
			default:
				// Post-mig re-gate passes after healing.
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: true}},
					LogsText:     "[INFO] Post-mig success after healing\n",
					LogDigest:    testLogDigest(3),
				}, nil
			}
		},
	}

	// Track container execution: main mig + healing mig.
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			callSequence = append(callSequence, "container:"+spec.Image)
			return step.ContainerHandle("mock"), nil
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
				Mod:     ModContainerSpec{Image: contracts.JobImage{Universal: "healer:latest"}},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.JobID),
		Name:  "Main mig",
		Image: "main:latest",
		Inputs: []contracts.StepInput{
			{Name: "workspace", MountPath: "/workspace", Mode: contracts.StepInputModeReadWrite},
		},
		Gate: &contracts.StepGateSpec{Enabled: true},
	}

	execResult, err := rc.executeWithHealing(context.Background(), runner, req, manifest, workspace, outDir, &inDir, 0)

	// Should succeed after post-mig healing.
	if err != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil", err)
	}

	// Main mig should exit 0.
	if execResult.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", execResult.ExitCode)
	}

	// Verify call sequence:
	// 1. gate-1 (pre-mig, passes)
	// 2. container:main:latest (main mig)
	// 3. gate-2 (post-mig, fails)
	// 4. container:healer:latest (healing mig)
	// 5. gate-3 (post-mig re-gate, passes)
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

	// PreGate should capture the pre-mig gate.
	if execResult.PreGate == nil {
		t.Fatal("PreGate should be captured")
	}
	if execResult.PreGate.Metadata.LogDigest != testLogDigest(1) {
		t.Errorf("PreGate.LogDigest = %q, want %q", execResult.PreGate.Metadata.LogDigest, testLogDigest(1))
	}

	// ReGates should contain 2 entries: initial post-mig gate (failed) + post-mig re-gate (passed).
	if len(execResult.ReGates) != 2 {
		t.Fatalf("len(ReGates) = %d, want 2 (post-mig + post-mig re-gate)", len(execResult.ReGates))
	}

	// First entry: initial post-mig gate (failed).
	if execResult.ReGates[0].Metadata.LogDigest != testLogDigest(2) {
		t.Errorf("ReGates[0].LogDigest = %q, want %q", execResult.ReGates[0].Metadata.LogDigest, testLogDigest(2))
	}
	if execResult.ReGates[0].Metadata.StaticChecks[0].Passed {
		t.Error("ReGates[0] should have failed")
	}

	// Second entry: post-mig re-gate (passed after healing).
	if execResult.ReGates[1].Metadata.LogDigest != testLogDigest(3) {
		t.Errorf("ReGates[1].LogDigest = %q, want %q", execResult.ReGates[1].Metadata.LogDigest, testLogDigest(3))
	}
	if !execResult.ReGates[1].Metadata.StaticChecks[0].Passed {
		t.Error("ReGates[1] should have passed")
	}

	// result.BuildGate should be updated to the final post-mig re-gate result.
	if execResult.BuildGate == nil {
		t.Fatal("BuildGate should be set")
	}
	if execResult.BuildGate.LogDigest != testLogDigest(3) {
		t.Errorf("BuildGate.LogDigest = %q, want %q", execResult.BuildGate.LogDigest, testLogDigest(3))
	}

	// Gate call count: 1 pre-mig + 1 post-mig + 1 post-mig re-gate = 3.
	if gateCallCount != 3 {
		t.Errorf("gate call count = %d, want 3", gateCallCount)
	}
}
