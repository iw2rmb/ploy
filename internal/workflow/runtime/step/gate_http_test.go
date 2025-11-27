package step

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// fakeBuildGateHTTPClient is a test double for BuildGateHTTPClient.
// It allows configuring responses for Validate and GetJob calls.
// For async polling tests, use GetJobResponses to queue multiple responses.
type fakeBuildGateHTTPClient struct {
	// ValidateResp is returned by Validate when ValidateErr is nil.
	ValidateResp *contracts.BuildGateValidateResponse
	// ValidateErr is returned by Validate if non-nil.
	ValidateErr error
	// ValidateCallCount tracks how many times Validate was called.
	ValidateCallCount int
	// LastValidateReq stores the last request passed to Validate.
	LastValidateReq contracts.BuildGateValidateRequest

	// GetJobResp is returned by GetJob when GetJobErr is nil and GetJobResponses is empty.
	GetJobResp *contracts.BuildGateJobStatusResponse
	// GetJobErr is returned by GetJob if non-nil and GetJobErrors is empty.
	GetJobErr error
	// GetJobCallCount tracks how many times GetJob was called.
	GetJobCallCount int
	// LastGetJobID stores the last jobID passed to GetJob.
	LastGetJobID string

	// GetJobResponses is a queue of responses for GetJob. If non-empty, each call
	// to GetJob pops from this queue. When empty, falls back to GetJobResp/GetJobErr.
	// Use this for async polling tests that need multiple poll iterations.
	GetJobResponses []*contracts.BuildGateJobStatusResponse
	// GetJobErrors is a queue of errors for GetJob, parallel to GetJobResponses.
	// If an entry is non-nil, it is returned as the error for that call.
	GetJobErrors []error
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

	// If we have queued responses, pop from the queue.
	if len(f.GetJobResponses) > 0 {
		resp := f.GetJobResponses[0]
		f.GetJobResponses = f.GetJobResponses[1:]

		var err error
		if len(f.GetJobErrors) > 0 {
			err = f.GetJobErrors[0]
			f.GetJobErrors = f.GetJobErrors[1:]
		}
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	// Fall back to single response mode.
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

// TestHTTPGateExecutor_Sync_Pending tests that pending status triggers async polling.
// Note: With B3 implementation, pending status now triggers GetJob polling.
func TestHTTPGateExecutor_Sync_Pending(t *testing.T) {
	t.Parallel()

	// Server returns pending status (async job queued).
	// GetJob then returns Completed immediately.
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-pending-456",
			Status: contracts.BuildGateJobStatusPending,
		},
		GetJobResponses: []*contracts.BuildGateJobStatusResponse{
			{JobID: "job-pending-456", Status: contracts.BuildGateJobStatusCompleted, Result: &contracts.BuildGateStageMetadata{LogDigest: "pending-done"}},
		},
	}
	executor := NewHTTPGateExecutor(fake)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(ctx, spec, "/workspace")

	// With B3, pending status triggers polling and should complete successfully.
	if err != nil {
		t.Fatalf("expected nil error after polling, got: %v", err)
	}
	if result == nil || result.LogDigest != "pending-done" {
		t.Errorf("expected result with LogDigest 'pending-done', got: %+v", result)
	}
	if fake.GetJobCallCount != 1 {
		t.Errorf("expected 1 GetJob call, got %d", fake.GetJobCallCount)
	}
}

// TestHTTPGateExecutor_Sync_Claimed tests that claimed status triggers async polling.
// Note: With B3 implementation, claimed status now triggers GetJob polling.
func TestHTTPGateExecutor_Sync_Claimed(t *testing.T) {
	t.Parallel()

	// Server returns claimed status (job picked up but not running).
	// GetJob then returns Completed.
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-claimed",
			Status: contracts.BuildGateJobStatusClaimed,
		},
		GetJobResponses: []*contracts.BuildGateJobStatusResponse{
			{JobID: "job-claimed", Status: contracts.BuildGateJobStatusCompleted, Result: &contracts.BuildGateStageMetadata{LogDigest: "claimed-done"}},
		},
	}
	executor := NewHTTPGateExecutor(fake)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(ctx, spec, "/workspace")

	// With B3, claimed status triggers polling and should complete successfully.
	if err != nil {
		t.Fatalf("expected nil error after polling, got: %v", err)
	}
	if result == nil || result.LogDigest != "claimed-done" {
		t.Errorf("expected result with LogDigest 'claimed-done', got: %+v", result)
	}
	if fake.GetJobCallCount != 1 {
		t.Errorf("expected 1 GetJob call, got %d", fake.GetJobCallCount)
	}
}

