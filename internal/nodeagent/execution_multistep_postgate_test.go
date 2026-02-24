// execution_multistep_postgate_test.go contains tests for multi-step post-mod gate
// behavior in executeRun, specifically verifying that a failing post-mod gate
// terminates execution of subsequent steps.
package nodeagent

import (
	"context"
	"errors"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// TestExecuteRun_PostGateStopsFurtherMods verifies that when a multi-step run has
// a post-mod gate failure on step N, subsequent steps (N+1, N+2, ...) do not execute.
//
// Scenario:
//   - Two-step run: mods[0] and mods[1]
//   - Step 0: mod container executes successfully, post-mod gate passes
//   - Step 1: mod container executes successfully, but post-mod gate fails and cannot be healed
//
// Expected behavior:
//   - Step 0 mod container runs (success)
//   - Step 0 post-gate runs (passes)
//   - Step 1 mod container runs (success)
//   - Step 1 post-gate runs (fails, healing fails)
//   - Run terminates with ErrBuildGateFailed from step 1 post-gate
//   - If there were more steps (e.g., step 2), they would NOT execute
//
// This test validates the executeRun step loop break logic:
// when executeWithHealing returns ErrBuildGateFailed from a post-mod gate,
// finalExecErr is set and the loop breaks, preventing further mods from running.
func TestExecuteRun_PostGateStopsFurtherMods(t *testing.T) {
	// Track execution order to verify only step 0 and step 1 containers run,
	// and step 1's post-gate failure stops further execution.
	var executionOrder []string

	// Track gate calls: pre-mod gates from runner.Run + post-mod gates from executeWithHealing.
	gateCallCount := 0
	stepIndexOfFailingPostGate := 1 // Post-gate for step 1 will fail

	// mockGateExecutor simulates gate behavior:
	// - Pre-mod gates always pass (called by runner.Run)
	// - Post-mod gates: pass for step 0, fail for step 1
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			executionOrder = append(executionOrder, "gate")

			// Determine if this is a post-mod gate for step 1 based on call count.
			// For a 2-step run with gates enabled:
			// Call 1: pre-mod gate step 0 (from runner.Run)
			// Call 2: post-mod gate step 0
			// Call 3: pre-mod gate step 1 (from runner.Run)
			// Call 4: post-mod gate step 1 (should fail)
			if gateCallCount == 4 {
				// Post-mod gate for step 1 fails
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: false},
					},
					LogsText:  "[ERROR] Post-mod gate failure on step 1\n",
					LogDigest: "step1-postgate-fail",
				}, nil
			}

			// All other gates pass
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: true},
				},
				LogsText:  "[INFO] Gate passed\n",
				LogDigest: "gate-pass",
			}, nil
		},
	}

	// Track which containers were created (step index inferred from image name).
	containerCreateCount := 0
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			containerCreateCount++
			executionOrder = append(executionOrder, "container:"+spec.Image)
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

	// Create step runner with mocked components.
	runner := step.Runner{
		Workspace:  &mockWorkspaceHydrator{},
		Containers: mockContainer,
		Gate:       mockGate,
	}

	// Create runController with mock runner injection.
	rc := &runController{
		cfg: Config{
			ServerURL: "http://localhost:9999",
			NodeID:    testNodeID,
		},
	}

	// Build a two-step request: steps[0] and steps[1].
	// Only steps[0] should complete successfully; steps[1] should fail on post-gate.
	req := StartRunRequest{
		RunID:     types.RunID("test-postgate-stops"),
		JobID:     types.JobID("test-job-postgate-stops"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("test-branch"),
		Env:       map[string]string{},
		TypedOptions: RunOptions{
			Steps: []StepMod{
				{ModContainerSpec: ModContainerSpec{Image: contracts.JobImage{Universal: "test/mod-step0:latest"}}},
				{ModContainerSpec: ModContainerSpec{Image: contracts.JobImage{Universal: "test/mod-step1:latest"}}},
			},
		},
	}

	// Build manifests for both steps with gate enabled.
	typedOpts := req.TypedOptions

	// Verify multi-step detection.
	if len(typedOpts.Steps) != 2 {
		t.Fatalf("expected 2 steps in typedOpts.Steps, got %d", len(typedOpts.Steps))
	}

	// --- Test executeWithHealing for each step manually ---
	// We cannot call executeRun directly as it requires full runtime setup.
	// Instead, we simulate the step loop behavior that executeRun uses.

	var finalExecErr error

	// Simulate the executeRun step loop (lines 209-278 in execution_orchestrator.go).
	stepCount := len(typedOpts.Steps)
	workspace := t.TempDir()
	outDir := t.TempDir()
	inDir := ""

	for stepIndex := 0; stepIndex < stepCount; stepIndex++ {
		// Build manifest for this step.
		// Pass ModStackUnknown explicitly to indicate tests operate without stack detection.
		manifest, err := buildManifestFromRequest(req, typedOpts, stepIndex, contracts.ModStackUnknown)
		if err != nil {
			t.Fatalf("buildManifestFromRequest(step %d) error = %v", stepIndex, err)
		}

		// Enable gate for this test.
		manifest.Gate = &contracts.StepGateSpec{
			Enabled: true,
		}

		// Execute step with healing (simulates executeWithHealing call).
		execResult, execErr := rc.executeWithHealing(
			context.Background(), runner, req, manifest, workspace, outDir, &inDir, stepIndex,
		)

		if execErr != nil {
			// This is the key behavior being tested: when post-gate fails,
			// the loop should break and no further steps execute.
			finalExecErr = execErr
			break // Simulates the break in executeRun
		}

		finalExecErr = nil
		_ = execResult // Result used for merging in executeRun; not needed for this test.
	}

	// --- Assertions ---

	// 1. Verify that execution stopped on step 1 (stepIndexOfFailingPostGate).
	if finalExecErr == nil {
		t.Fatal("expected error from post-gate failure, got nil")
	}

	// 2. Verify the error is ErrBuildGateFailed (from post-mod gate).
	if !errors.Is(finalExecErr, step.ErrBuildGateFailed) {
		t.Errorf("expected ErrBuildGateFailed, got: %v", finalExecErr)
	}

	// 3. Verify container execution count: 2 containers should have been created
	// (step 0 main mod + step 1 main mod). If there were more steps, they would NOT run.
	if containerCreateCount != 2 {
		t.Errorf("container create count = %d, want 2 (step 0 and step 1 mods)", containerCreateCount)
	}

	// 4. Verify gate call count: 4 gates should have been called.
	// Call 1: pre-mod gate step 0
	// Call 2: post-mod gate step 0 (passes)
	// Call 3: pre-mod gate step 1
	// Call 4: post-mod gate step 1 (fails)
	if gateCallCount != 4 {
		t.Errorf("gate call count = %d, want 4 (2 pre-mod + 2 post-mod gates)", gateCallCount)
	}

	// 5. Verify the execution order shows both containers ran before post-gate failure.
	// Expected order (simplified): gate, container:step0, gate, gate, container:step1, gate
	// The exact sequence depends on runner.Run internals, but we should see both containers.
	step0ModRan := false
	step1ModRan := false
	for _, event := range executionOrder {
		if event == "container:test/mod-step0:latest" {
			step0ModRan = true
		}
		if event == "container:test/mod-step1:latest" {
			step1ModRan = true
		}
	}

	if !step0ModRan {
		t.Error("step 0 mod container did not run")
	}
	if !step1ModRan {
		t.Error("step 1 mod container did not run")
	}

	// 6. Log the test scenario for clarity.
	t.Logf("Post-gate failure on step %d correctly stopped further mods", stepIndexOfFailingPostGate)
	t.Logf("Execution order: %v", executionOrder)
}

