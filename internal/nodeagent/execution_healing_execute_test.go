package nodeagent

import (
	"context"
	"os"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
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
	defer func() { _ = os.RemoveAll(workspace) }()

	outDir, err := os.MkdirTemp("", "ploy-test-out-*")
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
		RunID:     types.RunID("test-run-stats"),
		JobID:     types.JobID("test-job-stats"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("test-branch"),
		Env:       map[string]string{},
		TypedOptions: RunOptions{
			Healing: &HealingConfig{
				Retries: 1,
				Mod: ModContainerSpec{
					Image: contracts.JobImage{Universal: "test/healer:latest"},
				},
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID(req.JobID),
		Name:  "Main mig",
		Image: "test/main-mig:latest",
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
		},
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

	// Verify re-gate stats are captured (1 pre-mig re-gate + 1 post-mig gate = 2 total).
	if len(execResult.ReGates) != 2 {
		t.Fatalf("len(execResult.ReGates) = %d, want 2 (1 pre-mig re-gate + 1 post-mig gate)", len(execResult.ReGates))
	}

	// First re-gate: pre-mig healing re-gate.
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

	// Second re-gate: post-mig gate.
	postGate := execResult.ReGates[1]
	if postGate.Metadata == nil {
		t.Fatal("post-mig gate metadata should not be nil")
	}

	// Verify duration is tracked (should be >= 0).
	if execResult.PreGate.DurationMs < 0 {
		t.Errorf("pre-gate duration = %d, want >= 0", execResult.PreGate.DurationMs)
	}

	if reGate.DurationMs < 0 {
		t.Errorf("re-gate duration = %d, want >= 0", reGate.DurationMs)
	}
}
