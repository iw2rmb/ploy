package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRunStatusReportTextContract(t *testing.T) {
	t.Helper()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	preGateID := domaintypes.NewJobID()
	healID := domaintypes.NewJobID()
	postGateID := domaintypes.NewJobID()

	server := newRunStatusReportServer(t, runID, migID, specID, repoID, preGateID, healID, postGateID)
	defer server.Close()

	useServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"run", "status", runID.String()}, &buf)
	if err != nil {
		t.Fatalf("run status error: %v", err)
	}

	out := buf.String()
	assertContains(t, out, "   Mig:   "+migID.String()+"   | java17-upgrade")
	assertContains(t, out, "   Spec:  "+specID.String()+" | Download ("+server.URL+"/v1/migs/"+migID.String()+"/specs/latest)")
	assertContains(t, out, "   Repos: 1")
	assertContains(t, out, "\n   Repos: 1\n   Run:   "+runID.String()+"\n\n")
	assertContains(t, out, "   [1/1] github.com/acme/service (https://github.com/acme/service.git) main -> ploy/java17")
	assertContains(t, out, "Artefacts")
	assertNotContains(t, out, "State")
	assertContains(t, out, "Logs")
	assertContains(t, out, " | Patch")
	if strings.Count(out, "Patch (") != 1 {
		t.Fatalf("expected exactly one patch link, got %q", out)
	}
	assertContains(t, out, "⣾")
	assertContains(t, out, "\x1b[91m✗\x1b[0m")
	assertContains(t, out, "\x1b[1;91m<infra>\x1b[0m └  Exit 137: \x1b[91mcompile failed at step 2\x1b[0m")
	assertContains(t, out, "└  Exit 0: Applied import fix and retried build")
}

func newRunStatusReportServer(t *testing.T, runID domaintypes.RunID, migID domaintypes.MigID, specID domaintypes.SpecID, repoID domaintypes.MigRepoID, preGateID domaintypes.JobID, healID domaintypes.JobID, postGateID domaintypes.JobID) *httptest.Server {
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
			lastErr := "compile\nfailed at step 2"
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
						"last_error": lastErr,
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
						"job_id":      preGateID.String(),
						"name":        "pre-gate",
						"job_type":    "pre_gate",
						"job_image":   "ghcr.io/acme/pre-gate:1",
						"next_id":     healID.String(),
						"node_id":     nil,
						"status":      "Failed",
						"exit_code":   137,
						"duration_ms": 1500,
						"recovery": map[string]any{
							"loop_kind":  "healing",
							"error_kind": "infra",
						},
					},
					{
						"job_id":         healID.String(),
						"name":           "heal-1-0",
						"job_type":       "heal",
						"job_image":      "ghcr.io/acme/heal:1",
						"next_id":        postGateID.String(),
						"node_id":        nil,
						"status":         "Success",
						"exit_code":      0,
						"duration_ms":    1200,
						"action_summary": "Applied import fix and retried build",
					},
					{
						"job_id":      postGateID.String(),
						"name":        "post-gate",
						"job_type":    "post_gate",
						"job_image":   "ghcr.io/acme/post-gate:1",
						"next_id":     nil,
						"node_id":     nil,
						"status":      "Running",
						"duration_ms": 600,
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"diffs": []map[string]any{
					{
						"id":           diffID,
						"job_id":       healID.String(),
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

func TestRunStatusJSONGate(t *testing.T) {
	t.Helper()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	preGateID := domaintypes.NewJobID()
	healID := domaintypes.NewJobID()
	postGateID := domaintypes.NewJobID()

	server := newRunStatusReportServer(t, runID, migID, specID, repoID, preGateID, healID, postGateID)
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
	for _, key := range []string{"mig_id", "mig_name", "repos", "runs"} {
		if _, ok := parsed[key]; !ok {
			t.Fatalf("expected %s in JSON output, got %v", key, parsed)
		}
	}
	if strings.Contains(buf.String(), "Mig:") {
		t.Fatalf("expected JSON output only, got text report marker in %q", buf.String())
	}
}

func assertContains(t *testing.T, output string, want string) {
	t.Helper()
	if !strings.Contains(output, want) {
		t.Fatalf("expected output to contain %q, got %q", want, output)
	}
}

func assertNotContains(t *testing.T, output string, want string) {
	t.Helper()
	if strings.Contains(output, want) {
		t.Fatalf("expected output to not contain %q, got %q", want, output)
	}
}