// TestHTTPGateExecutor_Sync_Running tests that running status triggers async polling.
// Note: With B3 implementation, running status now triggers GetJob polling.
func TestHTTPGateExecutor_Sync_Running(t *testing.T) {
	t.Parallel()

	// Server returns running status (job is executing).
	// GetJob then returns Completed.
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-running",
			Status: contracts.BuildGateJobStatusRunning,
		},
		GetJobResponses: []*contracts.BuildGateJobStatusResponse{
			{JobID: "job-running", Status: contracts.BuildGateJobStatusCompleted, Result: &contracts.BuildGateStageMetadata{LogDigest: "running-done"}},
		},
	}
	executor := NewHTTPGateExecutor(fake)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(ctx, spec, "/workspace")

	// With B3, running status triggers polling and should complete successfully.
	if err != nil {
		t.Fatalf("expected nil error after polling, got: %v", err)
	}
	if result == nil || result.LogDigest != "running-done" {
		t.Errorf("expected result with LogDigest 'running-done', got: %+v", result)
	}
	if fake.GetJobCallCount != 1 {
		t.Errorf("expected 1 GetJob call, got %d", fake.GetJobCallCount)
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

// =============================================================================
// Async Polling Tests (B3 - Phase B3 of ROADMAP.md)
// =============================================================================

// TestHTTPGateExecutor_Async_PendingThenCompleted tests polling from Pending to Completed.
// Simulates: Validate returns Pending, then GetJob returns Pending, Running, Completed.
func TestHTTPGateExecutor_Async_PendingThenCompleted(t *testing.T) {
	t.Parallel()

	// Expected result after job completes.
	expectedResult := &contracts.BuildGateStageMetadata{
		LogDigest: "async-result-123",
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{Language: "java", Tool: "maven", Passed: true},
		},
	}

	// Configure fake: Validate returns Pending, then GetJob returns sequence.
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-async-poll",
			Status: contracts.BuildGateJobStatusPending,
		},
		// GetJob response sequence: Pending -> Running -> Completed with result.
		GetJobResponses: []*contracts.BuildGateJobStatusResponse{
			{JobID: "job-async-poll", Status: contracts.BuildGateJobStatusPending},
			{JobID: "job-async-poll", Status: contracts.BuildGateJobStatusRunning},
			{JobID: "job-async-poll", Status: contracts.BuildGateJobStatusCompleted, Result: expectedResult},
		},
	}
	executor := NewHTTPGateExecutor(fake)

	// Use a context with timeout to limit test duration.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(ctx, spec, "/workspace")

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected result to be populated")
	}
	if result.LogDigest != "async-result-123" {
		t.Errorf("expected LogDigest 'async-result-123', got '%s'", result.LogDigest)
	}
	if len(result.StaticChecks) != 1 || !result.StaticChecks[0].Passed {
		t.Error("expected 1 passing static check")
	}
	// Verify GetJob was called 3 times (Pending -> Running -> Completed).
	if fake.GetJobCallCount != 3 {
		t.Errorf("expected 3 GetJob calls, got %d", fake.GetJobCallCount)
	}
	if fake.LastGetJobID != "job-async-poll" {
		t.Errorf("expected job ID 'job-async-poll', got '%s'", fake.LastGetJobID)
	}
}

// TestHTTPGateExecutor_Async_ClaimedThenCompleted tests polling from Claimed status.
func TestHTTPGateExecutor_Async_ClaimedThenCompleted(t *testing.T) {
	t.Parallel()

	expectedResult := &contracts.BuildGateStageMetadata{LogDigest: "claimed-result"}

	// Validate returns Claimed, GetJob returns Running then Completed.
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-claimed-async",
			Status: contracts.BuildGateJobStatusClaimed,
		},
		GetJobResponses: []*contracts.BuildGateJobStatusResponse{
			{JobID: "job-claimed-async", Status: contracts.BuildGateJobStatusRunning},
			{JobID: "job-claimed-async", Status: contracts.BuildGateJobStatusCompleted, Result: expectedResult},
		},
	}
	executor := NewHTTPGateExecutor(fake)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(ctx, spec, "/workspace")

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if result == nil || result.LogDigest != "claimed-result" {
		t.Errorf("expected result with LogDigest 'claimed-result', got: %+v", result)
	}
	if fake.GetJobCallCount != 2 {
		t.Errorf("expected 2 GetJob calls, got %d", fake.GetJobCallCount)
	}
}

