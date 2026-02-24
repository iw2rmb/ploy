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

// TestExecuteWithHealing_FinalGateFromHealingWhenMainModFails verifies that when the
// initial gate fails, healing succeeds, and the main mig exits with a non-zero code
// (so no post-mig gate runs), the final gate stored in Result.BuildGate reflects the
// last successful healing re-gate rather than the initial failing pre-gate.
func TestExecuteWithHealing_FinalGateFromHealingWhenMainModFails(t *testing.T) {
	// Gate call sequence:
	//  1. Pre-mig gate (fails)
	//  2. Healing re-gate (passes)
	gateCallCount := 0
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			switch gateCallCount {
			case 1:
				// Initial pre-mig gate failure.
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: false},
					},
					LogsText:  "[ERROR] Initial pre-gate failure\n",
					LogDigest: testLogDigest(1),
				}, nil
			case 2:
				// Re-gate after healing succeeds.
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: true},
					},
					LogsText:  "[INFO] Gate passed after healing\n",
					LogDigest: testLogDigest(2),
				}, nil
			default:
				t.Fatalf("unexpected gate call %d", gateCallCount)
				return nil, nil
			}
		},
	}

	// Container runtime: one healing mig (exit code 0) and one main mig (exit code 1).
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			switch spec.Image {
			case "test/healer-final-gate:latest":
				return step.ContainerHandle{ID: "healer"}, nil
			case "test/main-mig-final-gate:latest":
				return step.ContainerHandle{ID: "main"}, nil
			default:
				return step.ContainerHandle{ID: "unknown"}, nil
			}
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			switch handle.ID {
			case "healer":
				// Healing container succeeds.
				return step.ContainerResult{ExitCode: 0}, nil
			case "main":
				// Main mig exits with non-zero code to skip post-mig gate.
				return step.ContainerResult{ExitCode: 1}, nil
			default:
				return step.ContainerResult{ExitCode: 0}, nil
			}
		},
		logsFn: func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
			return []byte("logs"), nil
		},
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
	}

	workspace, err := os.MkdirTemp("", "ploy-final-gate-healing-ws-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()

	outDir, err := os.MkdirTemp("", "ploy-final-gate-healing-out-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    testNodeID,
		},
	}

	req := StartRunRequest{
		RunID:     types.RunID("test-final-gate-healing-main-fail"),
		JobID:     types.JobID("test-job-final-gate-healing-main-fail"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("test-branch"),
		Env:       map[string]string{},
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod: ModContainerSpec{
					Image: contracts.JobImage{Universal: "test/healer-final-gate:latest"},
				},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.JobID),
		Name:  "Main mig",
		Image: "test/main-mig-final-gate:latest",
		Inputs: []contracts.StepInput{
			{
				Name:        "workspace",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadWrite,
				SnapshotCID: types.CID("bafy-final-gate"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
		},
	}

	execResult, err := rc.executeWithHealing(context.Background(), runner, req, manifest, workspace, outDir, &inDir, 0)
	if err != nil {
		t.Fatalf("executeWithHealing() error = %v, want nil (main mig failure reported via exit code)", err)
	}

	// Main mig should have non-zero exit code.
	if execResult.ExitCode != 1 {
		t.Errorf("executeWithHealing() exit code = %d, want 1", execResult.ExitCode)
	}

	// PreGate should capture the initial failing gate.
	if execResult.PreGate == nil || execResult.PreGate.Metadata == nil {
		t.Fatalf("PreGate should be populated for initial failing gate")
	}
	if execResult.PreGate.Metadata.LogDigest != testLogDigest(1) {
		t.Errorf("PreGate.LogDigest = %q, want %q", execResult.PreGate.Metadata.LogDigest, testLogDigest(1))
	}

	// ReGates should contain the successful healing re-gate.
	if len(execResult.ReGates) != 1 {
		t.Fatalf("len(execResult.ReGates) = %d, want 1 (healing re-gate only)", len(execResult.ReGates))
	}
	finalReGate := execResult.ReGates[0]
	if finalReGate.Metadata == nil || finalReGate.Metadata.LogDigest != testLogDigest(2) {
		t.Fatalf("final re-gate metadata = %#v, want LogDigest=%q", finalReGate.Metadata, testLogDigest(2))
	}

	// Final gate in Result.BuildGate should reflect the last healing re-gate, not the initial pre-gate.
	if execResult.BuildGate == nil {
		t.Fatal("Result.BuildGate should be populated from final healing re-gate")
	}
	if execResult.BuildGate.LogDigest != testLogDigest(2) {
		t.Errorf("Result.BuildGate.LogDigest = %q, want %q", execResult.BuildGate.LogDigest, testLogDigest(2))
	}
	if len(execResult.BuildGate.StaticChecks) == 0 || !execResult.BuildGate.StaticChecks[0].Passed {
		t.Errorf("Result.BuildGate should represent a passing gate after healing")
	}

	// Only two gate executions should have occurred: initial pre-gate + healing re-gate.
	if gateCallCount != 2 {
		t.Errorf("gateCallCount = %d, want 2 (pre-gate + healing re-gate)", gateCallCount)
	}
}

// TestExecuteWithHealing_FullGateHistoryCapture verifies that the node agent
// captures the complete gate execution history (PreGate + ReGates) regardless
// of how many healing retries are configured. This test validates:
//
//   - PreGate is always captured when gate is enabled (even if it fails)
//   - ReGates slice grows with each healing retry attempt
//   - Each gate execution produces distinct BuildGateStageMetadata
//   - Gate history enables telemetry and debugging across the healing workflow
//   - Post-mig gate is also captured in ReGates after main mig succeeds
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
		"[INFO] Post-mig gate success", // Post-mig gate after main mig succeeds.
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
				LogDigest: testLogDigest(gateCallCount),
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
	defer func() { _ = os.RemoveAll(workspace) }()

	outDir, err := os.MkdirTemp("", "ploy-gate-history-out-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	inDir := ""

	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    testNodeID,
		},
	}

	req := StartRunRequest{
		RunID:     types.RunID("test-gate-history"),
		JobID:     types.JobID("test-job-gate-history"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature"),
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 3,
				Mod: ModContainerSpec{
					Image: contracts.JobImage{Universal: "healer:latest"},
				},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.JobID),
		Name:  "Main mig",
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
		},
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
	if execResult.PreGate.Metadata.LogDigest != testLogDigest(1) {
		t.Errorf("PreGate digest = %q, want %q", execResult.PreGate.Metadata.LogDigest, testLogDigest(1))
	}
	if len(execResult.PreGate.Metadata.StaticChecks) == 0 || execResult.PreGate.Metadata.StaticChecks[0].Passed {
		t.Error("PreGate should have failed check")
	}

	// --- Verify ReGates capture (3 pre-mig healing re-gates + 1 post-mig gate = 4 total) ---
	// With post-mig gate now enabled, ReGates contains:
	//   - ReGates[0..2]: 3 pre-mig healing re-gates (digest-2, digest-3, digest-4)
	//   - ReGates[3]: 1 post-mig gate (digest-5)
	if len(execResult.ReGates) != 4 {
		t.Fatalf("len(ReGates) = %d, want 4 (3 pre-mig re-gates + 1 post-mig gate)", len(execResult.ReGates))
	}

	// Verify each pre-mig re-gate has distinct metadata (proving node agent re-runs gate).
	for i := 0; i < 3; i++ {
		regate := execResult.ReGates[i]
		if regate.Metadata == nil {
			t.Fatalf("ReGates[%d].Metadata should not be nil", i)
		}
		expectedDigest := testLogDigest(i + 2) // digest-2, digest-3, digest-4
		if regate.Metadata.LogDigest != expectedDigest {
			t.Errorf("ReGates[%d] digest = %q, want %q", i, regate.Metadata.LogDigest, expectedDigest)
		}
		expectedLogs := gateLogs[i+1]
		if regate.Metadata.LogsText != expectedLogs {
			t.Errorf("ReGates[%d] logs = %q, want %q", i, regate.Metadata.LogsText, expectedLogs)
		}
		// Only the last pre-mig re-gate (index 2) should pass.
		shouldPass := i == 2
		if len(regate.Metadata.StaticChecks) == 0 {
			t.Fatalf("ReGates[%d] should have StaticChecks", i)
		}
		if regate.Metadata.StaticChecks[0].Passed != shouldPass {
			t.Errorf("ReGates[%d] passed = %v, want %v", i, regate.Metadata.StaticChecks[0].Passed, shouldPass)
		}
	}

	// Verify post-mig gate (ReGates[3]).
	postGate := execResult.ReGates[3]
	if postGate.Metadata == nil {
		t.Fatal("post-mig gate metadata should not be nil")
	}
	if postGate.Metadata.LogDigest != testLogDigest(5) {
		t.Errorf("post-mig gate digest = %q, want %q", postGate.Metadata.LogDigest, testLogDigest(5))
	}
	if !postGate.Metadata.StaticChecks[0].Passed {
		t.Error("post-mig gate should have passed")
	}

	// --- Verify total gate calls ---
	// 1 pre-gate + 3 pre-mig re-gates + 1 post-mig gate = 5 total.
	if gateCallCount != 5 {
		t.Errorf("total gate calls = %d, want 5 (1 pre-gate + 3 pre-mig re-gates + 1 post-mig gate)", gateCallCount)
	}

	// --- Verify healing containers ran ---
	// 3 healing containers (one per retry) + 1 main mig container (after gate passes) = 4 total.
	if healingContainerCount != 4 {
		t.Errorf("healing container count = %d, want 4 (3 healing + 1 main mig)", healingContainerCount)
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
