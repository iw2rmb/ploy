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

// int32Ptr returns a pointer to the given int32.
func int32Ptr(i int32) *int32 { return &i }

// statusUpload is the shape of the two upload paths under test.
type statusUpload func(*baseUploader, context.Context, types.JobID) error

func uploadJobStatusSuccess(u *baseUploader, ctx context.Context, jobID types.JobID) error {
	return u.UploadJobStatus(ctx, jobID, types.JobStatusSuccess.String(), nil, nil)
}

func uploadJobStatusReconcileSuccess(u *baseUploader, ctx context.Context, jobID types.JobID) error {
	return u.UploadJobStatusReconcile(ctx, jobID, types.JobStatusSuccess.String(), nil, nil)
}

// TestStatusUploader_RetryAndConflict exercises retry, backoff and conflict
// handling for the two status upload paths against a scripted-response server.
func TestStatusUploader_RetryAndConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		responses    []int
		upload       statusUpload
		wantErr      bool
		wantAttempts int
	}{
		// --- UploadJobStatus: retry on 5xx ---
		{
			name:         "eventual success after 503",
			responses:    []int{http.StatusServiceUnavailable, http.StatusNoContent},
			upload:       uploadJobStatusSuccess,
			wantAttempts: 2,
		},
		{
			name:         "eventual success after multiple 5xx",
			responses:    []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusNoContent},
			upload:       uploadJobStatusSuccess,
			wantAttempts: 3,
		},
		{
			name:         "all retries exhausted with 500",
			responses:    []int{http.StatusInternalServerError, http.StatusInternalServerError, http.StatusInternalServerError, http.StatusInternalServerError},
			upload:       uploadJobStatusSuccess,
			wantErr:      true,
			wantAttempts: 4,
		},
		{
			name:         "immediate 4xx error no retry",
			responses:    []int{http.StatusBadRequest},
			upload:       uploadJobStatusSuccess,
			wantErr:      true,
			wantAttempts: 1,
		},
		{
			name:         "4xx after 5xx retry",
			responses:    []int{http.StatusServiceUnavailable, http.StatusBadRequest},
			upload:       uploadJobStatusSuccess,
			wantErr:      true,
			wantAttempts: 2,
		},
		{
			name:         "200 OK accepted as success",
			responses:    []int{http.StatusOK},
			upload:       uploadJobStatusSuccess,
			wantAttempts: 1,
		},
		{
			name:         "204 No Content accepted as success",
			responses:    []int{http.StatusNoContent},
			upload:       uploadJobStatusSuccess,
			wantAttempts: 1,
		},
		// --- UploadJobStatus: conflict stays permanent ---
		{
			name:         "default upload keeps conflict permanent",
			responses:    []int{http.StatusConflict},
			upload:       uploadJobStatusSuccess,
			wantErr:      true,
			wantAttempts: 1,
		},
		// --- UploadJobStatusReconcile: conflict is accepted ---
		{
			name:         "reconcile accepts conflict",
			responses:    []int{http.StatusConflict},
			upload:       uploadJobStatusReconcileSuccess,
			wantAttempts: 1,
		},
		{
			name:         "reconcile retries on 5xx then accepts conflict",
			responses:    []int{http.StatusInternalServerError, http.StatusConflict},
			upload:       uploadJobStatusReconcileSuccess,
			wantAttempts: 2,
		},
		{
			name:         "reconcile keeps non conflict 4xx permanent",
			responses:    []int{http.StatusForbidden},
			upload:       uploadJobStatusReconcileSuccess,
			wantErr:      true,
			wantAttempts: 1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, attempts := newSequenceServer(t, tt.responses)
			uploader := newTestUploader(t, server.URL)

			err := tt.upload(uploader, context.Background(), types.JobID("test-job-id"))
			checkErr(t, tt.wantErr, err)

			if *attempts != tt.wantAttempts {
				t.Errorf("attempts = %d, want %d", *attempts, tt.wantAttempts)
			}
		})
	}
}

