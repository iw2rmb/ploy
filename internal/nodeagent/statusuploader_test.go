package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusUploader_UploadStatus(t *testing.T) {
	tests := []struct {
		name           string
		status         string
		reason         *string
		stats          map[string]interface{}
		wantStatusCode int
		wantErr        bool
	}{
		{
			name:   "successful upload with stats",
			status: "succeeded",
			reason: nil,
			stats: map[string]interface{}{
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
			name:   "failed status with reason",
			status: "failed",
			reason: stringPtr("exit code 1"),
			stats: map[string]interface{}{
				"exit_code":   1,
				"duration_ms": 500,
			},
			wantStatusCode: http.StatusNoContent,
			wantErr:        false,
		},
		{
			name:           "minimal upload without stats",
			status:         "succeeded",
			reason:         nil,
			stats:          nil,
			wantStatusCode: http.StatusNoContent,
			wantErr:        false,
		},
		{
			name:           "server error",
			status:         "succeeded",
			reason:         nil,
			stats:          map[string]interface{}{},
			wantStatusCode: http.StatusInternalServerError,
			wantErr:        true,
		},
		{
			name:           "conflict error (not running)",
			status:         "succeeded",
			reason:         nil,
			stats:          map[string]interface{}{},
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

				// Verify reason is present when expected.
				if tt.reason != nil {
					if _, ok := payload["reason"]; !ok {
						t.Error("reason not present in payload when expected")
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
			err = uploader.UploadStatus(ctx, "test-run-id", tt.status, tt.reason, tt.stats)

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
	reason := stringPtr("test failure")
	stats := map[string]interface{}{
		"exit_code":   1,
		"duration_ms": 2500,
		"timings": map[string]interface{}{
			"hydration_duration_ms": 500,
			"execution_duration_ms": 2000,
		},
	}

	err = uploader.UploadStatus(ctx, "test-run-id", "failed", reason, stats)
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

	if receivedPayload["reason"] != "test failure" {
		t.Errorf("expected reason='test failure', got %v", receivedPayload["reason"])
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

// Helper function to create string pointers.
func stringPtr(s string) *string {
	return &s
}
