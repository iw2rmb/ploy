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
		serverBody  any
		jsonError   string
		wantErr     bool
		wantErrText string
		wantErrNo   string
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
			name:       "contract drift: unknown field",
			runID:      domaintypes.RunID("run-drift"),
			statusCode: http.StatusOK,
			serverBody: map[string]any{
				"run_id":        "run-drift",
				"started":       1,
				"already_done":  0,
				"pending":       0,
				"extra_field":   true,
				"extra_field_2": "ignored",
			},
			wantErr:     true,
			wantErrText: "unknown field",
		},
		{
			name:        "run not found",
			runID:       domaintypes.RunID("nonexistent"),
			statusCode:  http.StatusNotFound,
			wantErr:     true,
			wantErrText: "run start",
		},
		{
			name:        "run not found with json error",
			runID:       domaintypes.RunID("nonexistent-json"),
			statusCode:  http.StatusNotFound,
			jsonError:   "run not found (json)",
			wantErr:     true,
			wantErrText: "run not found (json)",
			wantErrNo:   "{\"error\"",
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
				// Verify BaseURL.Path preservation.
				if !strings.HasPrefix(r.URL.Path, "/api/v1/runs/") {
					t.Errorf("expected path to start with /api/v1/runs/, got %s", r.URL.Path)
				}
				// Verify path ends with /start.
				if !strings.HasSuffix(r.URL.Path, "/start") {
					t.Errorf("expected path to end with /start, got %s", r.URL.Path)
				}

				if tc.statusCode == http.StatusNotFound {
					if tc.jsonError != "" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusNotFound)
						_ = json.NewEncoder(w).Encode(map[string]string{"error": tc.jsonError})
						return
					}
					http.Error(w, "run not found", http.StatusNotFound)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				if tc.serverBody != nil {
					_ = json.NewEncoder(w).Encode(tc.serverBody)
					return
				}
				_ = json.NewEncoder(w).Encode(tc.serverResp)
			}))
			t.Cleanup(srv.Close)

			baseURL, err := url.Parse(srv.URL + "/api")
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
				if tc.wantErrNo != "" && strings.Contains(err.Error(), tc.wantErrNo) {
					t.Errorf("error %q should not contain %q", err.Error(), tc.wantErrNo)
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
