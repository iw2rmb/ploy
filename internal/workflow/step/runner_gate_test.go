package step

import (
	"context"
	"errors"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestRunner_Run_WithBuildGateEnabled verifies that the build gate executor
// is invoked when the gate is enabled in the manifest and that gate metadata
// is properly captured in the result.
func TestRunner_Run_WithBuildGateEnabled(t *testing.T) {
	gateExecuted := false
	runner := Runner{
		Workspace: &testWorkspaceHydrator{},
		Gate: &testGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				gateExecuted = true
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{
							Tool:   "checkstyle",
							Passed: true,
						},
					},
				}, nil
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "maven:3-eclipse-temurin-17",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: types.CID("bafytest123"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
	}

	result, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if !gateExecuted {
		t.Errorf("Run() gate executor not called when enabled")
	}

	if result.BuildGate == nil {
		t.Errorf("Run() BuildGate metadata not captured")
	} else if len(result.BuildGate.StaticChecks) != 1 {
		t.Errorf("Run() BuildGate.StaticChecks = %d, want 1", len(result.BuildGate.StaticChecks))
	}

	if result.Timings.BuildGateDuration == 0 {
		t.Errorf("Run() BuildGateDuration not captured when gate enabled")
	}
}

// TestRunner_Run_WithBuildGateDisabled verifies that the build gate executor
// is not invoked when the gate is disabled in the manifest.
func TestRunner_Run_WithBuildGateDisabled(t *testing.T) {
	gateExecuted := false
	runner := Runner{
		Workspace: &testWorkspaceHydrator{},
		Gate: &testGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				gateExecuted = true
				return nil, nil
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "maven:3-eclipse-temurin-17",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: types.CID("bafytest123"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: false,
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
	}

	result, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if gateExecuted {
		t.Errorf("Run() gate executor called when disabled")
	}

	if result.BuildGate != nil {
		t.Errorf("Run() BuildGate metadata should be nil when disabled")
	}
}

// (Legacy fallback and precedence tests removed; Gate is the only supported spec.)

// TestRunner_Run_GateExecutionFailure verifies that gate executor errors are
// properly propagated to the caller when gate execution fails.
func TestRunner_Run_GateExecutionFailure(t *testing.T) {
	expectedErr := errors.New("gate execution failed")
	runner := Runner{
		Workspace: &testWorkspaceHydrator{},
		Gate: &testGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				return nil, expectedErr
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: types.CID("bafytest123"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
	}

	_, err := runner.Run(context.Background(), req)
	if err == nil {
		t.Fatalf("Run() expected error, got nil")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("Run() error chain doesn't include gate execution error: %v", err)
	}
}

// TestRunner_Run_PreModGateFailureWithoutHealing verifies that when the pre-mig
// gate fails and no healing is configured, the runner returns an error with the
// build-gate sentinel error without executing the mig step.
func TestRunner_Run_PreModGateFailureWithoutHealing(t *testing.T) {
	runner := Runner{
		Workspace: &testWorkspaceHydrator{},
		Gate: &testGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				// Simulate gate failure
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{
							Tool:   "maven",
							Passed: false,
						},
					},
					LogsText: "[ERROR] BUILD FAILURE\n[ERROR] Failed to compile",
				}, nil
			},
		},
		Containers: nil, // No container runtime; should not execute mig when gate fails
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "maven:3-eclipse-temurin-17",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: types.CID("bafytest123"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
		},
		// No build_gate_healing configured
		Options: map[string]any{},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
	}

	result, err := runner.Run(context.Background(), req)

	// Should return error when gate fails without healing
	if err == nil {
		t.Fatalf("Run() expected error for failed pre-mig gate, got nil")
	}

	// Error should wrap the sentinel ErrBuildGateFailed
	if !errors.Is(err, ErrBuildGateFailed) {
		// Check error string contains expected message
		errStr := err.Error()
		if errStr != "build gate failed: pre-mig validation failed" {
			t.Errorf("Run() error = %q, want error containing 'build gate failed'", errStr)
		}
	}

	// BuildGate metadata should be populated
	if result.BuildGate == nil {
		t.Errorf("Run() BuildGate metadata should be populated on gate failure")
	} else {
		if len(result.BuildGate.StaticChecks) != 1 {
			t.Errorf("Run() BuildGate.StaticChecks = %d, want 1", len(result.BuildGate.StaticChecks))
		}
		if result.BuildGate.StaticChecks[0].Passed {
			t.Errorf("Run() BuildGate.StaticChecks[0].Passed = true, want false")
		}
	}

	// Timings should be populated even on early failure
	if result.Timings.BuildGateDuration == 0 {
		t.Errorf("Run() BuildGateDuration should be captured on gate failure")
	}
	if result.Timings.TotalDuration == 0 {
		t.Errorf("Run() TotalDuration should be captured on gate failure")
	}
}

// TestRunner_Run_GateTimingCapture verifies that build gate timing is accurately
// measured and captured in the result timings when the gate is enabled.
func TestRunner_Run_GateTimingCapture(t *testing.T) {
	gateDelay := 5 * time.Millisecond

	runner := Runner{
		Workspace: &testWorkspaceHydrator{},
		Gate: &testGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				time.Sleep(gateDelay)
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "test", Passed: true},
					},
				}, nil
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: types.CID("bafytest123"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
	}

	result, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	// Verify gate timing measurement is reasonable
	if time.Duration(result.Timings.BuildGateDuration) < gateDelay {
		t.Errorf("Run() BuildGateDuration = %v, expected >= %v", result.Timings.BuildGateDuration, gateDelay)
	}
}

// -----------------------------------------------------------------------------
// RunGateOnly Tests
// -----------------------------------------------------------------------------

