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
		Workspace: &mockWorkspaceHydrator{},
		Gate: &mockGateExecutor{
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
			Profile: "java",
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
		Workspace: &mockWorkspaceHydrator{},
		Gate: &mockGateExecutor{
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
			Profile: "java",
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

// TestRunner_Run_FallbackToShiftSpec verifies that the deprecated Shift spec
// is used as a fallback when Gate spec is not present. This maintains backward
// compatibility with older manifests.
func TestRunner_Run_FallbackToShiftSpec(t *testing.T) {
	gateExecuted := false
	var capturedProfile string

	runner := Runner{
		Workspace: &mockWorkspaceHydrator{},
		Gate: &mockGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				gateExecuted = true
				capturedProfile = spec.Profile
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "test", Passed: true},
					},
				}, nil
			},
		},
	}

	// Use deprecated Shift spec instead of Gate
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
		Shift: &contracts.StepShiftSpec{
			Enabled: true,
			Profile: "legacy-profile",
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
		t.Errorf("Run() gate executor not called with Shift spec fallback")
	}

	if capturedProfile != "legacy-profile" {
		t.Errorf("Run() profile = %q, want 'legacy-profile'", capturedProfile)
	}

	if result.BuildGate == nil {
		t.Errorf("Run() BuildGate metadata not captured with Shift fallback")
	}
}

// TestRunner_Run_GatePrecedenceOverShift verifies that when both Gate and Shift
// specs are present, the Gate spec takes precedence over the deprecated Shift spec.
func TestRunner_Run_GatePrecedenceOverShift(t *testing.T) {
	var capturedProfile string
	runner := Runner{
		Workspace: &mockWorkspaceHydrator{},
		Gate: &mockGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				capturedProfile = spec.Profile
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
		Inputs: []contracts.StepInput{{
			Name:        "source",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadOnly,
			SnapshotCID: types.CID("bafytest123"),
		}},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
			Profile: "gate-profile",
		},
		Shift: &contracts.StepShiftSpec{
			Enabled: true,
			Profile: "shift-profile",
		},
	}

	_, err := runner.Run(context.Background(), Request{Manifest: manifest, Workspace: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if capturedProfile != "gate-profile" {
		t.Errorf("Run() used profile %q, want 'gate-profile' when Gate present", capturedProfile)
	}
}

// TestRunner_Run_GateExecutionFailure verifies that gate executor errors are
// properly propagated to the caller when gate execution fails.
func TestRunner_Run_GateExecutionFailure(t *testing.T) {
	expectedErr := errors.New("gate execution failed")
	runner := Runner{
		Workspace: &mockWorkspaceHydrator{},
		Gate: &mockGateExecutor{
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
			Profile: "java",
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

// TestRunner_Run_PreModGateFailureWithoutHealing verifies that when the pre-mod
// gate fails and no healing is configured, the runner returns an error with the
// build-gate sentinel error without executing the mod step.
func TestRunner_Run_PreModGateFailureWithoutHealing(t *testing.T) {
	runner := Runner{
		Workspace: &mockWorkspaceHydrator{},
		Gate: &mockGateExecutor{
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
		Containers: nil, // No container runtime; should not execute mod when gate fails
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
			Profile: "java",
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
		t.Fatalf("Run() expected error for failed pre-mod gate, got nil")
	}

	// Error should wrap the sentinel ErrBuildGateFailed
	if !errors.Is(err, ErrBuildGateFailed) {
		// Check error string contains expected message
		errStr := err.Error()
		if errStr != "build gate failed: pre-mod validation failed" {
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
		Workspace: &mockWorkspaceHydrator{},
		Gate: &mockGateExecutor{
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
			Profile: "java",
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
	if result.Timings.BuildGateDuration < gateDelay {
		t.Errorf("Run() BuildGateDuration = %v, expected >= %v", result.Timings.BuildGateDuration, gateDelay)
	}
}