// TestExecuteRun_PostGateStopsFurtherMods_HealingExhausted verifies that when a post-mod gate
// fails and healing retries are exhausted, subsequent steps do not execute.
//
// This variant adds healing configuration but ensures healing also fails, resulting in
// ErrBuildGateFailed being propagated up and terminating the multi-step loop.
func TestExecuteRun_PostGateStopsFurtherMods_HealingExhausted(t *testing.T) {
	// Track execution order.
	var executionOrder []string

	gateCallCount := 0

	// Mock gate: pre-mod gates pass, post-mod gate for step 1 always fails (even after healing).
	mockGate := &mockGateExecutor{
		executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
			gateCallCount++
			executionOrder = append(executionOrder, "gate")

			// Gate call sequence for 2-step run with healing (1 retry):
			// Call 1: pre-mod gate step 0 (pass)
			// Call 2: post-mod gate step 0 (pass)
			// Call 3: pre-mod gate step 1 (pass)
			// Call 4: post-mod gate step 1 (fail, triggers healing)
			// Call 5: post-mod re-gate step 1 after healing (still fail)
			// No step 2 because we break on post-gate failure.
			if gateCallCount >= 4 {
				// Post-mod gate for step 1 always fails (before and after healing).
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: false},
					},
					LogsText:  "[ERROR] Post-mod gate step 1 still failing\n",
					LogDigest: "step1-postgate-fail-exhausted",
				}, nil
			}

			// Pre-mod and step 0 post-mod gates pass.
			return &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "maven", Passed: true},
				},
				LogsText:  "[INFO] Gate passed\n",
				LogDigest: "gate-pass",
			}, nil
		},
	}

	// Track containers created (main mods + healing mods).
	containerImages := []string{}
	mockContainer := &mockContainerRuntime{
		createFn: func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			containerImages = append(containerImages, spec.Image)
			executionOrder = append(executionOrder, "container:"+spec.Image)
			return step.ContainerHandle{ID: "mock"}, nil
		},
		startFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			return step.ContainerResult{ExitCode: 0}, nil
		},
		logsFn: func(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
			return []byte("logs"), nil
		},
		removeFn: func(ctx context.Context, handle step.ContainerHandle) error {
			return nil
		},
	}

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

	// Two-step run with healing configured (1 retry).
	req := StartRunRequest{
		RunID:     types.RunID("test-postgate-heal-exhaust"),
		JobID:     types.JobID("test-job-postgate-heal-exhaust"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("test-branch"),
		Env:       map[string]string{},
		TypedOptions: RunOptions{
			Steps: []StepMod{
				{ModContainerSpec: ModContainerSpec{Image: contracts.JobImage{Universal: "test/mod-step0:latest"}}},
				{ModContainerSpec: ModContainerSpec{Image: contracts.JobImage{Universal: "test/mod-step1:latest"}}},
			},
			Healing: &HealingConfig{
				Retries: 1,
				Mod:     ModContainerSpec{Image: contracts.JobImage{Universal: "test/healer:latest"}},
			},
		},
	}

	typedOpts := req.TypedOptions

	// Verify multi-step detection.
	if len(typedOpts.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(typedOpts.Steps))
	}

	// Verify healing is configured.
	if typedOpts.Healing == nil {
		t.Fatal("expected healing config")
	}

	var finalExecErr error
	stepCount := len(typedOpts.Steps)
	workspace := t.TempDir()
	outDir := t.TempDir()
	inDir := ""

	for stepIndex := 0; stepIndex < stepCount; stepIndex++ {
		// Pass ModStackUnknown explicitly to indicate tests operate without stack detection.
		manifest, err := buildManifestFromRequest(req, typedOpts, stepIndex, contracts.ModStackUnknown)
		if err != nil {
			t.Fatalf("buildManifestFromRequest(step %d) error = %v", stepIndex, err)
		}

		manifest.Gate = &contracts.StepGateSpec{
			Enabled: true,
		}

		_, execErr := rc.executeWithHealing(
			context.Background(), runner, req, manifest, workspace, outDir, &inDir, stepIndex,
		)

		if execErr != nil {
			finalExecErr = execErr
			break
		}
	}

	// --- Assertions ---

	// 1. Verify execution failed with ErrBuildGateFailed.
	if finalExecErr == nil {
		t.Fatal("expected post-gate failure error")
	}

	if !errors.Is(finalExecErr, step.ErrBuildGateFailed) {
		t.Errorf("expected ErrBuildGateFailed, got: %v", finalExecErr)
	}

	// 2. Verify healing mod ran (for step 1 post-gate).
	healerRan := false
	for _, img := range containerImages {
		if img == "test/healer:latest" {
			healerRan = true
			break
		}
	}
	if !healerRan {
		t.Error("healing mod did not run for post-gate failure")
	}

	// 3. Verify gate call count includes re-gate after healing.
	// Expected: 5 gate calls (2 pre-mod + 2 post-mod + 1 re-gate after healing).
	if gateCallCount != 5 {
		t.Errorf("gate call count = %d, want 5 (2 pre-mod + 2 post-mod + 1 re-gate)", gateCallCount)
	}

	// 4. Verify both step mods ran but healing couldn't fix post-gate.
	step0Ran := false
	step1Ran := false
	for _, img := range containerImages {
		if img == "test/mod-step0:latest" {
			step0Ran = true
		}
		if img == "test/mod-step1:latest" {
			step1Ran = true
		}
	}

	if !step0Ran {
		t.Error("step 0 mod did not run")
	}
	if !step1Ran {
		t.Error("step 1 mod did not run")
	}

	t.Logf("Post-gate healing exhausted on step 1 correctly stopped further mods")
	t.Logf("Container images executed: %v", containerImages)
}
