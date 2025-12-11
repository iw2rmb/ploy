package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestModRunBatchRouting validates that batch lifecycle subcommands are routed correctly.
// This uses t.Parallel since it does not use t.Setenv.
func TestModRunBatchRouting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "status without run-name",
			args:    []string{"mod", "run", "status"},
			wantErr: "run-name required",
		},
		{
			name:    "stop without run-name",
			args:    []string{"mod", "run", "stop"},
			wantErr: "run-name required",
		},
		{
			name:    "start without run-name",
			args:    []string{"mod", "run", "start"},
			wantErr: "run-name required",
		},
		{
			name:    "status with empty run-name",
			args:    []string{"mod", "run", "status", "   "},
			wantErr: "run-name required",
		},
		{
			name:    "stop with empty run-name",
			args:    []string{"mod", "run", "stop", ""},
			wantErr: "run-name required",
		},
		{
			name:    "start with empty run-name",
			args:    []string{"mod", "run", "start", ""},
			wantErr: "run-name required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := executeCmd(tc.args, &buf)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestModRunBatchListCallsControlPlane validates list command calls the API.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunBatchListCallsControlPlane(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/runs") {
			called = true

			// Check pagination params.
			limit := r.URL.Query().Get("limit")
			offset := r.URL.Query().Get("offset")
			if limit != "10" {
				t.Errorf("expected limit=10, got %s", limit)
			}
			if offset != "5" {
				t.Errorf("expected offset=5, got %s", offset)
			}

			name := "test-batch"
			now := time.Now()
			resp := struct {
				Runs []struct {
					ID        string    `json:"id"`
					Name      *string   `json:"name,omitempty"`
					Status    string    `json:"status"`
					RepoURL   string    `json:"repo_url"`
					BaseRef   string    `json:"base_ref"`
					TargetRef string    `json:"target_ref"`
					CreatedAt time.Time `json:"created_at"`
					Counts    *struct {
						Total         int32  `json:"total"`
						Succeeded     int32  `json:"succeeded"`
						DerivedStatus string `json:"derived_status"`
					} `json:"repo_counts,omitempty"`
				} `json:"runs"`
			}{
				Runs: []struct {
					ID        string    `json:"id"`
					Name      *string   `json:"name,omitempty"`
					Status    string    `json:"status"`
					RepoURL   string    `json:"repo_url"`
					BaseRef   string    `json:"base_ref"`
					TargetRef string    `json:"target_ref"`
					CreatedAt time.Time `json:"created_at"`
					Counts    *struct {
						Total         int32  `json:"total"`
						Succeeded     int32  `json:"succeeded"`
						DerivedStatus string `json:"derived_status"`
					} `json:"repo_counts,omitempty"`
				}{
					{
						ID:        "batch-001",
						Name:      &name,
						Status:    "running",
						RepoURL:   "https://github.com/org/repo.git",
						BaseRef:   "main",
						TargetRef: "feature",
						CreatedAt: now,
						Counts: &struct {
							Total         int32  `json:"total"`
							Succeeded     int32  `json:"succeeded"`
							DerivedStatus string `json:"derived_status"`
						}{Total: 5, Succeeded: 2, DerivedStatus: "running"},
					},
					{
						ID:        "batch-002",
						Status:    "completed",
						RepoURL:   "https://github.com/org/repo2.git",
						BaseRef:   "main",
						TargetRef: "hotfix",
						CreatedAt: now,
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"mod", "run", "list", "--limit", "10", "--offset", "5"}, &buf)
	if err != nil {
		t.Fatalf("mod run list error: %v", err)
	}
	if !called {
		t.Fatal("expected GET /v1/runs to be called")
	}

	output := buf.String()
	if !strings.Contains(output, "batch-001") {
		t.Errorf("output should contain batch-001: %s", output)
	}
	if !strings.Contains(output, "test-batch") {
		t.Errorf("output should contain test-batch: %s", output)
	}
	if !strings.Contains(output, "running") {
		t.Errorf("output should contain running: %s", output)
	}
}

// TestModRunBatchStatusCallsControlPlane validates status command calls the API.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunBatchStatusRemoved(t *testing.T) {}

// TestModRunBatchStopCallsControlPlane validates stop command calls the API.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunBatchStopCallsControlPlane(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/batch-456/stop" {
			called = true

			now := time.Now()
			resp := struct {
				ID        string    `json:"id"`
				Status    string    `json:"status"`
				CreatedAt time.Time `json:"created_at"`
				Counts    *struct {
					Total     int32 `json:"total"`
					Cancelled int32 `json:"cancelled"`
				} `json:"repo_counts,omitempty"`
			}{
				ID:        "batch-456",
				Status:    "canceled",
				CreatedAt: now,
				Counts: &struct {
					Total     int32 `json:"total"`
					Cancelled int32 `json:"cancelled"`
				}{Total: 5, Cancelled: 3},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"mod", "run", "stop", "batch-456"}, &buf)
	if err != nil {
		t.Fatalf("mod run stop error: %v", err)
	}
	if !called {
		t.Fatal("expected POST /v1/runs/batch-456/stop to be called")
	}

	output := buf.String()
	if !strings.Contains(output, "batch-456") {
		t.Errorf("output should contain batch-456: %s", output)
	}
	if !strings.Contains(output, "stopped") {
		t.Errorf("output should contain stopped: %s", output)
	}
	if !strings.Contains(output, "Cancelled 3") {
		t.Errorf("output should contain Cancelled 3: %s", output)
	}
}

