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

func TestStartCommand_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		runID       domaintypes.RunID
		statusCode  int
		serverResp  StartResult
		wantErr     bool
		wantErrText string
	}{
		{
			name:  "start with pending repos",
			runID: domaintypes.RunID("run-partial"),
			serverResp: StartResult{
				RunID:       domaintypes.RunID("run-partial"),
				Started:     2,
				AlreadyDone: 0,
				Pending:     3,
			},
			statusCode: http.StatusOK,
		},
		{
			name:        "run not found",
			runID:       domaintypes.RunID("nonexistent"),
			statusCode:  http.StatusNotFound,
			wantErr:     true,
			wantErrText: "run start",
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
				// Verify POST method.
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				// Verify path ends with /start.
				if !strings.HasSuffix(r.URL.Path, "/start") {
					t.Errorf("expected path to end with /start, got %s", r.URL.Path)
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

			cmd := StartCommand{
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
			if result.Started != tc.serverResp.Started {
				t.Errorf("got Started %d, want %d", result.Started, tc.serverResp.Started)
			}
			if result.AlreadyDone != tc.serverResp.AlreadyDone {
				t.Errorf("got AlreadyDone %d, want %d", result.AlreadyDone, tc.serverResp.AlreadyDone)
			}
			if result.Pending != tc.serverResp.Pending {
				t.Errorf("got Pending %d, want %d", result.Pending, tc.serverResp.Pending)
			}
		})
	}
}