// TestRunGateOnly_Enabled verifies that RunGateOnly invokes the gate executor
// when the gate is enabled, populates BuildGate metadata, and does NOT invoke
// any container runtime methods.
func TestRunGateOnly_Enabled(t *testing.T) {
	gateExecuted := false
	crt := &testContainerRuntime{}

	runner := &Runner{
		Workspace: &testWorkspaceHydrator{},
		Gate: &testGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				gateExecuted = true
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{
							Tool:   "checkstyle",
							Passed: true,
						},
					},
				}, nil
			},
		},
		Containers: crt,
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "maven:3-eclipse-temurin-17",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: types.CID("bafytest123"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
	}

	result, err := RunGateOnly(context.Background(), runner, req)
	if err != nil {
		t.Fatalf("RunGateOnly() unexpected error: %v", err)
	}

	// Verify gate was executed.
	if !gateExecuted {
		t.Errorf("RunGateOnly() gate executor not called when enabled")
	}

	// Verify container runtime was NOT invoked.
	if crt.createCalled {
		t.Errorf("RunGateOnly() should not create containers")
	}

	// Verify BuildGate metadata is populated.
	if result.BuildGate == nil {
		t.Errorf("RunGateOnly() BuildGate metadata not captured")
	} else if len(result.BuildGate.StaticChecks) != 1 {
		t.Errorf("RunGateOnly() BuildGate.StaticChecks = %d, want 1", len(result.BuildGate.StaticChecks))
	}

	// Verify timings are captured.
	if result.Timings.BuildGateDuration == 0 {
		t.Errorf("RunGateOnly() BuildGateDuration not captured when gate enabled")
	}
	if result.Timings.TotalDuration == 0 {
		t.Errorf("RunGateOnly() TotalDuration not captured")
	}

	// Verify ExitCode is 0 (no container was executed).
	if result.ExitCode != 0 {
		t.Errorf("RunGateOnly() ExitCode = %d, want 0", result.ExitCode)
	}
}

// TestRunGateOnly_Disabled verifies that RunGateOnly does NOT invoke the gate
// executor when the gate is disabled in the manifest, and that no container
// runtime methods are invoked.
func TestRunGateOnly_Disabled(t *testing.T) {
	gateExecuted := false
	crt := &testContainerRuntime{}

	runner := &Runner{
		Workspace: &testWorkspaceHydrator{},
		Gate: &testGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				gateExecuted = true
				return nil, nil
			},
		},
		Containers: crt,
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "maven:3-eclipse-temurin-17",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: types.CID("bafytest123"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: false,
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
	}

	result, err := RunGateOnly(context.Background(), runner, req)
	if err != nil {
		t.Fatalf("RunGateOnly() unexpected error: %v", err)
	}

	// Verify gate was NOT executed.
	if gateExecuted {
		t.Errorf("RunGateOnly() gate executor called when disabled")
	}

	// Verify container runtime was NOT invoked.
	if crt.createCalled {
		t.Errorf("RunGateOnly() should not create containers")
	}

	// Verify BuildGate metadata is nil when disabled.
	if result.BuildGate != nil {
		t.Errorf("RunGateOnly() BuildGate metadata should be nil when disabled")
	}

	// Verify ExitCode is 0.
	if result.ExitCode != 0 {
		t.Errorf("RunGateOnly() ExitCode = %d, want 0", result.ExitCode)
	}
}

// TestRunGateOnly_GateFailure verifies that RunGateOnly returns ErrBuildGateFailed
// when the gate validation fails, allowing callers to trigger healing.
func TestRunGateOnly_GateFailure(t *testing.T) {
	runner := &Runner{
		Workspace: &testWorkspaceHydrator{},
		Gate: &testGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				// Simulate gate failure.
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{
							Tool:   "maven",
							Passed: false,
						},
					},
					LogsText: "[ERROR] BUILD FAILURE",
				}, nil
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "maven:3-eclipse-temurin-17",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: types.CID("bafytest123"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
	}

	result, err := RunGateOnly(context.Background(), runner, req)

	// Should return error when gate fails.
	if err == nil {
		t.Fatalf("RunGateOnly() expected error for failed gate, got nil")
	}

	// Error should wrap ErrBuildGateFailed sentinel.
	if !errors.Is(err, ErrBuildGateFailed) {
		t.Errorf("RunGateOnly() error should wrap ErrBuildGateFailed: %v", err)
	}

	// BuildGate metadata should be populated even on failure.
	if result.BuildGate == nil {
		t.Errorf("RunGateOnly() BuildGate metadata should be populated on gate failure")
	} else if result.BuildGate.StaticChecks[0].Passed {
		t.Errorf("RunGateOnly() BuildGate.StaticChecks[0].Passed = true, want false")
	}

	// Timings should be captured even on early failure.
	if result.Timings.BuildGateDuration == 0 {
		t.Errorf("RunGateOnly() BuildGateDuration should be captured on gate failure")
	}
	if result.Timings.TotalDuration == 0 {
		t.Errorf("RunGateOnly() TotalDuration should be captured on gate failure")
	}
}

// TestRunGateOnly_NilGate verifies that RunGateOnly succeeds without error
// when no gate executor is configured.
func TestRunGateOnly_NilGate(t *testing.T) {
	runner := &Runner{
		Workspace: &testWorkspaceHydrator{},
		Gate:      nil, // No gate executor configured.
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: types.CID("bafytest123"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
	}

	result, err := RunGateOnly(context.Background(), runner, req)
	if err != nil {
		t.Fatalf("RunGateOnly() unexpected error with nil gate: %v", err)
	}

	// BuildGate should be nil when no executor is configured.
	if result.BuildGate != nil {
		t.Errorf("RunGateOnly() BuildGate should be nil when gate executor is nil")
	}
}