// TestHTTPGateExecutor_Async_RunningThenCompleted tests polling from Running status.
func TestHTTPGateExecutor_Async_RunningThenCompleted(t *testing.T) {
	t.Parallel()

	expectedResult := &contracts.BuildGateStageMetadata{LogDigest: "running-result"}

	// Validate returns Running, GetJob returns Completed immediately.
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-running-async",
			Status: contracts.BuildGateJobStatusRunning,
		},
		GetJobResponses: []*contracts.BuildGateJobStatusResponse{
			{JobID: "job-running-async", Status: contracts.BuildGateJobStatusCompleted, Result: expectedResult},
		},
	}
	executor := NewHTTPGateExecutor(fake)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(ctx, spec, "/workspace")

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if result == nil || result.LogDigest != "running-result" {
		t.Errorf("expected result with LogDigest 'running-result', got: %+v", result)
	}
	if fake.GetJobCallCount != 1 {
		t.Errorf("expected 1 GetJob call, got %d", fake.GetJobCallCount)
	}
}

// TestHTTPGateExecutor_Async_PendingThenFailed tests polling until job fails.
func TestHTTPGateExecutor_Async_PendingThenFailed(t *testing.T) {
	t.Parallel()

	// Validate returns Pending, GetJob returns Pending then Failed.
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-will-fail",
			Status: contracts.BuildGateJobStatusPending,
		},
		GetJobResponses: []*contracts.BuildGateJobStatusResponse{
			{JobID: "job-will-fail", Status: contracts.BuildGateJobStatusPending},
			{JobID: "job-will-fail", Status: contracts.BuildGateJobStatusFailed, Error: "compilation error"},
		},
	}
	executor := NewHTTPGateExecutor(fake)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(ctx, spec, "/workspace")

	// Should return error for failed job.
	if err == nil {
		t.Fatal("expected error for failed job")
	}
	if !strings.Contains(err.Error(), "build gate job failed") {
		t.Errorf("expected 'build gate job failed' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "job-will-fail") {
		t.Errorf("expected error to contain job ID, got: %v", err)
	}
	if !strings.Contains(err.Error(), "compilation error") {
		t.Errorf("expected error to contain error message, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for failed job, got: %+v", result)
	}
	// Should have polled twice before failing.
	if fake.GetJobCallCount != 2 {
		t.Errorf("expected 2 GetJob calls, got %d", fake.GetJobCallCount)
	}
}

// TestHTTPGateExecutor_Async_FailedWithoutErrorMessage tests failed job without error details.
func TestHTTPGateExecutor_Async_FailedWithoutErrorMessage(t *testing.T) {
	t.Parallel()

	// Validate returns Pending, GetJob returns Failed without error message.
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-fail-no-msg",
			Status: contracts.BuildGateJobStatusPending,
		},
		GetJobResponses: []*contracts.BuildGateJobStatusResponse{
			{JobID: "job-fail-no-msg", Status: contracts.BuildGateJobStatusFailed, Error: ""},
		},
	}
	executor := NewHTTPGateExecutor(fake)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec := &contracts.StepGateSpec{Enabled: true}
	_, err := executor.Execute(ctx, spec, "/workspace")

	if err == nil {
		t.Fatal("expected error for failed job")
	}
	if !strings.Contains(err.Error(), "build gate job failed") {
		t.Errorf("expected 'build gate job failed' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "job-fail-no-msg") {
		t.Errorf("expected error to contain job ID, got: %v", err)
	}
}

// TestHTTPGateExecutor_Async_ContextTimeout tests that polling respects context timeout.
func TestHTTPGateExecutor_Async_ContextTimeout(t *testing.T) {
	t.Parallel()

	// Validate returns Pending. GetJob always returns Pending (never completes).
	// Configure GetJobResp for fallback after queue is empty.
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-timeout",
			Status: contracts.BuildGateJobStatusPending,
		},
		// Fallback response: always Pending.
		GetJobResp: &contracts.BuildGateJobStatusResponse{
			JobID:  "job-timeout",
			Status: contracts.BuildGateJobStatusPending,
		},
	}
	executor := NewHTTPGateExecutor(fake)

	// Very short timeout to trigger timeout quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(ctx, spec, "/workspace")

	// Should return timeout error.
	if err == nil {
		t.Fatal("expected error for timeout")
	}
	if !strings.Contains(err.Error(), "timed out") && !strings.Contains(err.Error(), "deadline exceeded") {
		t.Errorf("expected timeout error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "job-timeout") {
		t.Errorf("expected error to contain job ID, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for timeout, got: %+v", result)
	}
	// Should have polled at least once.
	if fake.GetJobCallCount < 1 {
		t.Errorf("expected at least 1 GetJob call, got %d", fake.GetJobCallCount)
	}
}

