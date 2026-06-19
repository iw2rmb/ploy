package migs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestListRunsCommand_Run validates ListRunsCommand with pagination.
func TestListRunsCommand_Run(t *testing.T) {
	t.Parallel()

	const basePathPrefix = "/api"
	runID1 := domaintypes.NewRunID()
	runID2 := domaintypes.NewRunID()
	runIDPage := domaintypes.NewRunID()
	migID1 := domaintypes.NewMigID()
	migID2 := domaintypes.NewMigID()
	migIDPage := domaintypes.NewMigID()
	specID1 := domaintypes.NewSpecID()
	specID2 := domaintypes.NewSpecID()
	specIDPage := domaintypes.NewSpecID()

	tests := []struct {
		name        string
		limit       int32
		offset      int32
		createdBy   string
		all         bool
		serverResp  []domaintypes.RunSummary
		wantCount   int
		wantErr     bool
		wantErrText string
	}{
		{
			name:   "list runs with results",
			limit:  50,
			offset: 0,
			serverResp: []domaintypes.RunSummary{
				{
					ID:        runID1,
					Status:    "Started",
					MigID:     migID1,
					SpecID:    specID1,
					CreatedAt: time.Now(),
					Counts: &domaintypes.RunCounts{
						Total:         5,
						Queued:        2,
						Running:       1,
						Success:       2,
						DerivedStatus: "running",
					},
				},
				{
					ID:        runID2,
					Status:    "Finished",
					MigID:     migID2,
					SpecID:    specID2,
					CreatedAt: time.Now(),
				},
			},
			wantCount: 2,
		},
		{
			name:       "empty list",
			limit:      50,
			offset:     0,
			serverResp: []domaintypes.RunSummary{},
			wantCount:  0,
		},
		{
			name:   "with pagination",
			limit:  10,
			offset: 5,
			serverResp: []domaintypes.RunSummary{
				{ID: runIDPage, Status: "Started", MigID: migIDPage, SpecID: specIDPage},
			},
			wantCount: 1,
		},
		{
			name:      "scoped by created_by",
			limit:     10,
			createdBy: "alice",
			serverResp: []domaintypes.RunSummary{
				{ID: runIDPage, Status: "Started", MigID: migIDPage, SpecID: specIDPage},
			},
			wantCount: 1,
		},
		{
			name:  "all runs",
			limit: 10,
			all:   true,
			serverResp: []domaintypes.RunSummary{
				{ID: runIDPage, Status: "Started", MigID: migIDPage, SpecID: specIDPage},
			},
			wantCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Set up mock server returning the test response.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and path.
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				if !strings.HasPrefix(r.URL.Path, basePathPrefix+"/v1/runs") {
					t.Errorf("expected path %s/v1/runs, got %s", basePathPrefix, r.URL.Path)
				}

				// Verify pagination query params if provided.
				if tc.limit > 0 {
					limitParam := r.URL.Query().Get("limit")
					if limitParam == "" {
						t.Error("expected limit query param")
					}
				}
				if got := r.URL.Query().Get("created_by"); got != tc.createdBy {
					t.Errorf("created_by = %q, want %q", got, tc.createdBy)
				}
				wantAll := ""
				if tc.all {
					wantAll = "true"
				}
				if got := r.URL.Query().Get("all"); got != wantAll {
					t.Errorf("all = %q, want %q", got, wantAll)
				}

				resp := struct {
					Runs []domaintypes.RunSummary `json:"runs"`
				}{Runs: tc.serverResp}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			}))
			t.Cleanup(srv.Close)

			baseURL, err := url.Parse(srv.URL + basePathPrefix)
			if err != nil {
				t.Fatalf("parse server URL: %v", err)
			}

			cmd := ListRunsCommand{
				Client:    srv.Client(),
				BaseURL:   baseURL,
				Limit:     tc.limit,
				Offset:    tc.offset,
				CreatedBy: tc.createdBy,
				All:       tc.all,
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
			if len(result) != tc.wantCount {
				t.Errorf("got %d results, want %d", len(result), tc.wantCount)
			}
		})
	}
}

// TestRunCommand_Errors validates error handling for missing required fields.
func TestRunCommand_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  interface {
			Run(context.Context) (any, error)
		}
		wantErr string
	}{
		{
			name:    "list missing client",
			cmd:     &listWrapper{ListRunsCommand{BaseURL: &url.URL{Scheme: "http", Host: "localhost"}}},
			wantErr: "http client required",
		},
		{
			name:    "list missing base URL",
			cmd:     &listWrapper{ListRunsCommand{Client: http.DefaultClient}},
			wantErr: "base url required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := tc.cmd.Run(context.Background())
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestHTTPError validates HTTP error response decoding.
func TestHTTPError(t *testing.T) {
	t.Parallel()

	const basePathPrefix = "/api"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	t.Cleanup(srv.Close)

	baseURL, _ := url.Parse(srv.URL + basePathPrefix)

	cmd := ListRunsCommand{
		Client:  srv.Client(),
		BaseURL: baseURL,
	}

	_, err := cmd.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "internal server error") {
		t.Errorf("error should contain server message: %v", err)
	}
	if strings.Contains(err.Error(), "{\"error\"") {
		t.Errorf("error should prefer decoded error message, got: %v", err)
	}
}

// Wrapper types to unify Run() signature for error tests.
type listWrapper struct{ ListRunsCommand }

func (w *listWrapper) Run(ctx context.Context) (any, error) { return w.ListRunsCommand.Run(ctx) }
