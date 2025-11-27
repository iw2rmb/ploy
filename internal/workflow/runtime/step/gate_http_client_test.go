package step

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestBuildGateHTTPClient_Validate_Success tests successful validation with immediate result.
func TestBuildGateHTTPClient_Validate_Success(t *testing.T) {
	t.Parallel()

	// Setup mock server that returns a completed validation response.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path.
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1/buildgate/validate") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Verify content type header.
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		// Decode and verify request body.
		var req contracts.BuildGateValidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req.RepoURL != "https://github.com/test/repo" {
			t.Errorf("unexpected repo_url: %s", req.RepoURL)
		}
		if req.Ref != "main" {
			t.Errorf("unexpected ref: %s", req.Ref)
		}

		// Return successful response with immediate result.
		resp := contracts.BuildGateValidateResponse{
			JobID:  "job-123",
			Status: contracts.BuildGateJobStatusCompleted,
			Result: &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Language: "java", Tool: "maven", Passed: true},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client with test server URL.
	client, err := NewBuildGateHTTPClient(BuildGateHTTPClientConfig{
		ServerURL: server.URL,
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBuildGateHTTPClient() error: %v", err)
	}

	// Execute validation request.
	resp, err := client.Validate(context.Background(), contracts.BuildGateValidateRequest{
		RepoURL: "https://github.com/test/repo",
		Ref:     "main",
		Profile: "java-maven",
	})
	if err != nil {
		t.Fatalf("Validate() error: %v", err)
	}

	// Verify response.
	if resp.JobID != "job-123" {
		t.Errorf("expected job_id 'job-123', got '%s'", resp.JobID)
	}
	if resp.Status != contracts.BuildGateJobStatusCompleted {
		t.Errorf("expected status 'completed', got '%s'", resp.Status)
	}
	if resp.Result == nil {
		t.Fatal("expected Result to be populated")
	}
	if len(resp.Result.StaticChecks) != 1 {
		t.Fatalf("expected 1 static check, got %d", len(resp.Result.StaticChecks))
	}
	if !resp.Result.StaticChecks[0].Passed {
		t.Error("expected static check to pass")
	}
}

// TestBuildGateHTTPClient_Validate_Pending tests validation returning a pending job.
func TestBuildGateHTTPClient_Validate_Pending(t *testing.T) {
	t.Parallel()

	// Setup mock server that returns a pending job.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := contracts.BuildGateValidateResponse{
			JobID:  "job-456",
			Status: contracts.BuildGateJobStatusPending,
			// No Result for pending jobs.
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewBuildGateHTTPClient(BuildGateHTTPClientConfig{
		ServerURL: server.URL,
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBuildGateHTTPClient() error: %v", err)
	}

	resp, err := client.Validate(context.Background(), contracts.BuildGateValidateRequest{
		RepoURL: "https://github.com/test/repo",
		Ref:     "main",
	})
	if err != nil {
		t.Fatalf("Validate() error: %v", err)
	}

	if resp.JobID != "job-456" {
		t.Errorf("expected job_id 'job-456', got '%s'", resp.JobID)
	}
	if resp.Status != contracts.BuildGateJobStatusPending {
		t.Errorf("expected status 'pending', got '%s'", resp.Status)
	}
	if resp.Result != nil {
		t.Error("expected Result to be nil for pending job")
	}
}

// TestBuildGateHTTPClient_Validate_ClientError tests 4xx responses are permanent errors.
func TestBuildGateHTTPClient_Validate_ClientError(t *testing.T) {
	t.Parallel()

	// Setup mock server that returns 400 Bad Request.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid repo_url"}`))
	}))
	defer server.Close()

	client, err := NewBuildGateHTTPClient(BuildGateHTTPClientConfig{
		ServerURL: server.URL,
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBuildGateHTTPClient() error: %v", err)
	}

	_, err = client.Validate(context.Background(), contracts.BuildGateValidateRequest{
		RepoURL: "https://github.com/test/repo",
		Ref:     "main",
	})

	// Should return an error containing the status code.
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("expected error to contain '400', got: %v", err)
	}
}

