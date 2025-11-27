package step

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// fakeBuildGateHTTPClient is a test double for BuildGateHTTPClient.
// It allows configuring responses for Validate and GetJob calls.
type fakeBuildGateHTTPClient struct {
	// ValidateResp is returned by Validate when ValidateErr is nil.
	ValidateResp *contracts.BuildGateValidateResponse
	// ValidateErr is returned by Validate if non-nil.
	ValidateErr error
	// ValidateCallCount tracks how many times Validate was called.
	ValidateCallCount int
	// LastValidateReq stores the last request passed to Validate.
	LastValidateReq contracts.BuildGateValidateRequest

	// GetJobResp is returned by GetJob when GetJobErr is nil.
	GetJobResp *contracts.BuildGateJobStatusResponse
	// GetJobErr is returned by GetJob if non-nil.
	GetJobErr error
	// GetJobCallCount tracks how many times GetJob was called.
	GetJobCallCount int
	// LastGetJobID stores the last jobID passed to GetJob.
	LastGetJobID string
}

func (f *fakeBuildGateHTTPClient) Validate(ctx context.Context, req contracts.BuildGateValidateRequest) (*contracts.BuildGateValidateResponse, error) {
	f.ValidateCallCount++
	f.LastValidateReq = req
	if f.ValidateErr != nil {
		return nil, f.ValidateErr
	}
	return f.ValidateResp, nil
}

func (f *fakeBuildGateHTTPClient) GetJob(ctx context.Context, jobID string) (*contracts.BuildGateJobStatusResponse, error) {
	f.GetJobCallCount++
	f.LastGetJobID = jobID
	if f.GetJobErr != nil {
		return nil, f.GetJobErr
	}
	return f.GetJobResp, nil
}

// TestHTTPGateExecutor_Sync_NilSpec tests that Execute returns nil,nil for nil spec.
func TestHTTPGateExecutor_Sync_NilSpec(t *testing.T) {
	t.Parallel()

	fake := &fakeBuildGateHTTPClient{}
	executor := NewHTTPGateExecutor(fake)

	// Execute with nil spec should return nil,nil without calling client.
	result, err := executor.Execute(context.Background(), nil, "/workspace")

	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got: %+v", result)
	}
	if fake.ValidateCallCount != 0 {
		t.Errorf("expected Validate to not be called, got %d calls", fake.ValidateCallCount)
	}
}

// TestHTTPGateExecutor_Sync_DisabledSpec tests that Execute returns nil,nil for disabled spec.
func TestHTTPGateExecutor_Sync_DisabledSpec(t *testing.T) {
	t.Parallel()

	fake := &fakeBuildGateHTTPClient{}
	executor := NewHTTPGateExecutor(fake)

	// Execute with Enabled=false should return nil,nil without calling client.
	spec := &contracts.StepGateSpec{
		Enabled: false,
		Profile: "java-maven",
	}
	result, err := executor.Execute(context.Background(), spec, "/workspace")

	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got: %+v", result)
	}
	if fake.ValidateCallCount != 0 {
		t.Errorf("expected Validate to not be called, got %d calls", fake.ValidateCallCount)
	}
}

// TestHTTPGateExecutor_Sync_Completed tests synchronous completion with immediate result.
func TestHTTPGateExecutor_Sync_Completed(t *testing.T) {
	t.Parallel()

	// Configure fake client to return completed status with result.
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-sync-123",
			Status: contracts.BuildGateJobStatusCompleted,
			Result: &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Language: "java", Tool: "maven", Passed: true},
				},
			},
		},
	}
	executor := NewHTTPGateExecutor(fake)

	spec := &contracts.StepGateSpec{
		Enabled: true,
		Profile: "java-maven",
	}
	result, err := executor.Execute(context.Background(), spec, "/workspace")

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected result to be populated")
	}
	if len(result.StaticChecks) != 1 {
		t.Fatalf("expected 1 static check, got %d", len(result.StaticChecks))
	}
	if !result.StaticChecks[0].Passed {
		t.Error("expected static check to pass")
	}
	if fake.ValidateCallCount != 1 {
		t.Errorf("expected 1 Validate call, got %d", fake.ValidateCallCount)
	}
	// Verify profile was passed to the request.
	if fake.LastValidateReq.Profile != "java-maven" {
		t.Errorf("expected profile 'java-maven', got '%s'", fake.LastValidateReq.Profile)
	}
}

