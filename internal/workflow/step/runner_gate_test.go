package step

import (
	"context"
	"errors"
	"testing"
	"time"

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
						{Tool: "checkstyle", Passed: true},
					},
				}, nil
			},
		},
	}

	manifest := newGateTestManifest(true)
	result, err := runner.Run(context.Background(), newGateTestRequest(manifest))
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

	manifest := newGateTestManifest(false)
	result, err := runner.Run(context.Background(), newGateTestRequest(manifest))
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

	manifest := newGateTestManifest(true)
	manifest.Image = "test:latest"
	_, err := runner.Run(context.Background(), newGateTestRequest(manifest))
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
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: false},
					},
					LogsText: "[ERROR] BUILD FAILURE\n[ERROR] Failed to compile",
				}, nil
			},
		},
		Containers: nil,
	}

	manifest := newGateTestManifest(true)
	manifest.Options = map[string]any{}
	result, err := runner.Run(context.Background(), newGateTestRequest(manifest))

	if err == nil {
		t.Fatalf("Run() expected error for failed pre-mig gate, got nil")
	}
	if !errors.Is(err, ErrBuildGateFailed) {
		errStr := err.Error()
		if errStr != "build gate failed: pre-mig validation failed" {
			t.Errorf("Run() error = %q, want error containing 'build gate failed'", errStr)
		}
	}

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

	manifest := newGateTestManifest(true)
	manifest.Image = "test:latest"
	result, err := runner.Run(context.Background(), newGateTestRequest(manifest))
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if time.Duration(result.Timings.BuildGateDuration) < gateDelay {
		t.Errorf("Run() BuildGateDuration = %v, expected >= %v", result.Timings.BuildGateDuration, gateDelay)
	}
}

// -----------------------------------------------------------------------------
// RunGateOnly Tests
// -----------------------------------------------------------------------------

func TestRunGateOnly(t *testing.T) {
	tests := []struct {
		name         string
		gateEnabled  bool
		nilGate      bool // if true, Runner.Gate is nil
		executeFn    func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error)
		wantErr      error
		assertResult func(t *testing.T, result Result, crt *testContainerRuntime)
	}{
		{
			name:        "enabled",
			gateEnabled: true,
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "checkstyle", Passed: true},
					},
				}, nil
			},
			assertResult: func(t *testing.T, result Result, crt *testContainerRuntime) {
				t.Helper()
				if crt.createCalled {
					t.Errorf("RunGateOnly() should not create containers")
				}
				if result.BuildGate == nil {
					t.Errorf("RunGateOnly() BuildGate metadata not captured")
				} else if len(result.BuildGate.StaticChecks) != 1 {
					t.Errorf("RunGateOnly() BuildGate.StaticChecks = %d, want 1", len(result.BuildGate.StaticChecks))
				}
				if result.Timings.BuildGateDuration == 0 {
					t.Errorf("RunGateOnly() BuildGateDuration not captured when gate enabled")
				}
				if result.Timings.TotalDuration == 0 {
					t.Errorf("RunGateOnly() TotalDuration not captured")
				}
				if result.ExitCode != 0 {
					t.Errorf("RunGateOnly() ExitCode = %d, want 0", result.ExitCode)
				}
			},
		},
		{
			name:        "disabled",
			gateEnabled: false,
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				t.Errorf("RunGateOnly() gate executor called when disabled")
				return nil, nil
			},
			assertResult: func(t *testing.T, result Result, crt *testContainerRuntime) {
				t.Helper()
				if crt.createCalled {
					t.Errorf("RunGateOnly() should not create containers")
				}
				if result.BuildGate != nil {
					t.Errorf("RunGateOnly() BuildGate metadata should be nil when disabled")
				}
				if result.ExitCode != 0 {
					t.Errorf("RunGateOnly() ExitCode = %d, want 0", result.ExitCode)
				}
			},
		},
		{
			name:        "gate failure",
			gateEnabled: true,
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: false},
					},
					LogsText: "[ERROR] BUILD FAILURE",
				}, nil
			},
			wantErr: ErrBuildGateFailed,
			assertResult: func(t *testing.T, result Result, crt *testContainerRuntime) {
				t.Helper()
				if result.BuildGate == nil {
					t.Errorf("RunGateOnly() BuildGate metadata should be populated on gate failure")
				} else if result.BuildGate.StaticChecks[0].Passed {
					t.Errorf("RunGateOnly() BuildGate.StaticChecks[0].Passed = true, want false")
				}
				if result.Timings.BuildGateDuration == 0 {
					t.Errorf("RunGateOnly() BuildGateDuration should be captured on gate failure")
				}
				if result.Timings.TotalDuration == 0 {
					t.Errorf("RunGateOnly() TotalDuration should be captured on gate failure")
				}
			},
		},
		{
			name:        "nil gate executor",
			gateEnabled: true,
			nilGate:     true,
			assertResult: func(t *testing.T, result Result, crt *testContainerRuntime) {
				t.Helper()
				if result.BuildGate != nil {
					t.Errorf("RunGateOnly() BuildGate should be nil when gate executor is nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			crt := &testContainerRuntime{}

			runner := &Runner{
				Workspace:  &testWorkspaceHydrator{},
				Containers: crt,
			}
			if !tt.nilGate {
				runner.Gate = &testGateExecutor{executeFn: tt.executeFn}
			}

			manifest := newGateTestManifest(tt.gateEnabled)
			if tt.name == "nil gate executor" {
				manifest.Image = "test:latest"
			}

			result, err := RunGateOnly(context.Background(), runner, newGateTestRequest(manifest))

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("RunGateOnly() expected error, got nil")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("RunGateOnly() error should wrap %v: %v", tt.wantErr, err)
				}
			} else if err != nil {
				t.Fatalf("RunGateOnly() unexpected error: %v", err)
			}

			tt.assertResult(t, result, crt)
		})
	}
}
