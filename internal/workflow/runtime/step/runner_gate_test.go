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

// -----------------------------------------------------------------------------
// RunGateOnly Tests
// -----------------------------------------------------------------------------

// TestRunGateOnly_Enabled verifies that RunGateOnly invokes the gate executor
// when the gate is enabled, populates BuildGate metadata, and does NOT invoke
// any container runtime methods.
func TestRunGateOnly_Enabled(t *testing.T) {
	gateExecuted := false
	containerCreated := false

	runner := &Runner{
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
		// Use a mock container runtime to detect if it's ever called.
		Containers: &mockContainerRuntimeForGateOnly{
			createFn: func() { containerCreated = true },
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

	result, err := RunGateOnly(context.Background(), runner, req)
	if err != nil {
		t.Fatalf("RunGateOnly() unexpected error: %v", err)
	}

	// Verify gate was executed.
	if !gateExecuted {
		t.Errorf("RunGateOnly() gate executor not called when enabled")
	}

	// Verify container runtime was NOT invoked.
	if containerCreated {
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
	containerCreated := false

	runner := &Runner{
		Workspace: &mockWorkspaceHydrator{},
		Gate: &mockGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				gateExecuted = true
				return nil, nil
			},
		},
		Containers: &mockContainerRuntimeForGateOnly{
			createFn: func() { containerCreated = true },
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

	result, err := RunGateOnly(context.Background(), runner, req)
	if err != nil {
		t.Fatalf("RunGateOnly() unexpected error: %v", err)
	}

	// Verify gate was NOT executed.
	if gateExecuted {
		t.Errorf("RunGateOnly() gate executor called when disabled")
	}

	// Verify container runtime was NOT invoked.
	if containerCreated {
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
		Workspace: &mockWorkspaceHydrator{},
		Gate: &mockGateExecutor{
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
			Profile: "java",
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
		Workspace: &mockWorkspaceHydrator{},
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
			Profile: "java",
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

// mockContainerRuntimeForGateOnly is a minimal mock to detect if container
// methods are invoked by RunGateOnly (they should not be).
type mockContainerRuntimeForGateOnly struct {
	createFn func()
}

func (m *mockContainerRuntimeForGateOnly) Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
	if m.createFn != nil {
		m.createFn()
	}
	return ContainerHandle{ID: "mock-container"}, nil
}

func (m *mockContainerRuntimeForGateOnly) Start(ctx context.Context, handle ContainerHandle) error {
	return nil
}

func (m *mockContainerRuntimeForGateOnly) Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error) {
	return ContainerResult{ExitCode: 0}, nil
}

func (m *mockContainerRuntimeForGateOnly) Logs(ctx context.Context, handle ContainerHandle) ([]byte, error) {
	return nil, nil
}

func (m *mockContainerRuntimeForGateOnly) Remove(ctx context.Context, handle ContainerHandle) error {
	return nil
}

// =============================================================================
// HTTP Mode Integration Tests (Phase D - ROADMAP.md line 124)
//
// These tests verify that when Runner.Gate is set to an HTTPGateExecutor,
// the gate executor correctly routes calls to the Build Gate HTTP API
// (POST /v1/buildgate/validate) and that results are wired into step results.
// =============================================================================

// httpModeGateClient is a test double for BuildGateHTTPClient used in HTTP mode tests.
// It tracks call counts and stores captured requests for assertions.
type httpModeGateClient struct {
	// validateCallCount tracks how many times Validate was called.
	validateCallCount int
	// lastValidateReq stores the last request passed to Validate.
	lastValidateReq contracts.BuildGateValidateRequest
	// validateResp is returned by Validate.
	validateResp *contracts.BuildGateValidateResponse
	// validateErr is returned by Validate if non-nil.
	validateErr error
}

func (c *httpModeGateClient) Validate(ctx context.Context, req contracts.BuildGateValidateRequest) (*contracts.BuildGateValidateResponse, error) {
	c.validateCallCount++
	c.lastValidateReq = req
	if c.validateErr != nil {
		return nil, c.validateErr
	}
	return c.validateResp, nil
}

func (c *httpModeGateClient) GetJob(ctx context.Context, jobID string) (*contracts.BuildGateJobStatusResponse, error) {
	// Not used in sync mode tests.
	return nil, nil
}

// TestRunner_Run_WithHTTPGateExecutor_MakesValidateCall verifies that when the Runner
// is configured with an HTTPGateExecutor (simulating PLOY_BUILDGATE_MODE=remote-http),
// the gate executor makes at least one HTTP call to /v1/buildgate/validate.
//
// This test fulfills ROADMAP.md Phase D requirement:
// "Assert that in HTTP mode, the gate executor makes at least one HTTP call to /v1/buildgate/validate."
func TestRunner_Run_WithHTTPGateExecutor_MakesValidateCall(t *testing.T) {
	t.Parallel()

	// Configure mock HTTP client to return sync-completed result.
	httpClient := &httpModeGateClient{
		validateResp: &contracts.BuildGateValidateResponse{
			JobID:  "http-gate-job-123",
			Status: contracts.BuildGateJobStatusCompleted,
			Result: &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Language: "java", Tool: "maven", Passed: true},
				},
				LogDigest: "http-mode-digest",
			},
		},
	}

	// Create HTTPGateExecutor using the mock client.
	// This simulates the configuration when PLOY_BUILDGATE_MODE=remote-http.
	httpGateExecutor := NewHTTPGateExecutor(httpClient)

	// Build Runner with HTTP-based gate executor (no container runtime needed for gate-only).
	runner := Runner{
		Workspace: &mockWorkspaceHydrator{},
		Gate:      httpGateExecutor,
	}

	// Build manifest with gate enabled and repo metadata populated (as buildManifestFromRequest does).
	manifest := contracts.StepManifest{
		ID:    types.StepID("http-gate-test"),
		Name:  "HTTP Gate Test",
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
			Profile: "java-maven",
			// Repo metadata populated by buildManifestFromRequest (C1).
			RepoURL: "https://github.com/example/repo.git",
			Ref:     "main",
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/http-gate-workspace",
	}

	// Execute the runner.
	result, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	// Assert: HTTP client's Validate was called at least once.
	if httpClient.validateCallCount < 1 {
		t.Errorf("HTTP gate: Validate() was not called, expected at least 1 call")
	}

	// Assert: Request contained expected profile (passed from StepGateSpec).
	if httpClient.lastValidateReq.Profile != "java-maven" {
		t.Errorf("HTTP gate: Validate() profile = %q, want %q", httpClient.lastValidateReq.Profile, "java-maven")
	}

	// Assert: Request contained repo URL (wired by C1 from StepGateSpec).
	if httpClient.lastValidateReq.RepoURL != "https://github.com/example/repo.git" {
		t.Errorf("HTTP gate: Validate() RepoURL = %q, want %q", httpClient.lastValidateReq.RepoURL, "https://github.com/example/repo.git")
	}

	// Assert: Request contained ref (wired by C1 from StepGateSpec).
	if httpClient.lastValidateReq.Ref != "main" {
		t.Errorf("HTTP gate: Validate() Ref = %q, want %q", httpClient.lastValidateReq.Ref, "main")
	}

	// Assert: Result has BuildGate metadata wired from HTTP response.
	if result.BuildGate == nil {
		t.Fatalf("Run() BuildGate metadata not captured in result")
	}
	if result.BuildGate.LogDigest != "http-mode-digest" {
		t.Errorf("HTTP gate: BuildGate.LogDigest = %q, want %q", result.BuildGate.LogDigest, "http-mode-digest")
	}
	if len(result.BuildGate.StaticChecks) != 1 {
		t.Fatalf("HTTP gate: BuildGate.StaticChecks = %d, want 1", len(result.BuildGate.StaticChecks))
	}
	if !result.BuildGate.StaticChecks[0].Passed {
		t.Errorf("HTTP gate: StaticChecks[0].Passed = false, want true")
	}

	// Assert: BuildGate timing was captured.
	if result.Timings.BuildGateDuration == 0 {
		t.Errorf("HTTP gate: BuildGateDuration not captured")
	}
}

// TestRunner_Run_WithHTTPGateExecutor_MetadataWiredToResult verifies that the
// BuildGateStageMetadata returned by the HTTP gate executor is correctly wired
// into the step result, maintaining consistency with docker-based gate execution.
//
// This test fulfills ROADMAP.md Phase D requirement:
// "Assert that the returned BuildGateStageMetadata is wired into step results as before."
func TestRunner_Run_WithHTTPGateExecutor_MetadataWiredToResult(t *testing.T) {
	t.Parallel()

	// Configure mock HTTP client to return detailed metadata (simulating real gate output).
	expectedMetadata := &contracts.BuildGateStageMetadata{
		LogDigest: "detailed-digest-456",
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{
				Language: "java",
				Tool:     "maven",
				Passed:   false, // Simulate build failure to verify failure metadata is preserved.
				Failures: []contracts.BuildGateStaticCheckFailure{
					{
						File:     "src/main/java/App.java",
						Line:     42,
						Column:   10,
						Severity: "error",
						Message:  "cannot find symbol",
					},
				},
			},
		},
		LogFindings: []contracts.BuildGateLogFinding{
			{
				Code:     "COMPILE_ERROR",
				Severity: "error",
				Message:  "Compilation failure",
				Evidence: "[ERROR] BUILD FAILURE",
			},
		},
		LogsText: "[INFO] Building...\n[ERROR] BUILD FAILURE",
	}

	httpClient := &httpModeGateClient{
		validateResp: &contracts.BuildGateValidateResponse{
			JobID:  "metadata-test-job",
			Status: contracts.BuildGateJobStatusCompleted,
			Result: expectedMetadata,
		},
	}

	httpGateExecutor := NewHTTPGateExecutor(httpClient)

	runner := Runner{
		Workspace: &mockWorkspaceHydrator{},
		Gate:      httpGateExecutor,
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("metadata-test"),
		Name:  "Metadata Wire Test",
		Image: "maven:3-eclipse-temurin-17",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: types.CID("bafytest456"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
			Profile: "java-maven",
			RepoURL: "https://gitlab.com/org/project.git",
			Ref:     "feature-branch",
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/metadata-test-workspace",
	}

	// Execute the runner. Since the gate returns failed static checks with
	// Passed=false, the runner should return ErrBuildGateFailed but still
	// populate the BuildGate metadata in the result.
	result, err := runner.Run(context.Background(), req)

	// Expect error for failed gate (Passed=false in StaticChecks).
	if err == nil {
		t.Fatalf("Run() expected ErrBuildGateFailed for failed gate, got nil")
	}
	if !errors.Is(err, ErrBuildGateFailed) {
		t.Errorf("Run() error should be ErrBuildGateFailed, got: %v", err)
	}

	// Assert: BuildGate metadata is fully populated in result.
	if result.BuildGate == nil {
		t.Fatalf("Run() BuildGate metadata not captured on gate failure")
	}

	// Verify all metadata fields are wired correctly.
	if result.BuildGate.LogDigest != "detailed-digest-456" {
		t.Errorf("LogDigest = %q, want %q", result.BuildGate.LogDigest, "detailed-digest-456")
	}

	if len(result.BuildGate.StaticChecks) != 1 {
		t.Fatalf("StaticChecks count = %d, want 1", len(result.BuildGate.StaticChecks))
	}
	sc := result.BuildGate.StaticChecks[0]
	if sc.Passed {
		t.Errorf("StaticChecks[0].Passed = true, want false")
	}
	if sc.Language != "java" {
		t.Errorf("StaticChecks[0].Language = %q, want %q", sc.Language, "java")
	}
	if sc.Tool != "maven" {
		t.Errorf("StaticChecks[0].Tool = %q, want %q", sc.Tool, "maven")
	}
	if len(sc.Failures) != 1 {
		t.Fatalf("StaticChecks[0].Failures count = %d, want 1", len(sc.Failures))
	}
	if sc.Failures[0].Line != 42 {
		t.Errorf("StaticChecks[0].Failures[0].Line = %d, want 42", sc.Failures[0].Line)
	}

	if len(result.BuildGate.LogFindings) != 1 {
		t.Fatalf("LogFindings count = %d, want 1", len(result.BuildGate.LogFindings))
	}
	lf := result.BuildGate.LogFindings[0]
	if lf.Code != "COMPILE_ERROR" {
		t.Errorf("LogFindings[0].Code = %q, want %q", lf.Code, "COMPILE_ERROR")
	}

	if result.BuildGate.LogsText != "[INFO] Building...\n[ERROR] BUILD FAILURE" {
		t.Errorf("LogsText not preserved correctly")
	}

	// Assert: Timing was captured even on gate failure.
	if result.Timings.BuildGateDuration == 0 {
		t.Errorf("BuildGateDuration not captured on gate failure")
	}
}

// TestRunGateOnly_WithHTTPGateExecutor verifies that RunGateOnly also works
// correctly with an HTTPGateExecutor, routing calls to the HTTP API.
func TestRunGateOnly_WithHTTPGateExecutor(t *testing.T) {
	t.Parallel()

	httpClient := &httpModeGateClient{
		validateResp: &contracts.BuildGateValidateResponse{
			JobID:  "gate-only-http-job",
			Status: contracts.BuildGateJobStatusCompleted,
			Result: &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Tool: "go build", Passed: true},
				},
				LogDigest: "gate-only-digest",
			},
		},
	}

	httpGateExecutor := NewHTTPGateExecutor(httpClient)

	runner := &Runner{
		Workspace: &mockWorkspaceHydrator{},
		Gate:      httpGateExecutor,
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("gate-only-http-test"),
		Name:  "Gate Only HTTP Test",
		Image: "golang:1.23",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: types.CID("bafygo123"),
			},
		},
		Gate: &contracts.StepGateSpec{
			Enabled: true,
			Profile: "go",
			RepoURL: "https://github.com/org/go-project.git",
			Ref:     "v1.0.0",
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/gate-only-http-workspace",
	}

	// Execute gate-only (no container execution).
	result, err := RunGateOnly(context.Background(), runner, req)
	if err != nil {
		t.Fatalf("RunGateOnly() unexpected error: %v", err)
	}

	// Assert: HTTP Validate was called.
	if httpClient.validateCallCount < 1 {
		t.Errorf("RunGateOnly: Validate() was not called")
	}

	// Assert: Repo metadata was passed to the HTTP request.
	if httpClient.lastValidateReq.RepoURL != "https://github.com/org/go-project.git" {
		t.Errorf("RunGateOnly: RepoURL = %q, want %q", httpClient.lastValidateReq.RepoURL, "https://github.com/org/go-project.git")
	}
	if httpClient.lastValidateReq.Ref != "v1.0.0" {
		t.Errorf("RunGateOnly: Ref = %q, want %q", httpClient.lastValidateReq.Ref, "v1.0.0")
	}

	// Assert: BuildGate metadata is populated.
	if result.BuildGate == nil {
		t.Fatalf("RunGateOnly: BuildGate metadata not captured")
	}
	if result.BuildGate.LogDigest != "gate-only-digest" {
		t.Errorf("RunGateOnly: LogDigest = %q, want %q", result.BuildGate.LogDigest, "gate-only-digest")
	}
}
