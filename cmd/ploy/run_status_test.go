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

func TestRunStatusPrintsSummary(t *testing.T) {
	t.Helper()

	runID := domaintypes.NewRunID()
	modID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	jobID := domaintypes.NewJobID()

	server := newRunStatusReportServer(t, runID, modID, specID, repoID, jobID)
	defer server.Close()

	useServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"run", "status", runID.String()}, &buf)
	if err != nil {
		t.Fatalf("run status error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Run: "+runID.String()) {
		t.Fatalf("expected output to contain run id; got %q", out)
	}
	if !strings.Contains(out, "Repo: github.com/acme/service main -> ploy/java17") {
		t.Fatalf("expected output to contain unified repo header; got %q", out)
	}
}

// RED gate for roadmap/reporting.md Phase 0:
// run status should migrate from summary lines to the unified report view.
func TestRunStatusReportTextGate(t *testing.T) {
	t.Helper()

	runID := domaintypes.NewRunID()
	modID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	jobID := domaintypes.NewJobID()

	server := newRunStatusReportServer(t, runID, modID, specID, repoID, jobID)
	defer server.Close()

	useServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"run", "status", runID.String()}, &buf)
	if err != nil {
		t.Fatalf("run status error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Mig Name:") {
		t.Fatalf("expected output to contain unified report field 'Mig Name', got %q", out)
	}
	if !strings.Contains(out, "State") {
		t.Fatalf("expected output to contain follow-style job table header, got %q", out)
	}
}

func newRunStatusReportServer(t *testing.T, runID domaintypes.RunID, migID domaintypes.MigID, specID domaintypes.SpecID, repoID domaintypes.MigRepoID, jobID domaintypes.JobID) *httptest.Server {
	t.Helper()

	diffID := "11111111-1111-1111-1111-111111111111"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String():
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         runID.String(),
				"status":     "running",
				"mig_id":     migID.String(),
				"spec_id":    specID.String(),
				"created_at": "2026-02-24T08:00:00Z",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/migs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"migs": []map[string]any{
					{
						"id":         migID.String(),
						"name":       "java17-upgrade",
						"spec_id":    specID.String(),
						"archived":   false,
						"created_at": "2026-02-24T07:30:00Z",
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String()+"/repos":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repos": []map[string]any{
					{
						"run_id":     runID.String(),
						"repo_id":    repoID.String(),
						"repo_url":   "https://github.com/acme/service.git",
						"base_ref":   "main",
						"target_ref": "ploy/java17",
						"status":     "Running",
						"attempt":    1,
						"created_at": "2026-02-24T08:01:00Z",
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id":  runID.String(),
				"repo_id": repoID.String(),
				"attempt": 1,
				"jobs": []map[string]any{
					{
						"job_id":      jobID.String(),
						"name":        "step-1",
						"job_type":    "step",
						"job_image":   "ghcr.io/acme/runner:1",
						"next_id":     nil,
						"node_id":     nil,
						"status":      "Running",
						"duration_ms": 1500,
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"diffs": []map[string]any{
					{
						"id":           diffID,
						"job_id":       jobID.String(),
						"created_at":   "2026-02-24T08:03:00Z",
						"gzipped_size": 128,
						"summary":      nil,
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

// RED gate for roadmap/reporting.md Phase 0:
// run status must provide JSON mode matching the unified report schema.
func TestRunStatusJSONGate(t *testing.T) {
	t.Helper()

	runID := domaintypes.NewRunID()
	modID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String() {
			now := time.Now()
			resp := struct {
				ID        string    `json:"id"`
				Status    string    `json:"status"`
				MigID     string    `json:"mig_id"`
				SpecID    string    `json:"spec_id"`
				CreatedAt time.Time `json:"created_at"`
				Counts    *struct {
					Total         int32  `json:"total"`
					Queued        int32  `json:"queued"`
					Running       int32  `json:"running"`
					Success       int32  `json:"success"`
					Fail          int32  `json:"fail"`
					Cancelled     int32  `json:"cancelled"`
					DerivedStatus string `json:"derived_status"`
				} `json:"repo_counts,omitempty"`
			}{
				ID:        runID.String(),
				Status:    "running",
				MigID:     modID.String(),
				SpecID:    specID.String(),
				CreatedAt: now,
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
	err := executeCmd([]string{"run", "status", "--json", runID.String()}, &buf)
	if err != nil {
		t.Fatalf("run status --json error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("expected valid JSON output, got %q (err=%v)", buf.String(), err)
	}
	if _, ok := parsed["mig_id"]; !ok {
		t.Fatalf("expected mig_id in JSON output, got %v", parsed)
	}
	if _, ok := parsed["mig_name"]; !ok {
		t.Fatalf("expected mig_name in JSON output, got %v", parsed)
	}
	if _, ok := parsed["repos"]; !ok {
		t.Fatalf("expected repos in JSON output, got %v", parsed)
	}
	if _, ok := parsed["runs"]; !ok {
		t.Fatalf("expected runs in JSON output, got %v", parsed)
	}
}