// TestBuildGateHTTPClient_Validate_InvalidRequest tests request validation.
func TestBuildGateHTTPClient_Validate_InvalidRequest(t *testing.T) {
	t.Parallel()

	client, err := NewBuildGateHTTPClient(BuildGateHTTPClientConfig{
		ServerURL: "http://localhost:9999",
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBuildGateHTTPClient() error: %v", err)
	}

	// Missing required fields should fail validation before HTTP call.
	_, err = client.Validate(context.Background(), contracts.BuildGateValidateRequest{
		// Missing RepoURL and Ref.
	})

	if err == nil {
		t.Fatal("expected validation error for missing fields")
	}
	if !strings.Contains(err.Error(), "repo_url") {
		t.Errorf("expected error about repo_url, got: %v", err)
	}
}

// TestBuildGateHTTPClient_GetJob_Completed tests retrieving a completed job.
func TestBuildGateHTTPClient_GetJob_Completed(t *testing.T) {
	t.Parallel()

	// Setup mock server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method.
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		// Verify path contains job ID.
		if !strings.Contains(r.URL.Path, "/v1/buildgate/jobs/job-789") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := contracts.BuildGateJobStatusResponse{
			JobID:      "job-789",
			Status:     contracts.BuildGateJobStatusCompleted,
			CreatedAt:  "2024-01-15T10:00:00Z",
			StartedAt:  stringPtr("2024-01-15T10:00:05Z"),
			FinishedAt: stringPtr("2024-01-15T10:01:00Z"),
			Result: &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{
					{Language: "java", Tool: "gradle", Passed: true},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewBuildGateHTTPClient(BuildGateHTTPClientConfig{
		ServerURL: server.URL,
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBuildGateHTTPClient() error: %v", err)
	}

	resp, err := client.GetJob(context.Background(), "job-789")
	if err != nil {
		t.Fatalf("GetJob() error: %v", err)
	}

	if resp.JobID != "job-789" {
		t.Errorf("expected job_id 'job-789', got '%s'", resp.JobID)
	}
	if resp.Status != contracts.BuildGateJobStatusCompleted {
		t.Errorf("expected status 'completed', got '%s'", resp.Status)
	}
	if resp.Result == nil {
		t.Fatal("expected Result to be populated")
	}
	if resp.FinishedAt == nil {
		t.Error("expected FinishedAt to be populated for completed job")
	}
}

// TestBuildGateHTTPClient_GetJob_Failed tests retrieving a failed job.
func TestBuildGateHTTPClient_GetJob_Failed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := contracts.BuildGateJobStatusResponse{
			JobID:      "job-fail",
			Status:     contracts.BuildGateJobStatusFailed,
			Error:      "build compilation failed",
			CreatedAt:  "2024-01-15T10:00:00Z",
			StartedAt:  stringPtr("2024-01-15T10:00:05Z"),
			FinishedAt: stringPtr("2024-01-15T10:00:30Z"),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewBuildGateHTTPClient(BuildGateHTTPClientConfig{
		ServerURL: server.URL,
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBuildGateHTTPClient() error: %v", err)
	}

	resp, err := client.GetJob(context.Background(), "job-fail")
	if err != nil {
		t.Fatalf("GetJob() error: %v", err)
	}

	if resp.Status != contracts.BuildGateJobStatusFailed {
		t.Errorf("expected status 'failed', got '%s'", resp.Status)
	}
	if resp.Error != "build compilation failed" {
		t.Errorf("expected error message 'build compilation failed', got '%s'", resp.Error)
	}
}

// TestBuildGateHTTPClient_GetJob_NotFound tests 404 response for missing job.
func TestBuildGateHTTPClient_GetJob_NotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"job not found"}`))
	}))
	defer server.Close()

	client, err := NewBuildGateHTTPClient(BuildGateHTTPClientConfig{
		ServerURL: server.URL,
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBuildGateHTTPClient() error: %v", err)
	}

	_, err = client.GetJob(context.Background(), "nonexistent-job")

	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to contain '404', got: %v", err)
	}
}

// TestBuildGateHTTPClient_GetJob_EmptyJobID tests validation of empty job ID.
func TestBuildGateHTTPClient_GetJob_EmptyJobID(t *testing.T) {
	t.Parallel()

	client, err := NewBuildGateHTTPClient(BuildGateHTTPClientConfig{
		ServerURL: "http://localhost:9999",
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBuildGateHTTPClient() error: %v", err)
	}

	_, err = client.GetJob(context.Background(), "")

	if err == nil {
		t.Fatal("expected error for empty job ID")
	}
	if !strings.Contains(err.Error(), "job ID is required") {
		t.Errorf("expected error about job ID, got: %v", err)
	}
}

// TestBuildGateHTTPClient_BearerToken tests authorization header is set.
func TestBuildGateHTTPClient_BearerToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Authorization header.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token-123" {
			t.Errorf("expected 'Bearer test-token-123', got '%s'", auth)
		}

		resp := contracts.BuildGateValidateResponse{
			JobID:  "job-auth",
			Status: contracts.BuildGateJobStatusCompleted,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewBuildGateHTTPClient(BuildGateHTTPClientConfig{
		ServerURL: server.URL,
		APIToken:  "test-token-123",
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBuildGateHTTPClient() error: %v", err)
	}

	_, err = client.Validate(context.Background(), contracts.BuildGateValidateRequest{
		RepoURL: "https://github.com/test/repo",
		Ref:     "main",
	})
	if err != nil {
		t.Fatalf("Validate() error: %v", err)
	}
}

// TestBuildGateHTTPClient_ContextCancellation tests context cancellation handling.
func TestBuildGateHTTPClient_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Server that delays response (will be cancelled before completion).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			// Request was cancelled.
			return
		case <-time.After(10 * time.Second):
			// Should not reach here in test.
			resp := contracts.BuildGateValidateResponse{JobID: "delayed"}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client, err := NewBuildGateHTTPClient(BuildGateHTTPClientConfig{
		ServerURL: server.URL,
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBuildGateHTTPClient() error: %v", err)
	}

	// Create cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err = client.Validate(ctx, contracts.BuildGateValidateRequest{
		RepoURL: "https://github.com/test/repo",
		Ref:     "main",
	})

	// Should return context error.
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestBuildGateHTTPClientConfigFromEnv tests environment variable loading.
func TestBuildGateHTTPClientConfigFromEnv(t *testing.T) {
	// Note: This test modifies environment variables, so it cannot run in parallel
	// with other tests that depend on these variables.

	// Set environment variable (t.Setenv handles cleanup automatically).
	t.Setenv("PLOY_SERVER_URL", "https://api.test.ploy.io")

	cfg, err := BuildGateHTTPClientConfigFromEnv()
	if err != nil {
		t.Fatalf("BuildGateHTTPClientConfigFromEnv() error: %v", err)
	}

	if cfg.ServerURL != "https://api.test.ploy.io" {
		t.Errorf("expected ServerURL 'https://api.test.ploy.io', got '%s'", cfg.ServerURL)
	}
}

// TestBuildGateHTTPClientConfigFromEnv_MissingServerURL tests error for missing URL.
func TestBuildGateHTTPClientConfigFromEnv_MissingServerURL(t *testing.T) {
	// Ensure PLOY_SERVER_URL is unset for this test.
	t.Setenv("PLOY_SERVER_URL", "")

	_, err := BuildGateHTTPClientConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for missing PLOY_SERVER_URL")
	}
	if !strings.Contains(err.Error(), "PLOY_SERVER_URL") {
		t.Errorf("expected error about PLOY_SERVER_URL, got: %v", err)
	}
}

// stringPtr is a helper to create *string from a string literal.
func stringPtr(s string) *string {
	return &s
}