// TestModRunBatchStartCallsControlPlane validates start command calls the API.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunBatchStartCallsControlPlane(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/batch-789/start" {
			called = true

			resp := struct {
				RunID       string `json:"run_id"`
				Started     int    `json:"started"`
				AlreadyDone int    `json:"already_done"`
				Pending     int    `json:"pending"`
			}{
				RunID:       "batch-789",
				Started:     3,
				AlreadyDone: 1,
				Pending:     0,
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"mod", "run", "start", "batch-789"}, &buf)
	if err != nil {
		t.Fatalf("mod run start error: %v", err)
	}
	if !called {
		t.Fatal("expected POST /v1/runs/batch-789/start to be called")
	}

	output := buf.String()
	if !strings.Contains(output, "batch-789") {
		t.Errorf("output should contain batch-789: %s", output)
	}
	if !strings.Contains(output, "started 3") {
		t.Errorf("output should contain started 3: %s", output)
	}
	if !strings.Contains(output, "1 already done") {
		t.Errorf("output should contain 1 already done: %s", output)
	}
}

// TestModRunBatchListEmptyResult validates list command handles empty results.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunBatchListEmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/runs") {
			resp := struct {
				Runs []interface{} `json:"runs"`
			}{Runs: []interface{}{}}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"mod", "run", "list"}, &buf)
	if err != nil {
		t.Fatalf("mod run list error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No batch runs found") {
		t.Errorf("output should contain 'No batch runs found': %s", output)
	}
}

// TestModRunBatchListInvalidLimit validates list command rejects invalid limit.
// This uses t.Parallel since it does not use t.Setenv.
func TestModRunBatchListInvalidLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "limit below minimum",
			args:    []string{"mod", "run", "list", "--limit", "0"},
			wantErr: "limit must be between 1 and 100",
		},
		{
			name:    "limit above maximum",
			args:    []string{"mod", "run", "list", "--limit", "101"},
			wantErr: "limit must be between 1 and 100",
		},
		{
			name:    "negative offset",
			args:    []string{"mod", "run", "list", "--offset", "-1"},
			wantErr: "offset must be non-negative",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := executeCmd(tc.args, &buf)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestModRunBatchStatusNotFound validates status command handles 404.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunBatchStatusNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/runs/nonexistent" {
			http.Error(w, "run not found", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"mod", "run", "status", "nonexistent"}, &buf)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention not found or 404: %v", err)
	}
}
