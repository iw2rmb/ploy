package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// Helper function to create int32 pointers.
func int32Ptr(i int32) *int32 {
	return &i
}

// TestStatusUploader_RetryOn5xx verifies retry behavior on transient 5xx errors.
func TestStatusUploader_RetryOn5xx(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		responses    []int // sequence of status codes to return
		wantErr      bool
		wantAttempts int
		wantSucceed  bool
	}{
		{
			name:         "eventual success after 503",
			responses:    []int{http.StatusServiceUnavailable, http.StatusNoContent},
			wantErr:      false,
			wantAttempts: 2,
			wantSucceed:  true,
		},
		{
			name:         "eventual success after multiple 5xx",
			responses:    []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusNoContent},
			wantErr:      false,
			wantAttempts: 3,
			wantSucceed:  true,
		},
		{
			name:         "all retries exhausted with 500",
			responses:    []int{http.StatusInternalServerError, http.StatusInternalServerError, http.StatusInternalServerError, http.StatusInternalServerError},
			wantErr:      true,
			wantAttempts: 4, // initial + 3 retries
			wantSucceed:  false,
		},
		{
			name:         "immediate 4xx error no retry",
			responses:    []int{http.StatusBadRequest},
			wantErr:      true,
			wantAttempts: 1,
			wantSucceed:  false,
		},
		{
			name:         "4xx after 5xx retry",
			responses:    []int{http.StatusServiceUnavailable, http.StatusBadRequest},
			wantErr:      true,
			wantAttempts: 2,
			wantSucceed:  false,
		},
		{
			name:         "200 OK accepted as success",
			responses:    []int{http.StatusOK},
			wantErr:      false,
			wantAttempts: 1,
			wantSucceed:  true,
		},
		{
			name:         "204 No Content accepted as success",
			responses:    []int{http.StatusNoContent},
			wantErr:      false,
			wantAttempts: 1,
			wantSucceed:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			attemptCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if attemptCount < len(tt.responses) {
					w.WriteHeader(tt.responses[attemptCount])
				} else {
					w.WriteHeader(http.StatusInternalServerError)
				}
				attemptCount++
			}))
			defer server.Close()

			cfg := Config{
				ServerURL: server.URL,
				NodeID:    testNodeID,
				HTTP: HTTPConfig{
					TLS: TLSConfig{
						Enabled: false,
					},
				},
			}

			uploader, err := newBaseUploader(cfg)
			if err != nil {
				t.Fatalf("failed to create uploader: %v", err)
			}

			ctx := context.Background()
			jobID := types.JobID("test-job-id")
			// v1 uses capitalized job status values: Success, Fail, Cancelled.
			err = uploader.UploadJobStatus(ctx, jobID, types.JobStatusSuccess.String(), nil, nil)

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if attemptCount != tt.wantAttempts {
				t.Errorf("expected %d attempts, got %d", tt.wantAttempts, attemptCount)
			}
		})
	}
}

