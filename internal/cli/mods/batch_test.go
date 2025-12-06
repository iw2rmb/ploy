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
					ID:        "batch-001",
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
					ID:        "batch-002",
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
				{ID: "batch-page", Status: "queued"},
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

// TestGetBatchStatusCommand_Run validates GetBatchStatusCommand responses.
func TestGetBatchStatusCommand_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		batchID     string
		serverResp  BatchSummary
		statusCode  int
		wantErr     bool
		wantErrText string
	}{
		{
			name:    "successful status fetch",
			batchID: "batch-123",
			serverResp: BatchSummary{
				ID:        "batch-123",
				Name:      strPtr("my-batch"),
				Status:    "running",
				RepoURL:   "https://github.com/org/repo.git",
				BaseRef:   "main",
				TargetRef: "feature",
				CreatedAt: time.Now(),
				Counts: &RunRepoCounts{
					Total:         3,
					Pending:       1,
					Running:       1,
					Succeeded:     1,
					DerivedStatus: "running",
				},
			},
			statusCode: http.StatusOK,
		},
		{
			name:        "batch not found",
			batchID:     "nonexistent",
			statusCode:  http.StatusNotFound,
			wantErr:     true,
			wantErrText: "run not found",
		},
		{
			name:        "empty batch id",
			batchID:     "",
			wantErr:     true,
			wantErrText: "batch id required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Set up mock server.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

			cmd := GetBatchStatusCommand{
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
			if result.ID != tc.serverResp.ID {
				t.Errorf("got ID %q, want %q", result.ID, tc.serverResp.ID)
			}
			if result.Status != tc.serverResp.Status {
				t.Errorf("got status %q, want %q", result.Status, tc.serverResp.Status)
			}
		})
	}
}

// TestStopBatchCommand_Run validates StopBatchCommand responses.
func TestStopBatchCommand_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		batchID     string
		serverResp  BatchSummary
		statusCode  int
		wantErr     bool
		wantErrText string
	}{
		{
			name:    "successful stop",
			batchID: "batch-456",
			serverResp: BatchSummary{
				ID:     "batch-456",
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
			batchID:     "nonexistent",
			statusCode:  http.StatusNotFound,
			wantErr:     true,
			wantErrText: "run not found",
		},
		{
			name:        "empty batch id",
			batchID:     "",
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
		name        string
		batchID     string
		serverResp  StartBatchResult
		statusCode  int
		wantErr     bool
		wantErrText string
	}{
		{
			name:    "successful start",
			batchID: "batch-789",
			serverResp: StartBatchResult{
				RunID:       "batch-789",
				Started:     3,
				AlreadyDone: 1,
				Pending:     0,
			},
			statusCode: http.StatusOK,
		},
		{
			name:    "start with pending",
			batchID: "batch-partial",
			serverResp: StartBatchResult{
				RunID:       "batch-partial",
				Started:     2,
				AlreadyDone: 0,
				Pending:     3,
			},
			statusCode: http.StatusOK,
		},
		{
			name:        "batch not found",
			batchID:     "nonexistent",
			statusCode:  http.StatusNotFound,
			wantErr:     true,
			wantErrText: "run not found",
		},
		{
			name:        "empty batch id",
			batchID:     "",
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

			cmd := StartBatchCommand{
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
					TicketID  string `json:"ticket_id"`
					Status    string `json:"status"`
					RepoURL   string `json:"repo_url"`
					BaseRef   string `json:"base_ref"`
					TargetRef string `json:"target_ref"`
				}{
					TicketID:  "batch-new-001",
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
			name: "status missing base URL",
			cmd: &statusWrapper{GetBatchStatusCommand{
				Client:  http.DefaultClient,
				BatchID: "test",
			}},
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
		{
			name: "start missing batch ID",
			cmd: &startWrapper{StartBatchCommand{
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

	cmd := GetBatchStatusCommand{
		Client:  srv.Client(),
		BaseURL: baseURL,
		BatchID: "test",
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

type statusWrapper struct{ GetBatchStatusCommand }

func (w *statusWrapper) Run(ctx context.Context) (any, error) {
	return w.GetBatchStatusCommand.Run(ctx)
}

type stopWrapper struct{ StopBatchCommand }

func (w *stopWrapper) Run(ctx context.Context) (any, error) { return w.StopBatchCommand.Run(ctx) }

type startWrapper struct{ StartBatchCommand }

func (w *startWrapper) Run(ctx context.Context) (any, error) { return w.StartBatchCommand.Run(ctx) }
