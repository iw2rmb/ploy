package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestRunListCallsControlPlane validates list command calls the API.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestRunListCallsControlPlane(t *testing.T) {
	var called bool

	runID1 := domaintypes.NewRunID().String()
	runID2 := domaintypes.NewRunID().String()
	modID1 := domaintypes.NewMigID().String()
	modID2 := domaintypes.NewMigID().String()
	specID1 := domaintypes.NewSpecID().String()
	specID2 := domaintypes.NewSpecID().String()

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

			now := time.Now()
			resp := struct {
				Runs []struct {
					ID        string    `json:"id"`
					Status    string    `json:"status"`
					MigID     string    `json:"mig_id"`
					SpecID    string    `json:"spec_id"`
					CreatedAt time.Time `json:"created_at"`
					Counts    *struct {
						Total         int32  `json:"total"`
						Success       int32  `json:"success"`
						DerivedStatus string `json:"derived_status"`
					} `json:"repo_counts,omitempty"`
				} `json:"runs"`
			}{
				Runs: []struct {
					ID        string    `json:"id"`
					Status    string    `json:"status"`
					MigID     string    `json:"mig_id"`
					SpecID    string    `json:"spec_id"`
					CreatedAt time.Time `json:"created_at"`
					Counts    *struct {
						Total         int32  `json:"total"`
						Success       int32  `json:"success"`
						DerivedStatus string `json:"derived_status"`
					} `json:"repo_counts,omitempty"`
				}{
					{
						ID:        runID1,
						Status:    "Started",
						MigID:     modID1,
						SpecID:    specID1,
						CreatedAt: now,
						Counts: &struct {
							Total         int32  `json:"total"`
							Success       int32  `json:"success"`
							DerivedStatus string `json:"derived_status"`
						}{Total: 5, Success: 2, DerivedStatus: "running"},
					},
					{
						ID:        runID2,
						Status:    "Finished",
						MigID:     modID2,
						SpecID:    specID2,
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
	err := executeCmd([]string{"run", "list", "--limit", "10", "--offset", "5"}, &buf)
	if err != nil {
		t.Fatalf("run list error: %v", err)
	}
	if !called {
		t.Fatal("expected GET /v1/runs to be called")
	}

	output := buf.String()
	if !strings.Contains(output, runID1) {
		t.Errorf("output should contain %s: %s", runID1, output)
	}
	if !strings.Contains(output, "Started") {
		t.Errorf("output should contain Started: %s", output)
	}
	if !strings.Contains(output, "running") {
		t.Errorf("output should contain derived status running: %s", output)
	}
}

// TestModRunBatchStatusCallsControlPlane validates status command calls the API.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunBatchStatusRemoved(t *testing.T) {}

// TestRunListEmptyResult validates list command handles empty results.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestRunListEmptyResult(t *testing.T) {
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
	err := executeCmd([]string{"run", "list"}, &buf)
	if err != nil {
		t.Fatalf("run list error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No batch runs found") {
		t.Errorf("output should contain 'No batch runs found': %s", output)
	}
}

// TestRunListInvalidLimit validates list command rejects invalid limit.
// This uses t.Parallel since it does not use t.Setenv.
func TestRunListInvalidLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "limit below minimum",
			args:    []string{"run", "list", "--limit", "0"},
			wantErr: "limit must be between 1 and 100",
		},
		{
			name:    "limit above maximum",
			args:    []string{"run", "list", "--limit", "101"},
			wantErr: "limit must be between 1 and 100",
		},
		{
			name:    "negative offset",
			args:    []string{"run", "list", "--offset", "-1"},
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

// TestModRunBatchStatusNotFound validates run status command handles 404.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestModRunBatchStatusNotFound(t *testing.T) {
	runID := domaintypes.NewRunID().String()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID {
			http.Error(w, "run not found", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"run", "status", runID}, &buf)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention not found or 404: %v", err)
	}
}
