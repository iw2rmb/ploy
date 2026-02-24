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
	firstPage := make([]map[string]any, 0, migListPageSize)
	for i := 0; i < migListPageSize; i++ {
		firstPage = append(firstPage, map[string]any{
			"id":         domaintypes.NewMigID().String(),
			"name":       "other",
			"archived":   false,
			"created_at": "2026-02-24T07:00:00Z",
		})
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String():
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         runID.String(),
				"status":     "Started",
				"mig_id":     migID.String(),
				"spec_id":    specID.String(),
				"created_at": "2026-02-24T08:00:00Z",
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/migs":
			offset := r.URL.Query().Get("offset")
			if got := r.URL.Query().Get("limit"); got != "100" {
				t.Fatalf("expected limit=100, got %q", got)
			}
			switch offset {
			case "0":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"migs": firstPage,
				})
			case "100":
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
			default:
				t.Fatalf("unexpected migs offset: %s", offset)
			}
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
		t.Fatalf("expected 1 repo report, got %d", len(report.Repos))
	}
	if len(report.Runs) != 1 {
		t.Fatalf("expected 1 run entry, got %d", len(report.Runs))
	}

	repo := report.Repos[0]
	if repo.PatchURL == "" {
		t.Fatalf("expected repo patch URL to be populated")
	}
	assertURL(t, repo.BuildLogURL, "/api/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/logs", nil)
	assertURL(t, repo.PatchURL, "/api/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs", map[string]string{
		"download": "true",
		"diff_id":  diffID2.String(),
	})

	runEntry := report.Runs[0]
	if len(runEntry.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(runEntry.Jobs))
	}

	job0 := runEntry.Jobs[0]
	assertURL(t, job0.BuildLogURL, "/api/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/logs", nil)
	assertURL(t, job0.PatchURL, "/api/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs", map[string]string{
		"download": "true",
		"diff_id":  diffID2.String(),
	})

	job1 := runEntry.Jobs[1]
	if strings.TrimSpace(job1.PatchURL) != "" {
		t.Fatalf("expected no patch URL for job without diffs, got %q", job1.PatchURL)
	}
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
				"spec_id":    specID.String(),
				"created_at": "2026-02-24T09:00:00Z",
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/migs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"migs": []map[string]any{
					{
						"id":         migID.String(),
						"name":       "empty-diffs",
						"archived":   false,
						"created_at": "2026-02-24T08:30:00Z",
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

	if len(report.Repos) != 1 || len(report.Runs) != 1 {
		t.Fatalf("expected 1 repo and 1 run entry, got repos=%d runs=%d", len(report.Repos), len(report.Runs))
	}
	if report.Repos[0].PatchURL != "" {
		t.Fatalf("expected empty repo patch URL, got %q", report.Repos[0].PatchURL)
	}
	if report.Runs[0].Jobs[0].PatchURL != "" {
		t.Fatalf("expected empty job patch URL, got %q", report.Runs[0].Jobs[0].PatchURL)
	}
	if report.Repos[0].BuildLogURL == "" || report.Runs[0].Jobs[0].BuildLogURL == "" {
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
				"spec_id":    specID.String(),
				"created_at": "2026-02-24T10:00:00Z",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/"+runID.String()+"/repos":
			_ = json.NewEncoder(w).Encode(map[string]any{"repos": []map[string]any{}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/migs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"migs": []map[string]any{
					{
						"id":         migID.String(),
						"name":       "no-repos",
						"archived":   false,
						"created_at": "2026-02-24T09:30:00Z",
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

	if report.Repos == nil {
		t.Fatal("expected repos slice to be non-nil")
	}
	if report.Runs == nil {
		t.Fatal("expected runs slice to be non-nil")
	}
	if len(report.Repos) != 0 || len(report.Runs) != 0 {
		t.Fatalf("expected empty repos/runs, got repos=%d runs=%d", len(report.Repos), len(report.Runs))
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
