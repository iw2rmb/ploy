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

func TestStatusUploader_UploadStatus(t *testing.T) {
	tests := []struct {
		name           string
		status         string
		exitCode       *int32
		stats          types.RunStats
		wantStatusCode int
		wantErr        bool
	}{
		{
			name:     "successful upload with stats",
			status:   "succeeded",
			exitCode: int32Ptr(0),
			stats: types.RunStats{
				"exit_code":   0,
				"duration_ms": 1000,
				"timings": map[string]interface{}{
					"total_duration_ms": 1000,
				},
			},
			wantStatusCode: http.StatusNoContent,
			wantErr:        false,
		},
		{
			name:     "failed status with exit code",
			status:   "failed",
			exitCode: int32Ptr(1),
			stats: types.RunStats{
				"exit_code":   1,
				"duration_ms": 500,
			},
			wantStatusCode: http.StatusNoContent,
			wantErr:        false,
		},
		{
			name:           "minimal upload without stats",
			status:         "succeeded",
			exitCode:       nil,
			stats:          nil,
			wantStatusCode: http.StatusNoContent,
			wantErr:        false,
		},
		{
			name:           "server error",
			status:         "succeeded",
			exitCode:       nil,
			stats:          types.RunStats{},
			wantStatusCode: http.StatusInternalServerError,
			wantErr:        true,
		},
		{
			name:           "conflict error (not running)",
			status:         "succeeded",
			exitCode:       nil,
			stats:          types.RunStats{},
			wantStatusCode: http.StatusConflict,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and path.
				if r.Method != http.MethodPost {
					t.Errorf("expected POST request, got %s", r.Method)
				}

				// Verify content type.
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("expected Content-Type application/json, got %s", ct)
				}

				// Decode and verify payload.
				var payload map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("failed to decode payload: %v", err)
				}

				// Verify run_id is present.
				if _, ok := payload["run_id"]; !ok {
					t.Error("run_id not present in payload")
				}

				// Verify status is present.
				if _, ok := payload["status"]; !ok {
					t.Error("status not present in payload")
				}

				// Verify exit_code is present when expected.
				if tt.exitCode != nil {
					if _, ok := payload["exit_code"]; !ok {
						t.Error("exit_code not present in payload when expected")
					}
				}

				// Verify stats is present when expected.
				if tt.stats != nil {
					if _, ok := payload["stats"]; !ok {
						t.Error("stats not present in payload when expected")
					}
				}

				w.WriteHeader(tt.wantStatusCode)
			}))
			defer server.Close()

			// Create uploader with test config.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    "test-node-id",
				HTTP: HTTPConfig{
					TLS: TLSConfig{
						Enabled: false,
					},
				},
			}

			uploader, err := NewStatusUploader(cfg)
			if err != nil {
				t.Fatalf("failed to create uploader: %v", err)
			}

			// Upload status.
			ctx := context.Background()
			err = uploader.UploadStatus(ctx, "test-run-id", tt.status, tt.exitCode, tt.stats, 1000)

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestStatusUploader_PayloadFormat(t *testing.T) {
	var receivedPayload map[string]interface{}

	// Create a test server that captures the payload.
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
		NodeID:    "test-node-id",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	uploader, err := NewStatusUploader(cfg)
	if err != nil {
		t.Fatalf("failed to create uploader: %v", err)
	}

	ctx := context.Background()
	exitCode := int32Ptr(1)
	stats := types.RunStats{
		"exit_code":   1,
		"duration_ms": 2500,
		"timings": map[string]interface{}{
			"hydration_duration_ms": 500,
			"execution_duration_ms": 2000,
		},
	}

	err = uploader.UploadStatus(ctx, "test-run-id", "failed", exitCode, stats, 2000)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify payload structure.
	if receivedPayload["run_id"] != "test-run-id" {
		t.Errorf("expected run_id=test-run-id, got %v", receivedPayload["run_id"])
	}

	if receivedPayload["status"] != "failed" {
		t.Errorf("expected status=failed, got %v", receivedPayload["status"])
	}

	// Verify exit_code is present in payload.
	if ec, ok := receivedPayload["exit_code"].(float64); !ok || ec != 1 {
		t.Errorf("expected exit_code=1, got %v", receivedPayload["exit_code"])
	}

	if statsPayload, ok := receivedPayload["stats"].(map[string]interface{}); ok {
		if exitCode, ok := statsPayload["exit_code"].(float64); !ok || exitCode != 1 {
			t.Errorf("expected stats.exit_code=1, got %v", statsPayload["exit_code"])
		}
		if durationMs, ok := statsPayload["duration_ms"].(float64); !ok || durationMs != 2500 {
			t.Errorf("expected stats.duration_ms=2500, got %v", statsPayload["duration_ms"])
		}
	} else {
		t.Error("stats not present or not a map in payload")
	}
}

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
				NodeID:    "test-node-id",
				HTTP: HTTPConfig{
					TLS: TLSConfig{
						Enabled: false,
					},
				},
			}

			uploader, err := NewStatusUploader(cfg)
			if err != nil {
				t.Fatalf("failed to create uploader: %v", err)
			}

			ctx := context.Background()
			err = uploader.UploadStatus(ctx, "test-run-id", "succeeded", nil, nil, 1000)

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
		NodeID:    "test-node-id",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	uploader, err := NewStatusUploader(cfg)
	if err != nil {
		t.Fatalf("failed to create uploader: %v", err)
	}

	ctx := context.Background()
	start := time.Now()
	err = uploader.UploadStatus(ctx, "test-run-id", "succeeded", nil, nil, 1000)
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
		NodeID:    "test-node-id",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	uploader, err := NewStatusUploader(cfg)
	if err != nil {
		t.Fatalf("failed to create uploader: %v", err)
	}

	// Create context with short timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = uploader.UploadStatus(ctx, "test-run-id", "succeeded", nil, nil, 1000)

	if err == nil {
		t.Error("expected context cancellation error")
	}

	// Verify it's a context error.
	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}
}

// TestStatusUploader_StepIndexIncluded verifies step_index is included in payload
// when provided (multi-step run completion).
func TestStatusUploader_StepIndexIncluded(t *testing.T) {
	t.Parallel()

	var receivedPayload map[string]interface{}

	// Create a test server that captures the payload and verifies step_index.
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
		NodeID:    "test-node-id",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	uploader, err := NewStatusUploader(cfg)
	if err != nil {
		t.Fatalf("failed to create uploader: %v", err)
	}

	ctx := context.Background()
	stepIndex := types.StepIndex(2000)
	exitCode := int32Ptr(1)
	stats := types.RunStats{
		"exit_code":   1,
		"duration_ms": 1500,
	}

	// Upload status with step_index.
	err = uploader.UploadStatus(ctx, "test-run-id", "failed", exitCode, stats, stepIndex)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify payload structure includes step_index.
	if receivedPayload["run_id"] != "test-run-id" {
		t.Errorf("expected run_id=test-run-id, got %v", receivedPayload["run_id"])
	}

	if receivedPayload["status"] != "failed" {
		t.Errorf("expected status=failed, got %v", receivedPayload["status"])
	}

	// Verify exit_code is present.
	if ec, ok := receivedPayload["exit_code"].(float64); !ok || ec != 1 {
		t.Errorf("expected exit_code=1, got %v", receivedPayload["exit_code"])
	}

	// Verify step_index is present and correct.
	if stepIdx, ok := receivedPayload["step_index"].(float64); !ok || types.StepIndex(stepIdx) != stepIndex {
		t.Errorf("expected step_index=2000, got %v", receivedPayload["step_index"])
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