// TestHTTPGateExecutor_Async_ContextCancelledDuringPolling tests cancellation mid-poll.
func TestHTTPGateExecutor_Async_ContextCancelledDuringPolling(t *testing.T) {
	t.Parallel()

	// Validate returns Pending.
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-cancelled",
			Status: contracts.BuildGateJobStatusPending,
		},
		// GetJob returns Pending, then we cancel before next poll.
		GetJobResponses: []*contracts.BuildGateJobStatusResponse{
			{JobID: "job-cancelled", Status: contracts.BuildGateJobStatusPending},
		},
		// Fallback: Pending forever.
		GetJobResp: &contracts.BuildGateJobStatusResponse{
			JobID:  "job-cancelled",
			Status: contracts.BuildGateJobStatusPending,
		},
	}
	executor := NewHTTPGateExecutor(fake)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after brief delay (enough for 1 poll).
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(ctx, spec, "/workspace")

	// Should return cancellation error.
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "canceled") {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for cancelled, got: %+v", result)
	}
}

// TestHTTPGateExecutor_Async_CompletedNoResult tests async completion with nil result.
func TestHTTPGateExecutor_Async_CompletedNoResult(t *testing.T) {
	t.Parallel()

	// Validate returns Pending, GetJob returns Completed but with nil Result.
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-empty-result",
			Status: contracts.BuildGateJobStatusPending,
		},
		GetJobResponses: []*contracts.BuildGateJobStatusResponse{
			{JobID: "job-empty-result", Status: contracts.BuildGateJobStatusCompleted, Result: nil},
		},
	}
	executor := NewHTTPGateExecutor(fake)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(ctx, spec, "/workspace")

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	// Should return empty metadata, not nil.
	if result == nil {
		t.Fatal("expected non-nil result (empty metadata)")
	}
}

// TestHTTPGateExecutor_Async_UnknownStatusDuringPolling tests handling of unknown status.
func TestHTTPGateExecutor_Async_UnknownStatusDuringPolling(t *testing.T) {
	t.Parallel()

	// Validate returns Pending, GetJob returns unknown status.
	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-unknown-async",
			Status: contracts.BuildGateJobStatusPending,
		},
		GetJobResponses: []*contracts.BuildGateJobStatusResponse{
			{JobID: "job-unknown-async", Status: contracts.BuildGateJobStatus("weird_status")},
		},
	}
	executor := NewHTTPGateExecutor(fake)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(ctx, spec, "/workspace")

	if err == nil {
		t.Fatal("expected error for unknown status")
	}
	if !strings.Contains(err.Error(), "unexpected job status") {
		t.Errorf("expected 'unexpected job status' error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got: %+v", result)
	}
}

// TestHTTPGateExecutor_Async_ResultPreservedAfterPolling tests full result preservation.
func TestHTTPGateExecutor_Async_ResultPreservedAfterPolling(t *testing.T) {
	t.Parallel()

	// Rich result with multiple fields.
	expectedResult := &contracts.BuildGateStageMetadata{
		LogDigest: "rich-async-result",
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{
				Language: "go",
				Tool:     "go build",
				Passed:   false,
				Failures: []contracts.BuildGateStaticCheckFailure{
					{File: "main.go", Line: 10, Column: 5, Severity: "error", Message: "undefined"},
				},
			},
		},
		LogFindings: []contracts.BuildGateLogFinding{
			{Code: "BUILD_FAIL", Severity: "error", Message: "build failed", Evidence: "exit status 1"},
		},
	}

	fake := &fakeBuildGateHTTPClient{
		ValidateResp: &contracts.BuildGateValidateResponse{
			JobID:  "job-rich-async",
			Status: contracts.BuildGateJobStatusPending,
		},
		GetJobResponses: []*contracts.BuildGateJobStatusResponse{
			{JobID: "job-rich-async", Status: contracts.BuildGateJobStatusRunning},
			{JobID: "job-rich-async", Status: contracts.BuildGateJobStatusCompleted, Result: expectedResult},
		},
	}
	executor := NewHTTPGateExecutor(fake)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec := &contracts.StepGateSpec{Enabled: true}
	result, err := executor.Execute(ctx, spec, "/workspace")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}

	// Verify all fields preserved.
	if result.LogDigest != "rich-async-result" {
		t.Errorf("expected LogDigest 'rich-async-result', got '%s'", result.LogDigest)
	}
	if len(result.StaticChecks) != 1 {
		t.Fatalf("expected 1 static check, got %d", len(result.StaticChecks))
	}
	if result.StaticChecks[0].Passed {
		t.Error("expected static check to fail")
	}
	if len(result.StaticChecks[0].Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(result.StaticChecks[0].Failures))
	}
	if result.StaticChecks[0].Failures[0].Line != 10 {
		t.Errorf("expected line 10, got %d", result.StaticChecks[0].Failures[0].Line)
	}
	if len(result.LogFindings) != 1 {
		t.Fatalf("expected 1 log finding, got %d", len(result.LogFindings))
	}
	if result.LogFindings[0].Code != "BUILD_FAIL" {
		t.Errorf("expected code 'BUILD_FAIL', got '%s'", result.LogFindings[0].Code)
	}
}
