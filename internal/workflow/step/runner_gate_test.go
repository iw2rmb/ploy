package step

import (
	"context"
	"errors"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestRunner_Run_GateEnabledDisabled verifies that runner.Run correctly invokes
// the gate executor when Gate.Enabled=true and skips it when Enabled=false.
func TestRunner_Run_GateEnabledDisabled(t *testing.T) {
	tests := []struct {
		name        string
		gateEnabled bool
		wantGateRun bool
		wantMeta    bool
		wantTiming  bool
	}{
		{"enabled/runs", true, true, true, true},
		{"disabled/skipped", false, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			manifest := newGateTestManifest(tt.gateEnabled)
			result, err := runner.Run(context.Background(), newGateTestRequest(manifest))
			if err != nil {
				t.Fatalf("Run() unexpected error: %v", err)
			}

			if gateExecuted != tt.wantGateRun {
				t.Errorf("Run() gate executed = %v, want %v", gateExecuted, tt.wantGateRun)
			}
			if (result.BuildGate != nil) != tt.wantMeta {
				t.Errorf("Run() BuildGate populated = %v, want %v", result.BuildGate != nil, tt.wantMeta)
			}
			if tt.wantMeta && len(result.BuildGate.StaticChecks) != 1 {
				t.Errorf("Run() BuildGate.StaticChecks = %d, want 1", len(result.BuildGate.StaticChecks))
			}
			if tt.wantTiming && result.Timings.BuildGateDuration == 0 {
				t.Errorf("Run() BuildGateDuration not captured when gate enabled")
			}
		})
	}
}

// TestRunner_Run_GateFailureScenarios covers the two distinct gate failure modes:
// executor returning an error, and executor returning metadata with Passed=false.
func TestRunner_Run_GateFailureScenarios(t *testing.T) {
	tests := []struct {
		name          string
		executeFn     func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error)
		wantErrIs     error
		assertResult  func(t *testing.T, result Result)
	}{
		{
			name: "executor error/propagated",
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				return nil, errors.New("gate execution failed")
			},
			assertResult: func(t *testing.T, result Result) {
				t.Helper()
				// Error path exits before gate metadata is set.
			},
		},
		{
			name: "failed checks/ErrBuildGateFailed",
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "maven", Passed: false},
					},
					LogsText: "[ERROR] BUILD FAILURE\n[ERROR] Failed to compile",
				}, nil
			},
			wantErrIs: ErrBuildGateFailed,
			assertResult: func(t *testing.T, result Result) {
				t.Helper()
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
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := Runner{
				Workspace: &testWorkspaceHydrator{},
				Gate:      &testGateExecutor{executeFn: tt.executeFn},
				Containers: nil,
			}

			manifest := newGateTestManifest(true)
			manifest.Options = map[string]any{}
			result, err := runner.Run(context.Background(), newGateTestRequest(manifest))

			if err == nil {
				t.Fatalf("Run() expected error, got nil")
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Errorf("Run() error should wrap %v, got: %v", tt.wantErrIs, err)
			}

			tt.assertResult(t, result)
		})
	}
}

// TestRunGateStage_NilMetadata verifies that when the gate executor returns
// (nil, nil), the gate stage treats it as a failure without panicking.
func TestRunGateStage_NilMetadata(t *testing.T) {
	runner := Runner{
		Workspace: &testWorkspaceHydrator{},
		Gate: &testGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				return nil, nil
			},
		},
	}

	manifest := newGateTestManifest(true)
	req := newGateTestRequest(manifest)
	meta, dur, err := runGateStage(context.Background(), &runner, req, "test gate")

	if meta != nil {
		t.Errorf("expected nil metadata, got %+v", meta)
	}
	_ = dur
	if err == nil {
		t.Fatal("expected error for nil gate metadata (gate not passed)")
	}
	if !errors.Is(err, ErrBuildGateFailed) {
		t.Errorf("expected ErrBuildGateFailed, got %v", err)
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
