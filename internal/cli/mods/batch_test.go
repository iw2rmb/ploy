package mods

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

// TestListBatchesCommand_Run validates ListBatchesCommand with pagination.
func TestListBatchesCommand_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		limit       int32
		offset      int32
		serverResp  []BatchSummary
		wantCount   int
		wantErr     bool
		wantErrText string
	}{
		{
			name:   "list batches with results",
			limit:  50,
			offset: 0,
			serverResp: []BatchSummary{
				{
					ID:        domaintypes.RunID("batch-001"), // Convert to domain type
					Name:      strPtr("test-batch"),
					Status:    "running",
					RepoURL:   "https://github.com/org/repo.git",
					BaseRef:   "main",
					TargetRef: "feature",
					CreatedAt: time.Now(),
					Counts: &RunRepoCounts{
						Total:         5,
						Pending:       2,
						Running:       1,
						Succeeded:     2,
						DerivedStatus: "running",
					},
				},
				{
					ID:        domaintypes.RunID("batch-002"), // Convert to domain type
					Status:    "completed",
					RepoURL:   "https://github.com/org/repo2.git",
					BaseRef:   "main",
					TargetRef: "hotfix",
					CreatedAt: time.Now(),
				},
			},
			wantCount: 2,
		},
		{
			name:       "empty list",
			limit:      50,
			offset:     0,
			serverResp: []BatchSummary{},
			wantCount:  0,
		},
		{
			name:   "with pagination",
			limit:  10,
			offset: 5,
			serverResp: []BatchSummary{
				{ID: domaintypes.RunID("batch-page"), Status: "queued"}, // Convert to domain type
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
				if !strings.HasPrefix(r.URL.Path, "/v1/runs") {
					t.Errorf("expected path /v1/runs, got %s", r.URL.Path)
				}

				// Verify pagination query params if provided.
				if tc.limit > 0 {
					limitParam := r.URL.Query().Get("limit")
					if limitParam == "" {
						t.Error("expected limit query param")
					}
				}

				resp := struct {
					Runs []BatchSummary `json:"runs"`
				}{Runs: tc.serverResp}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			}))
			t.Cleanup(srv.Close)

			baseURL, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatalf("parse server URL: %v", err)
			}

			cmd := ListBatchesCommand{
				Client:  srv.Client(),
				BaseURL: baseURL,
				Limit:   tc.limit,
				Offset:  tc.offset,
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

// TestStopBatchCommand_Run validates StopBatchCommand responses.
func TestStopBatchCommand_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		batchID     domaintypes.RunID // Updated to domain type
		serverResp  BatchSummary
		statusCode  int
		wantErr     bool
		wantErrText string
	}{
		{
			name:    "successful stop",
			batchID: domaintypes.RunID("batch-456"), // Convert to domain type
			serverResp: BatchSummary{
				ID:     domaintypes.RunID("batch-456"), // Convert to domain type
				Status: "canceled",
				Counts: &RunRepoCounts{
					Total:         5,
					Cancelled:     3,
					Succeeded:     2,
					DerivedStatus: "cancelled",
				},
			},
			statusCode: http.StatusOK,
		},
		{
			name:        "batch not found",
			batchID:     domaintypes.RunID("nonexistent"), // Convert to domain type
			statusCode:  http.StatusNotFound,
			wantErr:     true,
			wantErrText: "run not found",
		},
		{
			name:        "empty batch id",
			batchID:     domaintypes.RunID(""), // Empty domain type
			wantErr:     true,
			wantErrText: "batch id required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify POST method.
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				// Verify path ends with /stop.
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

			cmd := StopBatchCommand{
				Client:  srv.Client(),
				BaseURL: baseURL,
				BatchID: tc.batchID,
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

// TestStartBatchCommand_Run validates StartBatchCommand responses.
func TestStartBatchCommand_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		batchID    domaintypes.RunID // Updated to domain type
		serverResp struct {
			RunID       domaintypes.RunID
			Started     int
			AlreadyDone int
			Pending     int
		}
		statusCode  int
		wantErr     bool
		wantErrText string
	}{
		{
			name:    "successful start",
			batchID: domaintypes.RunID("batch-789"), // Convert to domain type
			serverResp: struct {
				RunID       domaintypes.RunID
				Started     int
				AlreadyDone int
				Pending     int
			}{
				RunID:       domaintypes.RunID("batch-789"), // Convert to domain type
				Started:     3,
				AlreadyDone: 1,
				Pending:     0,
			},
			statusCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
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

			_ = baseURL
			_ = tc
			t.Skip("StartBatchCommand has been replaced by runs.StartCommand")
		})
	}
}

