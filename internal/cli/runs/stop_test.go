package runs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestStopCommand_Run validates StopCommand responses.
func TestStopCommand_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		runID       domaintypes.RunID
		serverResp  Summary
		statusCode  int
		wantErr     bool
		wantErrText string
	}{
		{
			name:  "successful stop",
			runID: domaintypes.RunID("run-456"),
			serverResp: Summary{
				ID:     domaintypes.RunID("run-456"),
				Status: "canceled",
			},
			statusCode: http.StatusOK,
		},
		{
			name:        "run not found",
			runID:       domaintypes.RunID("nonexistent"),
			statusCode:  http.StatusNotFound,
			wantErr:     true,
			wantErrText: "run stop",
		},
		{
			name:        "empty run id",
			runID:       domaintypes.RunID(""),
			wantErr:     true,
			wantErrText: "run id required",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if !strings.HasSuffix(r.URL.Path, "/stop") {
					t.Errorf("expected path to end with /stop, got %s", r.URL.Path)
				}

				if tc.statusCode == http.StatusNotFound {
					http.Error(w, "run not found", http.StatusNotFound)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(tc.serverResp)
			}))
			t.Cleanup(srv.Close)

			baseURL, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatalf("parse server URL: %v", err)
			}

			cmd := StopCommand{
				Client:  srv.Client(),
				BaseURL: baseURL,
				RunID:   tc.runID,
			}

			result, err := cmd.Run(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantErrText != "" && !strings.Contains(err.Error(), tc.wantErrText) {
					t.Errorf("error %q should contain %q", err.Error(), tc.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if result.Status != tc.serverResp.Status {
				t.Errorf("got status %q, want %q", result.Status, tc.serverResp.Status)
			}
		})
	}
}