// TestHTTPGateExecutor_Sync_CompletedNoResult tests sync completion with no result.
func TestHTTPGateExecutor_Sync_CompletedNoResult(t *testing.T) {
	t.Parallel()

	// Server returns completed status but no result (edge case).
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-empty",
			Status: contracts.BuildGateJobStatusCompleted,
			Result: nil, // No result even though completed.
		},
	}
	executor := NewHTTPGateExecutor(fake)

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(context.Background(), spec, "/workspace")

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	// Should return empty metadata, not nil.
	if result == nil {
		t.Fatal("expected non-nil result (empty metadata)")
	}
}

// TestHTTPGateExecutor_Sync_Pending tests that pending status returns an error.
func TestHTTPGateExecutor_Sync_Pending(t *testing.T) {
	t.Parallel()

	// Server returns pending status (async job queued).
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-pending-456",
			Status: contracts.BuildGateJobStatusPending,
		},
	}
	executor := NewHTTPGateExecutor(fake)

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(context.Background(), spec, "/workspace")

	// Should return error since async polling is not yet supported.
	if err == nil {
		t.Fatal("expected error for pending status")
	}
	if !strings.Contains(err.Error(), "async jobs not supported") {
		t.Errorf("expected 'async jobs not supported' error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for pending status, got: %+v", result)
	}
}

// TestHTTPGateExecutor_Sync_Claimed tests that claimed status returns an error.
func TestHTTPGateExecutor_Sync_Claimed(t *testing.T) {
	t.Parallel()

	// Server returns claimed status (job picked up but not running).
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-claimed",
			Status: contracts.BuildGateJobStatusClaimed,
		},
	}
	executor := NewHTTPGateExecutor(fake)

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(context.Background(), spec, "/workspace")

	// Should return error since async polling is not yet supported.
	if err == nil {
		t.Fatal("expected error for claimed status")
	}
	if !strings.Contains(err.Error(), "async jobs not supported") {
		t.Errorf("expected 'async jobs not supported' error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for claimed status, got: %+v", result)
	}
}

// TestHTTPGateExecutor_Sync_Running tests that running status returns an error.
func TestHTTPGateExecutor_Sync_Running(t *testing.T) {
	t.Parallel()

	// Server returns running status (job is executing).
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-running",
			Status: contracts.BuildGateJobStatusRunning,
		},
	}
	executor := NewHTTPGateExecutor(fake)

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(context.Background(), spec, "/workspace")

	// Should return error since async polling is not yet supported.
	if err == nil {
		t.Fatal("expected error for running status")
	}
	if !strings.Contains(err.Error(), "async jobs not supported") {
		t.Errorf("expected 'async jobs not supported' error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for running status, got: %+v", result)
	}
}

// TestHTTPGateExecutor_Sync_Failed tests that failed job status returns an error.
func TestHTTPGateExecutor_Sync_Failed(t *testing.T) {
	t.Parallel()

	// Server returns failed status (job execution failed).
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-fail-789",
			Status: contracts.BuildGateJobStatusFailed,
		},
	}
	executor := NewHTTPGateExecutor(fake)

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(context.Background(), spec, "/workspace")

	// Should return error for failed job.
	if err == nil {
		t.Fatal("expected error for failed status")
	}
	if !strings.Contains(err.Error(), "build gate job failed") {
		t.Errorf("expected 'build gate job failed' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "job-fail-789") {
		t.Errorf("expected error to contain job ID, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for failed status, got: %+v", result)
	}
}

// TestHTTPGateExecutor_Sync_ValidateError tests client validation error.
func TestHTTPGateExecutor_Sync_ValidateError(t *testing.T) {
	t.Parallel()

	// Client returns error from Validate call.
	fake := &fakeBuildGateHTTPClient{
		ValidateErr: errors.New("network error: connection refused"),
	}
	executor := NewHTTPGateExecutor(fake)

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(context.Background(), spec, "/workspace")

	if err == nil {
		t.Fatal("expected error from Validate failure")
	}
	if !strings.Contains(err.Error(), "validate via http") {
		t.Errorf("expected 'validate via http' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result on error, got: %+v", result)
	}
}

// TestHTTPGateExecutor_Sync_ContextCancelled tests context cancellation before Validate.
func TestHTTPGateExecutor_Sync_ContextCancelled(t *testing.T) {
	t.Parallel()

	fake := &fakeBuildGateHTTPClient{}
	executor := NewHTTPGateExecutor(fake)

	// Create already-cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(ctx, spec, "/workspace")

	// Should return context error without calling Validate.
	if err == nil {
		t.Fatal("expected context error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got: %+v", result)
	}
	if fake.ValidateCallCount != 0 {
		t.Errorf("expected Validate to not be called, got %d calls", fake.ValidateCallCount)
	}
}