// TestCreateBatchCommand_Run validates CreateBatchCommand responses.
func TestCreateBatchCommand_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		repoURL     string
		baseRef     string
		targetRef   string
		batchName   *string
		statusCode  int
		wantErr     bool
		wantErrText string
	}{
		{
			name:       "successful create",
			repoURL:    "https://github.com/org/repo.git",
			baseRef:    "main",
			targetRef:  "feature-branch",
			batchName:  strPtr("test-batch"),
			statusCode: http.StatusCreated,
		},
		{
			name:       "create without name",
			repoURL:    "https://github.com/org/repo.git",
			baseRef:    "main",
			targetRef:  "hotfix",
			batchName:  nil,
			statusCode: http.StatusCreated,
		},
		{
			name:        "missing client",
			repoURL:     "https://github.com/org/repo.git",
			baseRef:     "main",
			targetRef:   "feature",
			wantErr:     true,
			wantErrText: "http client required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify POST method.
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				// Verify path is /v1/mods.
				if r.URL.Path != "/v1/mods" {
					t.Errorf("expected path /v1/mods, got %s", r.URL.Path)
				}

				// Decode and verify request body.
				var req struct {
					Name      *string `json:"name,omitempty"`
					RepoURL   string  `json:"repo_url"`
					BaseRef   string  `json:"base_ref"`
					TargetRef string  `json:"target_ref"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("decode request body: %v", err)
				}
				if req.RepoURL != tc.repoURL {
					t.Errorf("request repo_url = %q, want %q", req.RepoURL, tc.repoURL)
				}

				resp := struct {
					RunID     string `json:"run_id"` // Run ID returned from server
					Status    string `json:"status"`
					RepoURL   string `json:"repo_url"`
					BaseRef   string `json:"base_ref"`
					TargetRef string `json:"target_ref"`
				}{
					RunID:     "batch-new-001",
					Status:    "queued",
					RepoURL:   tc.repoURL,
					BaseRef:   tc.baseRef,
					TargetRef: tc.targetRef,
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_ = json.NewEncoder(w).Encode(resp)
			}))
			t.Cleanup(srv.Close)

			baseURL, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatalf("parse server URL: %v", err)
			}

			var client *http.Client
			if !tc.wantErr || !strings.Contains(tc.wantErrText, "http client required") {
				client = srv.Client()
			}

			cmd := CreateBatchCommand{
				Client:    client,
				BaseURL:   baseURL,
				Name:      tc.batchName,
				RepoURL:   tc.repoURL,
				BaseRef:   tc.baseRef,
				TargetRef: tc.targetRef,
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
			if result.ID == "" {
				t.Error("expected non-empty batch ID")
			}
			if result.Status != "queued" {
				t.Errorf("got status %q, want %q", result.Status, "queued")
			}
		})
	}
}

// TestBatchCommand_Errors validates error handling for missing required fields.
func TestBatchCommand_Errors(t *testing.T) {
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
			cmd:     &listWrapper{ListBatchesCommand{BaseURL: &url.URL{Scheme: "http", Host: "localhost"}}},
			wantErr: "http client required",
		},
		{
			name:    "list missing base URL",
			cmd:     &listWrapper{ListBatchesCommand{Client: http.DefaultClient}},
			wantErr: "base url required",
		},
		{
			name: "stop missing batch ID",
			cmd: &stopWrapper{StopBatchCommand{
				Client:  http.DefaultClient,
				BaseURL: &url.URL{Scheme: "http", Host: "localhost"},
			}},
			wantErr: "batch id required",
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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	t.Cleanup(srv.Close)

	baseURL, _ := url.Parse(srv.URL)

	cmd := ListBatchesCommand{
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
}

// strPtr is a helper to create *string from string literal.
func strPtr(s string) *string {
	return &s
}

// Wrapper types to unify Run() signature for error tests.
type listWrapper struct{ ListBatchesCommand }

func (w *listWrapper) Run(ctx context.Context) (any, error) { return w.ListBatchesCommand.Run(ctx) }

type stopWrapper struct{ StopBatchCommand }

func (w *stopWrapper) Run(ctx context.Context) (any, error) { return w.StopBatchCommand.Run(ctx) }