// TestStatusUploader_RetryBackoff verifies exponential backoff on retries.
func TestStatusUploader_RetryBackoff(t *testing.T) {
	t.Parallel()

	server, attempts := newSequenceServer(t, []int{
		http.StatusInternalServerError,
		http.StatusInternalServerError,
		http.StatusNoContent,
	})
	uploader := newTestUploader(t, server.URL)

	start := time.Now()
	err := uploader.UploadJobStatus(context.Background(), types.JobID("test-job-id"),
		types.JobStatusSuccess.String(), nil, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// With shared backoff policy (100ms initial, 2x multiplier, 50% jitter):
	// conservative lower bound accounts for jitter.
	if elapsed < 150*time.Millisecond {
		t.Errorf("expected backoff duration >= 150ms, got %v", elapsed)
	}
	if *attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", *attempts)
	}
}

// TestStatusUploader_ContextCancellation verifies context cancellation during retry.
func TestStatusUploader_ContextCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	uploader := newTestUploader(t, server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := uploader.UploadJobStatus(ctx, types.JobID("test-job-id"),
		types.JobStatusSuccess.String(), nil, nil)
	if err == nil {
		t.Error("expected context cancellation error")
	}
	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}
}

// TestStatusUploader_PayloadIncludesStatusExitCodeAndStats verifies the payload
// shape for job-level completion requests.
func TestStatusUploader_PayloadIncludesStatusExitCodeAndStats(t *testing.T) {
	t.Parallel()

	var receivedPayload map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			t.Errorf("failed to decode payload: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	uploader := newTestUploader(t, server.URL)

	stats := types.NewRunStatsBuilder().ExitCode(1).DurationMs(1500).MustBuild()
	err := uploader.UploadJobStatus(context.Background(), types.JobID("test-job-id-uuid"),
		types.JobStatusFail.String(), int32Ptr(1), stats)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if receivedPayload["status"] != types.JobStatusFail.String() {
		t.Errorf("expected status=%s, got %v", types.JobStatusFail.String(), receivedPayload["status"])
	}
	if ec, ok := receivedPayload["exit_code"].(float64); !ok || ec != 1 {
		t.Errorf("expected exit_code=1, got %v", receivedPayload["exit_code"])
	}
	statsPayload, ok := receivedPayload["stats"].(map[string]interface{})
	if !ok {
		t.Fatal("stats not present or not a map in payload")
	}
	if ec, ok := statsPayload["exit_code"].(float64); !ok || ec != 1 {
		t.Errorf("expected stats.exit_code=1, got %v", statsPayload["exit_code"])
	}
}

func TestStatusUploader_UploadJobStatus_UsesJobEndpointAndPayloadShape(t *testing.T) {
	t.Parallel()

	var receivedPayload map[string]interface{}
	jobID := types.JobID("test-job-id-uuid")

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
		if got := r.Header.Get("PLOY_NODE_UUID"); got != testNodeID {
			t.Errorf("expected PLOY_NODE_UUID=%s, got %s", testNodeID, got)
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			t.Errorf("failed to decode payload: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	uploader := newTestUploader(t, server.URL)

	exitCode := int32(0)
	stats := types.NewRunStatsBuilder().ExitCode(0).DurationMs(1000).MustBuild()
	if err := uploader.UploadJobStatus(context.Background(), jobID,
		types.JobStatusSuccess.String(), &exitCode, stats); err != nil {
		t.Fatalf("unexpected error uploading job status: %v", err)
	}

	if receivedPayload["status"] != types.JobStatusSuccess.String() {
		t.Errorf("expected status=%s, got %v", types.JobStatusSuccess.String(), receivedPayload["status"])
	}
	if ec, ok := receivedPayload["exit_code"].(float64); !ok || ec != 0 {
		t.Errorf("expected exit_code=0, got %v", receivedPayload["exit_code"])
	}
	statsPayload, ok := receivedPayload["stats"].(map[string]interface{})
	if !ok {
		t.Fatal("stats not present or not a map in payload")
	}
	if ec, ok := statsPayload["exit_code"].(float64); !ok || ec != 0 {
		t.Errorf("expected stats.exit_code=0, got %v", statsPayload["exit_code"])
	}
	for _, unwanted := range []string{"run_id", "job_id", "next_id"} {
		if _, ok := receivedPayload[unwanted]; ok {
			t.Errorf("did not expect %q in job-level payload", unwanted)
		}
	}
}