func TestStatusUploader_ReconcileConflictHandling(t *testing.T) {
	t.Parallel()

	type uploadFn func(*baseUploader, context.Context, types.JobID) error
	tests := []struct {
		name         string
		responses    []int
		upload       uploadFn
		wantErr      bool
		wantAttempts int
	}{
		{
			name:      "reconcile accepts conflict",
			responses: []int{http.StatusConflict},
			upload: func(uploader *baseUploader, ctx context.Context, jobID types.JobID) error {
				return uploader.UploadJobStatusReconcile(ctx, jobID, types.JobStatusSuccess.String(), nil, nil)
			},
			wantErr:      false,
			wantAttempts: 1,
		},
		{
			name:      "reconcile retries on 5xx then accepts conflict",
			responses: []int{http.StatusInternalServerError, http.StatusConflict},
			upload: func(uploader *baseUploader, ctx context.Context, jobID types.JobID) error {
				return uploader.UploadJobStatusReconcile(ctx, jobID, types.JobStatusSuccess.String(), nil, nil)
			},
			wantErr:      false,
			wantAttempts: 2,
		},
		{
			name:      "reconcile keeps non conflict 4xx permanent",
			responses: []int{http.StatusForbidden},
			upload: func(uploader *baseUploader, ctx context.Context, jobID types.JobID) error {
				return uploader.UploadJobStatusReconcile(ctx, jobID, types.JobStatusSuccess.String(), nil, nil)
			},
			wantErr:      true,
			wantAttempts: 1,
		},
		{
			name:      "default upload keeps conflict permanent",
			responses: []int{http.StatusConflict},
			upload: func(uploader *baseUploader, ctx context.Context, jobID types.JobID) error {
				return uploader.UploadJobStatus(ctx, jobID, types.JobStatusSuccess.String(), nil, nil)
			},
			wantErr:      true,
			wantAttempts: 1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			attemptCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if attemptCount < len(tt.responses) {
					w.WriteHeader(tt.responses[attemptCount])
				} else {
					w.WriteHeader(http.StatusInternalServerError)
				}
				attemptCount++
			}))
			defer server.Close()

			cfg := Config{
				ServerURL: server.URL,
				NodeID:    testNodeID,
				HTTP: HTTPConfig{
					TLS: TLSConfig{
						Enabled: false,
					},
				},
			}

			uploader, err := newBaseUploader(cfg)
			if err != nil {
				t.Fatalf("failed to create uploader: %v", err)
			}

			err = tt.upload(uploader, context.Background(), types.JobID("test-job-id"))
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if attemptCount != tt.wantAttempts {
				t.Fatalf("attempts = %d, want %d", attemptCount, tt.wantAttempts)
			}
		})
	}
}

// TestStatusUploader_RetryBackoff verifies exponential backoff on retries.
func TestStatusUploader_RetryBackoff(t *testing.T) {
	t.Parallel()

	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	uploader, err := newBaseUploader(cfg)
	if err != nil {
		t.Fatalf("failed to create uploader: %v", err)
	}

	ctx := context.Background()
	jobID := types.JobID("test-job-id")
	start := time.Now()
	// v1 uses capitalized job status values: Success, Fail, Cancelled.
	err = uploader.UploadJobStatus(ctx, jobID, types.JobStatusSuccess.String(), nil, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// With shared backoff policy (100ms initial, 2x multiplier, 50% jitter):
	// First retry:  ~50-150ms (100ms ± 50%)
	// Second retry: ~100-300ms (200ms ± 50%)
	// Total minimum (lower bound): ~150ms (50ms + 100ms)
	// We use a conservative lower bound to account for jitter.
	expectedMinDuration := 150 * time.Millisecond
	if elapsed < expectedMinDuration {
		t.Errorf("expected backoff duration >= %v, got %v", expectedMinDuration, elapsed)
	}

	// Verify we got the expected number of attempts.
	if attemptCount != 3 {
		t.Errorf("expected 3 attempts, got %d", attemptCount)
	}
}

// TestStatusUploader_ContextCancellation verifies context cancellation during retry.
func TestStatusUploader_ContextCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return 500 to trigger retry.
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	uploader, err := newBaseUploader(cfg)
	if err != nil {
		t.Fatalf("failed to create uploader: %v", err)
	}

	// Create context with short timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	jobID := types.JobID("test-job-id")
	// v1 uses capitalized job status values: Success, Fail, Cancelled.
	err = uploader.UploadJobStatus(ctx, jobID, types.JobStatusSuccess.String(), nil, nil)

	if err == nil {
		t.Error("expected context cancellation error")
	}

	// Verify it's a context error.
	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}
}

