package step

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Mock implementations for testing.

type mockWorkspaceHydrator struct {
	hydrateFn func(ctx context.Context, manifest contracts.StepManifest, workspace string) error
}

func (m *mockWorkspaceHydrator) Hydrate(ctx context.Context, manifest contracts.StepManifest, workspace string) error {
	if m.hydrateFn != nil {
		return m.hydrateFn(ctx, manifest, workspace)
	}
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
		StaticChecks: []contracts.BuildGateStaticCheckReport{},
		LogFindings:  []contracts.BuildGateLogFinding{},
	}, nil
}

func TestRunner_Run_BasicExecution(t *testing.T) {
	runner := Runner{
		Workspace: &mockWorkspaceHydrator{},
		Gate:      &mockGateExecutor{},
	}

	manifest := contracts.StepManifest{
		ID:    "test-step",
		Name:  "Test Step",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{
				Name:      "source",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadOnly,
				Hydration: &contracts.StepInputHydration{
					Repo: &contracts.RepoMaterialization{
						URL: "https://github.com/example/repo.git",
					},
				},
			},
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

	if result.ExitCode != 0 {
		t.Errorf("Run() ExitCode = %d, want 0", result.ExitCode)
	}

	// Verify timing fields are populated
	if result.Timings.TotalDuration == 0 {
		t.Errorf("Run() TotalDuration not captured")
	}
	if result.Timings.HydrationDuration < 0 {
		t.Errorf("Run() HydrationDuration invalid: %v", result.Timings.HydrationDuration)
	}
	if result.Timings.ExecutionDuration < 0 {
		t.Errorf("Run() ExecutionDuration invalid: %v", result.Timings.ExecutionDuration)
	}
	if result.Timings.BuildGateDuration < 0 {
		t.Errorf("Run() BuildGateDuration invalid: %v", result.Timings.BuildGateDuration)
	}
}

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
		ID:    "test-step",
		Name:  "Test Step",
		Image: "maven:3-eclipse-temurin-17",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: "bafytest123",
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
		ID:    "test-step",
		Name:  "Test Step",
		Image: "maven:3-eclipse-temurin-17",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: "bafytest123",
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

func TestRunner_Run_FallbackToShiftSpec(t *testing.T) {
	gateExecuted := false
	var capturedProfile string

	runner := Runner{
		Workspace: &mockWorkspaceHydrator{},
		Gate: &mockGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				gateExecuted = true
				capturedProfile = spec.Profile
				return &contracts.BuildGateStageMetadata{}, nil
			},
		},
	}

	// Use deprecated Shift spec instead of Gate
	manifest := contracts.StepManifest{
		ID:    "test-step",
		Name:  "Test Step",
		Image: "maven:3-eclipse-temurin-17",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: "bafytest123",
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

func TestRunner_Run_GatePrecedenceOverShift(t *testing.T) {
	var capturedProfile string
	runner := Runner{
		Workspace: &mockWorkspaceHydrator{},
		Gate: &mockGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				capturedProfile = spec.Profile
				return &contracts.BuildGateStageMetadata{}, nil
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    "test-step",
		Name:  "Test Step",
		Image: "test:latest",
		Inputs: []contracts.StepInput{{
			Name:        "source",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadOnly,
			SnapshotCID: "bafytest123",
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

func TestRunner_Run_HydrationFailure(t *testing.T) {
	expectedErr := errors.New("hydration failed")
	runner := Runner{
		Workspace: &mockWorkspaceHydrator{
			hydrateFn: func(ctx context.Context, manifest contracts.StepManifest, workspace string) error {
				return expectedErr
			},
		},
		Gate: &mockGateExecutor{},
	}

	manifest := contracts.StepManifest{
		ID:    "test-step",
		Name:  "Test Step",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: "bafytest123",
			},
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
		t.Errorf("Run() error chain doesn't include hydration error: %v", err)
	}
}

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
		ID:    "test-step",
		Name:  "Test Step",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: "bafytest123",
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

func TestRunner_Run_TimingCapture(t *testing.T) {
	hydrationDelay := 10 * time.Millisecond
	gateDelay := 5 * time.Millisecond

	runner := Runner{
		Workspace: &mockWorkspaceHydrator{
			hydrateFn: func(ctx context.Context, manifest contracts.StepManifest, workspace string) error {
				time.Sleep(hydrationDelay)
				return nil
			},
		},
		Gate: &mockGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				time.Sleep(gateDelay)
				return &contracts.BuildGateStageMetadata{}, nil
			},
		},
	}

	manifest := contracts.StepManifest{
		ID:    "test-step",
		Name:  "Test Step",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: "bafytest123",
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

	// Verify timing measurements are reasonable
	if result.Timings.HydrationDuration < hydrationDelay {
		t.Errorf("Run() HydrationDuration = %v, expected >= %v", result.Timings.HydrationDuration, hydrationDelay)
	}

	if result.Timings.BuildGateDuration < gateDelay {
		t.Errorf("Run() BuildGateDuration = %v, expected >= %v", result.Timings.BuildGateDuration, gateDelay)
	}

	// Total duration should be sum of all stages (with some tolerance)
	minExpected := result.Timings.HydrationDuration +
		result.Timings.ExecutionDuration +
		result.Timings.BuildGateDuration +
		result.Timings.DiffDuration +
		result.Timings.PublishDuration

	if result.Timings.TotalDuration < minExpected {
		t.Errorf("Run() TotalDuration = %v, expected >= %v", result.Timings.TotalDuration, minExpected)
	}
}

func TestRunner_Run_NilComponents(t *testing.T) {
	// Runner with nil workspace and gate should still work
	runner := Runner{
		Workspace: nil,
		Gate:      nil,
	}

	manifest := contracts.StepManifest{
		ID:    "test-step",
		Name:  "Test Step",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: "bafytest123",
			},
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
	}

	result, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run() unexpected error with nil components: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Run() ExitCode = %d, want 0", result.ExitCode)
	}

	// Timing should still be captured even with nil components
	if result.Timings.TotalDuration == 0 {
		t.Errorf("Run() TotalDuration not captured with nil components")
	}
}
