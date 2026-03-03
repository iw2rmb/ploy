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

func TestGetRunReportCommandAssemblesCanonicalReport(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	jobID1 := domaintypes.NewJobID()
	jobID2 := domaintypes.NewJobID()
	diffID1 := domaintypes.DiffID("11111111-1111-1111-1111-111111111111")
	diffID2 := domaintypes.DiffID("22222222-2222-2222-2222-222222222222")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String():
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         runID.String(),
				"status":     "Started",
				"mig_id":     migID.String(),
				"mig_name":   "java17-upgrade",
				"spec_id":    specID.String(),
				"created_at": "2026-02-24T08:00:00Z",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String()+"/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id": runID.String(),
				"state":  "running",
				"stages": map[string]any{
					jobID1.String(): map[string]any{
						"state":        "succeeded",
						"attempts":     1,
						"max_attempts": 1,
						"artifacts": map[string]any{
							"diff": "bafy-step-1",
						},
					},
					jobID2.String(): map[string]any{
						"state":        "running",
						"attempts":     1,
						"max_attempts": 1,
						"artifacts": map[string]any{
							"logs": "bafy-step-2",
						},
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String()+"/repos":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repos": []map[string]any{
					{
						"run_id":      runID.String(),
						"repo_id":     repoID.String(),
						"repo_url":    "https://github.com/acme/service.git",
						"base_ref":    "main",
						"target_ref":  "ploy/java17",
						"status":      "Running",
						"attempt":     2,
						"last_error":  "build failed",
						"created_at":  "2026-02-24T08:01:00Z",
						"started_at":  "2026-02-24T08:02:00Z",
						"finished_at": nil,
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs":
			if got := r.URL.Query().Get("attempt"); got != "2" {
				t.Fatalf("expected attempt=2, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id":  runID.String(),
				"repo_id": repoID.String(),
				"attempt": 2,
				"jobs": []map[string]any{
					{
						"job_id":       jobID1.String(),
						"name":         "step-1",
						"job_type":     "step",
						"job_image":    "ghcr.io/acme/runner:1",
						"next_id":      jobID2.String(),
						"node_id":      nil,
						"status":       "Success",
						"duration_ms":  50,
						"display_name": "scan",
					},
					{
						"job_id":       jobID2.String(),
						"name":         "step-2",
						"job_type":     "step",
						"job_image":    "ghcr.io/acme/runner:1",
						"next_id":      nil,
						"node_id":      nil,
						"status":       "Running",
						"duration_ms":  5,
						"display_name": "apply",
						"recovery": map[string]any{
							"loop_kind":  "healing",
							"error_kind": "infra",
						},
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"diffs": []map[string]any{
					{
						"id":           diffID1.String(),
						"job_id":       jobID1.String(),
						"created_at":   "2026-02-24T08:03:00Z",
						"gzipped_size": 128,
						"summary":      nil,
					},
					{
						"id":           diffID2.String(),
						"job_id":       jobID1.String(),
						"created_at":   "2026-02-24T08:04:00Z",
						"gzipped_size": 256,
						"summary":      nil,
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	baseURL, err := url.Parse(server.URL + "/api")
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}

	report, err := GetRunReportCommand{
		Client:  server.Client(),
		BaseURL: baseURL,
		RunID:   runID,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("GetRunReportCommand.Run error: %v", err)
	}

	if report.RunID != runID {
		t.Fatalf("unexpected run id: got %s, want %s", report.RunID, runID)
	}
	if report.MigName != "java17-upgrade" {
		t.Fatalf("unexpected mig name: %q", report.MigName)
	}
	if len(report.Repos) != 1 {
		t.Fatalf("expected 1 repo entry, got %d", len(report.Repos))
	}

	entry := report.Repos[0]
	if entry.PatchURL == "" {
		t.Fatalf("expected repo patch URL to be populated")
	}
	assertURL(t, entry.BuildLogURL, "/api/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/logs", nil)
	assertURL(t, entry.PatchURL, "/api/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs", map[string]string{
		"download": "true",
		"diff_id":  diffID2.String(),
	})

	if len(entry.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(entry.Jobs))
	}

	job0 := entry.Jobs[0]
	assertURL(t, job0.BuildLogURL, "/api/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/logs", nil)
	assertURL(t, job0.PatchURL, "/api/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs", map[string]string{
		"download": "true",
		"diff_id":  diffID2.String(),
	})
	if len(job0.Artifacts) != 1 {
		t.Fatalf("expected one artifact for job0, got %d", len(job0.Artifacts))
	}
	if job0.Artifacts[0].Name != "diff" || job0.Artifacts[0].CID != "bafy-step-1" {
		t.Fatalf("unexpected job0 artifact payload: %#v", job0.Artifacts[0])
	}
	assertURL(t, job0.Artifacts[0].LookupURL, "/api/v1/artifacts", map[string]string{
		"cid": "bafy-step-1",
	})

	job1 := entry.Jobs[1]
	if strings.TrimSpace(job1.PatchURL) != "" {
		t.Fatalf("expected no patch URL for job without diffs, got %q", job1.PatchURL)
	}
	if job1.Recovery == nil || job1.Recovery.ErrorKind != "infra" {
		t.Fatalf("expected recovery.error_kind to propagate, got %#v", job1.Recovery)
	}
	if len(job1.Artifacts) != 1 {
		t.Fatalf("expected one artifact for job1, got %d", len(job1.Artifacts))
	}
	if job1.Artifacts[0].Name != "logs" || job1.Artifacts[0].CID != "bafy-step-2" {
		t.Fatalf("unexpected job1 artifact payload: %#v", job1.Artifacts[0])
	}
	assertURL(t, job1.Artifacts[0].LookupURL, "/api/v1/artifacts", map[string]string{
		"cid": "bafy-step-2",
	})
}

func TestGetRunReportCommandMissingOptionalFields(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	jobID := domaintypes.NewJobID()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String():
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         runID.String(),
				"status":     "Started",
				"mig_id":     migID.String(),
				"mig_name":   "empty-diffs",
				"spec_id":    specID.String(),
				"created_at": "2026-02-24T09:00:00Z",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String()+"/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id": runID.String(),
				"state":  "queued",
				"stages": map[string]any{},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String()+"/repos":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repos": []map[string]any{
					{
						"run_id":     runID.String(),
						"repo_id":    repoID.String(),
						"repo_url":   "https://github.com/acme/empty.git",
						"base_ref":   "main",
						"target_ref": "ploy/empty",
						"status":     "Queued",
						"attempt":    1,
						"created_at": "2026-02-24T09:01:00Z",
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/jobs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id":  runID.String(),
				"repo_id": repoID.String(),
				"attempt": 1,
				"jobs": []map[string]any{
					{
						"job_id":      jobID.String(),
						"name":        "step",
						"job_type":    "step",
						"job_image":   "",
						"next_id":     nil,
						"node_id":     nil,
						"status":      "Queued",
						"duration_ms": 0,
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs":
			_ = json.NewEncoder(w).Encode(map[string]any{"diffs": []map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	baseURL, err := url.Parse(server.URL + "/api")
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}

	report, err := GetRunReportCommand{
		Client:  server.Client(),
		BaseURL: baseURL,
		RunID:   runID,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("GetRunReportCommand.Run error: %v", err)
	}

	if len(report.Repos) != 1 {
		t.Fatalf("expected 1 repo entry, got %d", len(report.Repos))
	}
	if report.Repos[0].PatchURL != "" {
		t.Fatalf("expected empty repo patch URL, got %q", report.Repos[0].PatchURL)
	}
	if report.Repos[0].Jobs[0].PatchURL != "" {
		t.Fatalf("expected empty job patch URL, got %q", report.Repos[0].Jobs[0].PatchURL)
	}
	if report.Repos[0].BuildLogURL == "" || report.Repos[0].Jobs[0].BuildLogURL == "" {
		t.Fatalf("expected build log URLs to be populated")
	}
}

func TestGetRunReportCommandEmptyReposUsesEmptySlices(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String():
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         runID.String(),
				"status":     "Started",
				"mig_id":     migID.String(),
				"mig_name":   "no-repos",
				"spec_id":    specID.String(),
				"created_at": "2026-02-24T10:00:00Z",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String()+"/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id": runID.String(),
				"state":  "running",
				"stages": map[string]any{},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String()+"/repos":
			_ = json.NewEncoder(w).Encode(map[string]any{"repos": []map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	baseURL, err := url.Parse(server.URL + "/api")
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}

	report, err := GetRunReportCommand{
		Client:  server.Client(),
		BaseURL: baseURL,
		RunID:   runID,
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("GetRunReportCommand.Run error: %v", err)
	}

	if report.Repos == nil {
		t.Fatal("expected repos slice to be non-nil")
	}
	if len(report.Repos) != 0 {
		t.Fatalf("expected empty repos, got %d", len(report.Repos))
	}
}

func TestGetRunReportCommandValidation(t *testing.T) {
	t.Parallel()

	baseURL, err := url.Parse("http://127.0.0.1:12345")
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}

	tests := []struct {
		name    string
		cmd     GetRunReportCommand
		wantErr string
	}{
		{
			name: "missing client",
			cmd: GetRunReportCommand{
				BaseURL: baseURL,
				RunID:   domaintypes.NewRunID(),
			},
			wantErr: "http client required",
		},
		{
			name: "missing base url",
			cmd: GetRunReportCommand{
				Client: http.DefaultClient,
				RunID:  domaintypes.NewRunID(),
			},
			wantErr: "base url required",
		},
		{
			name: "missing run id",
			cmd: GetRunReportCommand{
				Client:  http.DefaultClient,
				BaseURL: baseURL,
			},
			wantErr: "run id required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.cmd.Run(context.Background())
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error %q to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func assertURL(t *testing.T, rawURL string, wantPath string, wantQuery map[string]string) {
	t.Helper()

	if strings.TrimSpace(rawURL) == "" {
		t.Fatalf("url is empty")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse URL %q: %v", rawURL, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		t.Fatalf("expected absolute URL, got %q", rawURL)
	}
	if parsed.Path != wantPath {
		t.Fatalf("unexpected path: got %q, want %q", parsed.Path, wantPath)
	}

	for key, want := range wantQuery {
		got := parsed.Query().Get(key)
		if got != want {
			t.Fatalf("unexpected query value for %q: got %q, want %q", key, got, want)
		}
	}
}
