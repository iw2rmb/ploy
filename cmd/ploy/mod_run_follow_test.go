package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// End-to-end happy path for 3.1: submit, follow job graph, download artifacts.
func TestModRunFollowStreamsAndDownloadsArtifacts(t *testing.T) {
	t.Helper()

	runID := domaintypes.NewRunID().String()
	repoID := domaintypes.NewModRepoID().String()
	artifactCID := "bafy-artifact-test"
	stageID := domaintypes.NewJobID()
	artifactID := "11111111-1111-1111-1111-111111111111"
	artifactDigest := "sha256:deadbeef" + strings.Repeat("0", 56)

	// Minimal control-plane emulator.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			var req modsapi.RunSubmitRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			// Server returns 201 Created with {run_id, mod_id, spec_id}.
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(struct {
				RunID  string `json:"run_id"`
				ModID  string `json:"mod_id"`
				SpecID string `json:"spec_id"`
			}{RunID: runID, ModID: domaintypes.NewModID().String(), SpecID: domaintypes.NewSpecID().String()})

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/repos", runID):
			// Return repos list for follow engine.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"repos": []map[string]interface{}{
					{
						"repo_id":    repoID,
						"repo_url":   "https://example.com/repo.git",
						"base_ref":   "main",
						"target_ref": "feature",
						"status":     "Running",
						"attempt":    1,
					},
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/repos/%s/jobs", runID, repoID):
			// Return jobs for repo.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"run_id":  runID,
				"repo_id": repoID,
				"attempt": 1,
				"jobs": []map[string]interface{}{
					{"job_id": stageID.String(), "name": "mod-0", "job_type": "mod", "next_id": 2000, "status": "Success", "duration_ms": 100},
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/logs", runID):
			// SSE stream: run running -> run succeeded
			w.Header().Set("Content-Type", "text/event-stream")
			fl, ok := w.(http.Flusher)
			if !ok {
				t.Fatalf("no flusher")
			}
			// running
			_, _ = w.Write([]byte("event: run\n"))
			data, _ := json.Marshal(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateRunning,
			})
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
			fl.Flush()
			time.Sleep(5 * time.Millisecond)
			// succeeded
			_, _ = w.Write([]byte("event: run\n"))
			data2, _ := json.Marshal(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateSucceeded,
			})
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data2)
			_, _ = w.Write([]byte("\n\n"))
			fl.Flush()

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/status", runID):
			// Return RunSummary directly — the canonical response shape.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateSucceeded,
				Stages: map[domaintypes.JobID]modsapi.StageStatus{
					stageID: {State: modsapi.StageStateSucceeded, Artifacts: map[string]string{"diff": artifactCID}},
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/v1/artifacts":
			if q := r.URL.Query().Get("cid"); q != artifactCID {
				t.Fatalf("unexpected artifact lookup cid: %q", q)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"artifacts":[{"id":"` + artifactID + `","cid":"` + artifactCID + `","digest":"` + artifactDigest + `","name":"plan-diff.tar.gz","size":10}]}`))

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/artifacts/"+artifactID):
			// Download bytes
			_, _ = w.Write([]byte("artifact-bytes"))

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	dir := t.TempDir()
	buf := &bytes.Buffer{}
	args := []string{
		"--repo-url", "https://example.com/repo.git",
		"--repo-base-ref", "main",
		"--repo-target-ref", "feature",
		"--follow",
		"--artifact-dir", dir,
	}
	if err := executeModRun(args, buf); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}

	// Output should at least acknowledge submission and success.
	out := buf.String()
	if !strings.Contains(out, "submitted") {
		t.Fatalf("expected submission message, got: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "succeeded") {
		t.Fatalf("expected success in output, got: %s", out)
	}

	// An artifact should be written and a manifest.json produced.
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var hasManifest, hasArtifact bool
	for _, f := range files {
		if f.Name() == "manifest.json" {
			hasManifest = true
		}
		if strings.Contains(f.Name(), "deadbeef") || strings.Contains(f.Name(), artifactCID) {
			hasArtifact = true
		}
	}
	if !hasManifest {
		t.Fatalf("manifest.json not found in %s; files=%v", dir, list(dir))
	}
	if !hasArtifact {
		t.Fatalf("artifact file not found in %s; files=%v", dir, list(dir))
	}
}

func list(dir string) []string {
	entries, _ := os.ReadDir(dir)
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

// TestModRunFollowShowsJobGraph verifies that mod run --follow displays
// the job graph with repo URL, job type, status, and display name.
// This replaces the old log streaming test since --follow now shows job graphs.
func TestModRunFollowShowsJobGraph(t *testing.T) {
	runID := domaintypes.NewRunID().String()
	repoID := domaintypes.NewModRepoID().String()
	jobID := domaintypes.NewJobID().String()

	// Control-plane emulator.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			// Server returns 201 Created with {run_id, mod_id, spec_id}.
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(struct {
				RunID  string `json:"run_id"`
				ModID  string `json:"mod_id"`
				SpecID string `json:"spec_id"`
			}{RunID: runID, ModID: domaintypes.NewModID().String(), SpecID: domaintypes.NewSpecID().String()})

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/status", runID):
			// Minimal RunSummary status response used by the submit command.
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateRunning,
			})

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/repos", runID):
			// Return repos list for follow engine.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"repos": []map[string]interface{}{
					{
						"repo_id":    repoID,
						"repo_url":   "https://example.com/repo.git",
						"base_ref":   "main",
						"target_ref": "feature",
						"status":     "Running",
						"attempt":    1,
					},
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/repos/%s/jobs", runID, repoID):
			// Return jobs for repo with display_name.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"run_id":  runID,
				"repo_id": repoID,
				"attempt": 1,
				"jobs": []map[string]interface{}{
					{"job_id": jobID, "name": "mod-0", "job_type": "mod", "next_id": 2000, "status": "Running", "duration_ms": 0, "display_name": "java17-upgrade"},
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/logs", runID):
			// SSE stream: run running -> run succeeded.
			w.Header().Set("Content-Type", "text/event-stream")
			fl, ok := w.(http.Flusher)
			if !ok {
				t.Fatalf("no flusher")
			}

			// Run running event.
			_, _ = w.Write([]byte("event: run\n"))
			runData, _ := json.Marshal(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateRunning,
			})
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(runData)
			_, _ = w.Write([]byte("\n\n"))
			fl.Flush()

			time.Sleep(5 * time.Millisecond)

			// Run succeeded event.
			_, _ = w.Write([]byte("event: run\n"))
			runData2, _ := json.Marshal(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateSucceeded,
			})
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(runData2)
			_, _ = w.Write([]byte("\n\n"))
			fl.Flush()

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	args := []string{
		"--repo-url", "https://example.com/repo.git",
		"--repo-base-ref", "main",
		"--repo-target-ref", "feature",
		"--follow",
	}
	if err := executeModRun(args, buf); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}

	out := buf.String()

	// Verify run state messages are present.
	if !strings.Contains(out, "submitted") {
		t.Errorf("expected submission message, got: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "succeeded") {
		t.Errorf("expected success in output, got: %s", out)
	}

	// Verify job graph output contains repo URL (scheme-less normalized) and job info.
	if !strings.Contains(out, "Repos: 1") {
		t.Errorf("expected repo count line in output, got: %s", out)
	}
	if !strings.Contains(out, "Repo 1/1: example.com/repo") {
		t.Errorf("expected repo block header in output, got: %s", out)
	}
	if !strings.Contains(out, "Step") || !strings.Contains(out, "Node") {
		t.Errorf("expected new follow header columns in output, got: %s", out)
	}
	if strings.Contains(out, "Index") || strings.Contains(out, "Status") || strings.Contains(out, "NodeID") {
		t.Errorf("expected legacy follow columns to be removed, got: %s", out)
	}
	if !strings.Contains(out, "example.com/repo") {
		t.Errorf("expected repo URL in output, got: %s", out)
	}
	if !strings.Contains(out, "mod") {
		t.Errorf("expected mod type in output, got: %s", out)
	}
}

// TestModRunFollowWithMultipleJobs verifies that mod run --follow displays
// all jobs in the job graph when a run has multiple jobs.
func TestModRunFollowWithMultipleJobs(t *testing.T) {
	runID := domaintypes.NewRunID().String()
	repoID := domaintypes.NewModRepoID().String()
	preGateJobID := domaintypes.NewJobID().String()
	modJobID := domaintypes.NewJobID().String()
	postGateJobID := domaintypes.NewJobID().String()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			// Server returns 201 Created with {run_id, mod_id, spec_id}.
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(struct {
				RunID  string `json:"run_id"`
				ModID  string `json:"mod_id"`
				SpecID string `json:"spec_id"`
			}{RunID: runID, ModID: domaintypes.NewModID().String(), SpecID: domaintypes.NewSpecID().String()})

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/status", runID):
			// Minimal RunSummary status response used by the submit command.
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateRunning,
			})

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/repos", runID):
			// Return repos list for follow engine.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"repos": []map[string]interface{}{
					{
						"repo_id":    repoID,
						"repo_url":   "https://example.com/repo.git",
						"base_ref":   "main",
						"target_ref": "feature",
						"status":     "Running",
						"attempt":    1,
					},
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/repos/%s/jobs", runID, repoID):
			// Return multiple jobs for repo.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"run_id":  runID,
				"repo_id": repoID,
				"attempt": 1,
				"jobs": []map[string]interface{}{
					{"job_id": preGateJobID, "name": "pre-gate", "job_type": "pre_gate", "next_id": 1000, "status": "Success", "duration_ms": 50},
					{"job_id": modJobID, "name": "mod-0", "job_type": "mod", "next_id": 2000, "status": "Running", "duration_ms": 0},
					{"job_id": postGateJobID, "name": "post-gate", "job_type": "post_gate", "next_id": 3000, "status": "Created", "duration_ms": 0},
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/logs", runID):
			w.Header().Set("Content-Type", "text/event-stream")
			fl, ok := w.(http.Flusher)
			if !ok {
				t.Fatalf("no flusher")
			}

			// Run running.
			_, _ = w.Write([]byte("event: run\n"))
			runData, _ := json.Marshal(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateRunning,
			})
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(runData)
			_, _ = w.Write([]byte("\n\n"))
			fl.Flush()

			time.Sleep(5 * time.Millisecond)

			// Run succeeded.
			_, _ = w.Write([]byte("event: run\n"))
			runData2, _ := json.Marshal(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateSucceeded,
			})
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(runData2)
			_, _ = w.Write([]byte("\n\n"))
			fl.Flush()

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	args := []string{
		"--repo-url", "https://example.com/repo.git",
		"--repo-base-ref", "main",
		"--repo-target-ref", "feature",
		"--follow",
	}
	if err := executeModRun(args, buf); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}

	out := buf.String()

	// Verify run submission and success.
	if !strings.Contains(out, "submitted") {
		t.Errorf("expected submission message, got: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "succeeded") {
		t.Errorf("expected success in output, got: %s", out)
	}

	// Verify job types appear in output.
	if !strings.Contains(out, "pre_gate") {
		t.Errorf("expected pre_gate in output, got: %s", out)
	}
	if !strings.Contains(out, "post_gate") {
		t.Errorf("expected post_gate in output, got: %s", out)
	}
}