// TestHTTPGateExecutor_NewHTTPGateExecutor_NilClient tests constructor with nil client.
func TestHTTPGateExecutor_NewHTTPGateExecutor_NilClient(t *testing.T) {
	t.Parallel()

	executor := NewHTTPGateExecutor(nil)

	// Should return nil executor for nil client.
	if executor != nil {
		t.Errorf("expected nil executor for nil client, got: %+v", executor)
	}
}

// TestHTTPGateExecutor_Sync_ProfilePropagation tests that spec.Profile is propagated.
func TestHTTPGateExecutor_Sync_ProfilePropagation(t *testing.T) {
	t.Parallel()

	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-profile",
			Status: contracts.BuildGateJobStatusCompleted,
			Result: &contracts.BuildGateStageMetadata{},
		},
	}
	executor := NewHTTPGateExecutor(fake)

	// Test with various profile values.
	testCases := []struct {
		name            string
		profile         string
		expectedProfile string
	}{
		{"java-maven", "java-maven", "java-maven"},
		{"java-gradle", "java-gradle", "java-gradle"},
		{"auto", "auto", "auto"},
		{"empty", "", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fake.ValidateCallCount = 0 // Reset counter.
			spec := &contracts.StepGateSpec{
				Enabled: true,
				Profile: tc.profile,
			}
			_, err := executor.Execute(context.Background(), spec, "/workspace")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fake.LastValidateReq.Profile != tc.expectedProfile {
				t.Errorf("expected profile '%s', got '%s'", tc.expectedProfile, fake.LastValidateReq.Profile)
			}
		})
	}
}

// TestHTTPGateExecutor_Sync_UnknownStatus tests handling of unknown job status.
func TestHTTPGateExecutor_Sync_UnknownStatus(t *testing.T) {
	t.Parallel()

	// Server returns unexpected status value.
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-unknown",
			Status: contracts.BuildGateJobStatus("unexpected_status"),
		},
	}
	executor := NewHTTPGateExecutor(fake)

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(context.Background(), spec, "/workspace")

	// Should return error for unknown status.
	if err == nil {
		t.Fatal("expected error for unknown status")
	}
	if !strings.Contains(err.Error(), "unexpected job status") {
		t.Errorf("expected 'unexpected job status' error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for unknown status, got: %+v", result)
	}
}

// TestHTTPGateExecutor_Sync_ResultPreserved tests that full result is preserved.
func TestHTTPGateExecutor_Sync_ResultPreserved(t *testing.T) {
	t.Parallel()

	// Create rich result with multiple fields.
	expectedResult := &contracts.BuildGateStageMetadata{
		LogDigest: "abc123",
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{
				Language: "java",
				Tool:     "maven",
				Passed:   false,
				Failures: []contracts.BuildGateStaticCheckFailure{
					{
						File:     "src/Main.java",
						Line:     42,
						Column:   10,
						Severity: "error",
						Message:  "syntax error",
					},
				},
			},
		},
		LogFindings: []contracts.BuildGateLogFinding{
			{
				Code:     "COMPILE_ERROR",
				Severity: "error",
				Message:  "compilation failed",
				Evidence: "[ERROR] BUILD FAILURE",
			},
		},
	}

	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-rich",
			Status: contracts.BuildGateJobStatusCompleted,
			Result: expectedResult,
		},
	}
	executor := NewHTTPGateExecutor(fake)

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(context.Background(), spec, "/workspace")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result to be populated")
	}

	// Verify result fields are preserved.
	if result.LogDigest != "abc123" {
		t.Errorf("expected LogDigest 'abc123', got '%s'", result.LogDigest)
	}
	if len(result.StaticChecks) != 1 {
		t.Fatalf("expected 1 static check, got %d", len(result.StaticChecks))
	}
	if result.StaticChecks[0].Passed {
		t.Error("expected static check to not pass")
	}
	if len(result.StaticChecks[0].Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(result.StaticChecks[0].Failures))
	}
	if result.StaticChecks[0].Failures[0].Line != 42 {
		t.Errorf("expected line 42, got %d", result.StaticChecks[0].Failures[0].Line)
	}
	if len(result.LogFindings) != 1 {
		t.Fatalf("expected 1 log finding, got %d", len(result.LogFindings))
	}
	if result.LogFindings[0].Code != "COMPILE_ERROR" {
		t.Errorf("expected code 'COMPILE_ERROR', got '%s'", result.LogFindings[0].Code)
	}
}