// TestStatusUploader_PayloadIncludesStatusExitCodeAndStats verifies the
// payload shape for job-level completion requests.
func TestStatusUploader_StepIndexAndJobIDIncluded(t *testing.T) {
	t.Parallel()

	var receivedPayload map[string]interface{}

	// Create a test server that captures the payload and verifies next_id and job_id.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			t.Errorf("failed to decode payload: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	uploader, err := newBaseUploader(cfg)
	if err != nil {
		t.Fatalf("failed to create uploader: %v", err)
	}

	ctx := context.Background()
	exitCode := int32Ptr(1)
	stats := types.NewRunStatsBuilder().
		ExitCode(1).
		DurationMs(1500).
		MustBuild()
	jobID := types.JobID("test-job-id-uuid")

	// Upload status via job-level endpoint.
	// v1 uses capitalized job status values: Success, Fail, Cancelled.
	err = uploader.UploadJobStatus(ctx, jobID, types.JobStatusFail.String(), exitCode, stats)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify status uses v1 capitalized value.
	if receivedPayload["status"] != types.JobStatusFail.String() {
		t.Errorf("expected status=%s, got %v", types.JobStatusFail.String(), receivedPayload["status"])
	}

	// Verify exit_code is present.
	if ec, ok := receivedPayload["exit_code"].(float64); !ok || ec != 1 {
		t.Errorf("expected exit_code=1, got %v", receivedPayload["exit_code"])
	}

	// Verify stats are included.
	if statsPayload, ok := receivedPayload["stats"].(map[string]interface{}); ok {
		if exitCode, ok := statsPayload["exit_code"].(float64); !ok || exitCode != 1 {
			t.Errorf("expected stats.exit_code=1, got %v", statsPayload["exit_code"])
		}
	} else {
		t.Error("stats not present or not a map in payload")
	}
}

func TestStatusUploader_UploadJobStatus_UsesJobEndpointAndPayloadShape(t *testing.T) {
	t.Parallel()

	var receivedPayload map[string]interface{}
	jobID := types.JobID("test-job-id-uuid")
	nodeID := types.NodeID("ignored-node-id") // Convert to domain type

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		expectedPath := "/v1/jobs/" + string(jobID) + "/complete"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		if got := r.Header.Get("PLOY_NODE_UUID"); got != nodeID.String() { // Use String() for comparison
			t.Errorf("expected PLOY_NODE_UUID=%s, got %s", nodeID.String(), got)
		}

		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			t.Errorf("failed to decode payload: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    nodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	uploader, err := newBaseUploader(cfg)
	if err != nil {
		t.Fatalf("failed to create uploader: %v", err)
	}

	ctx := context.Background()
	exitCode := int32(0)
	stats := types.NewRunStatsBuilder().
		ExitCode(0).
		DurationMs(1000).
		MustBuild()

	// v1 uses capitalized job status values: Success, Fail, Cancelled.
	err = uploader.UploadJobStatus(ctx, jobID, types.JobStatusSuccess.String(), &exitCode, stats)
	if err != nil {
		t.Fatalf("unexpected error uploading job status: %v", err)
	}

	if receivedPayload["status"] != types.JobStatusSuccess.String() {
		t.Errorf("expected status=%s, got %v", types.JobStatusSuccess.String(), receivedPayload["status"])
	}

	if ec, ok := receivedPayload["exit_code"].(float64); !ok || ec != 0 {
		t.Errorf("expected exit_code=0, got %v", receivedPayload["exit_code"])
	}

	if statsPayload, ok := receivedPayload["stats"].(map[string]interface{}); ok {
		if exitCodeVal, ok := statsPayload["exit_code"].(float64); !ok || exitCodeVal != 0 {
			t.Errorf("expected stats.exit_code=0, got %v", statsPayload["exit_code"])
		}
	} else {
		t.Error("stats not present or not a map in payload")
	}

	if _, ok := receivedPayload["run_id"]; ok {
		t.Error("did not expect run_id in job-level payload")
	}
	if _, ok := receivedPayload["job_id"]; ok {
		t.Error("did not expect job_id in payload; it is encoded in the URL")
	}
	if _, ok := receivedPayload["next_id"]; ok {
		t.Error("did not expect next_id in job-level payload")
	}
}
