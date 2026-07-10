package app

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
)

func TestRunSubmitPullDownloadsFinalArtifacts(t *testing.T) {
	specPath := writeRunSubmitSpec(t, "steps:\n  - image: alpine\n")
	artifactDir := filepath.Join(t.TempDir(), "artifacts")
	runID := domaintypes.NewRunID().String()
	server := newSuccessfulRunSubmitServer(t, successfulRunSubmitConfig{
		RunID:   runID,
		MigID:   domaintypes.NewMigID().String(),
		SpecID:  domaintypes.NewSpecID().String(),
		RepoID:  domaintypes.NewRepoID().String(),
		JobID:   domaintypes.NewJobID().String(),
		RepoURL: "https://gitlab.example.com/acme/service.git",
		Ref:     "main",
	})
	defer server.Close()
	clienv.UseControlPlaneEnv(t, server.URL)

	var buf bytes.Buffer
	if err := executeCmd([]string{"run", "--pull=" + artifactDir, specPath, "acme/service"}, &buf); err != nil {
		t.Fatalf("run --pull error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(artifactDir, "manifest.json")); err != nil {
		t.Fatalf("expected manifest.json to be written: %v", err)
	}
	if !strings.Contains(buf.String(), "Downloaded 0 artifacts to "+artifactDir) {
		t.Fatalf("expected artifact download output, got %q", buf.String())
	}
}

func TestRunSubmitFollowOutputCases(t *testing.T) {
	tests := []struct {
		name         string
		specArg      func(t *testing.T) string
		configure    func(*successfulRunSubmitConfig)
		wantContain  string
		wantNoSpecID bool
	}{
		{
			name: "local spec uses status follow renderer",
			specArg: func(t *testing.T) string {
				return writeRunSubmitSpec(t, "steps:\n  - image: alpine\n")
			},
			wantContain: "acme/service",
		},
		{
			name:    "named spec displays source reference instead of spec id",
			specArg: func(t *testing.T) string { return "upgrade-java" },
			configure: func(cfg *successfulRunSubmitConfig) {
				cfg.NamedSpecName = "upgrade-java"
				cfg.NamedSpecDomain = "gitlab.example.com"
				cfg.NamedSpecRepo = "acme/specs"
			},
			wantContain:  "Spec:  gitlab.example.com/acme/specs:upgrade-java",
			wantNoSpecID: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			specID := domaintypes.NewSpecID().String()
			runID := domaintypes.NewRunID().String()
			cfg := successfulRunSubmitConfig{
				RunID:   runID,
				MigID:   domaintypes.NewMigID().String(),
				SpecID:  specID,
				RepoID:  domaintypes.NewRepoID().String(),
				JobID:   domaintypes.NewJobID().String(),
				RepoURL: "https://gitlab.example.com/acme/service.git",
				Ref:     "main",
			}
			if tc.configure != nil {
				tc.configure(&cfg)
			}
			server := newSuccessfulRunSubmitServer(t, cfg)
			defer server.Close()
			clienv.UseControlPlaneEnv(t, server.URL)

			var buf bytes.Buffer
			if err := executeCmd([]string{"run", "--follow", tc.specArg(t), "acme/service"}, &buf); err != nil {
				t.Fatalf("run --follow error: %v", err)
			}
			out := buf.String()
			if !strings.Contains(out, tc.wantContain) {
				t.Fatalf("expected %q in follow output, got %q", tc.wantContain, out)
			}
			if tc.wantNoSpecID && strings.Contains(out, "Spec:  "+specID) {
				t.Fatalf("expected named spec reference instead of spec id, got %q", out)
			}
			if strings.Contains(out, "run_id: "+runID) {
				t.Fatalf("expected follow output without submit id prelude, got %q", out)
			}
		})
	}
}

func TestRunSubmitApplyAppliesFinalPatch(t *testing.T) {
	specPath := writeRunSubmitSpec(t, "steps:\n  - image: alpine\n")
	repoDir := gitrepo.SetupWithRemote(t, "https://gitlab.example.com/acme/service.git")
	sourceSHA := gitrepo.RevParse(t, repoDir, "HEAD")
	patch := []byte("diff --git a/README.md b/README.md\nindex 5b4f9e0..98a5560 100644\n--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-# Test Repo\n+# Applied From Submit\n")

	runID := domaintypes.NewRunID().String()
	server := newSuccessfulRunSubmitServer(t, successfulRunSubmitConfig{
		RunID:     runID,
		MigID:     domaintypes.NewMigID().String(),
		SpecID:    domaintypes.NewSpecID().String(),
		RepoID:    domaintypes.NewRepoID().String(),
		JobID:     domaintypes.NewJobID().String(),
		RepoURL:   "https://gitlab.example.com/acme/service.git",
		Ref:       sourceSHA,
		SourceSHA: sourceSHA,
		Patch:     patch,
	})
	defer server.Close()
	clienv.UseControlPlaneEnv(t, server.URL)

	var buf bytes.Buffer
	if err := executeCmd([]string{"run", "--apply", specPath, repoDir}, &buf); err != nil {
		t.Fatalf("run --apply error: %v", err)
	}
	gitrepo.AssertFileContent(t, filepath.Join(repoDir, "README.md"), "# Applied From Submit\n")
	if !strings.Contains(buf.String(), "Applied patch from run "+runID) {
		t.Fatalf("expected apply output, got %q", buf.String())
	}
}

type successfulRunSubmitConfig struct {
	RunID     string
	MigID     string
	SpecID    string
	RepoID    string
	JobID     string
	RepoURL   string
	Ref       string
	SourceSHA string
	RunStatus string
	RunState  string
	JobStatus string
	Patch     []byte

	NamedSpecName   string
	NamedSpecDomain string
	NamedSpecRepo   string
}

func newSuccessfulRunSubmitServer(t *testing.T, cfg successfulRunSubmitConfig) *httptest.Server {
	t.Helper()
	diffID := "11111111-1111-1111-1111-111111111111"
	sourceSHA := cfg.SourceSHA
	if sourceSHA == "" {
		sourceSHA = "0123456789abcdef0123456789abcdef01234567"
	}
	runStatus := cfg.RunStatus
	if runStatus == "" {
		runStatus = domaintypes.RunStatusSuccess.String()
	}
	runState := cfg.RunState
	if runState == "" {
		runState = stageStateForJobStatus(runStatus)
	}
	jobStatus := cfg.JobStatus
	if jobStatus == "" {
		jobStatus = runStatus
	}
	jobState := stageStateForJobStatus(jobStatus)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/specs/resolve" && cfg.NamedSpecName != "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":   cfg.NamedSpecName,
				"source": map[string]string{"domain": cfg.NamedSpecDomain, "repo": cfg.NamedSpecRepo},
				"spec": map[string]any{
					"steps": []map[string]any{{
						"image":   "alpine:latest",
						"command": "echo named",
					}},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repo_url":   cfg.RepoURL,
				"ref":        cfg.Ref,
				"ref_is_sha": false,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"run_id":  cfg.RunID,
				"mig_id":  cfg.MigID,
				"spec_id": cfg.SpecID,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+cfg.RunID:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                cfg.RunID,
				"status":            runStatus,
				"mig_id":            cfg.MigID,
				"spec_id":           cfg.SpecID,
				"repo_id":           cfg.RepoID,
				"repo_url":          cfg.RepoURL,
				"base_ref":          cfg.Ref,
				"source_commit_sha": sourceSHA,
				"attempt":           1,
				"created_at":        "2026-05-28T00:00:00Z",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+cfg.RunID+"/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id": cfg.RunID,
				"state":  runState,
				"stages": map[string]any{
					cfg.JobID: map[string]any{
						"state":     jobState,
						"artifacts": map[string]string{},
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+cfg.RunID+"/jobs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"run_id":  cfg.RunID,
				"repo_id": cfg.RepoID,
				"attempt": 1,
				"jobs": []map[string]any{{
					"job_id":    cfg.JobID,
					"job_type":  "mig",
					"job_image": "alpine",
					"status":    jobStatus,
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/"+cfg.RunID+"/pull":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"run_id":            cfg.RunID,
				"repo_id":           cfg.RepoID,
				"repo_url":          cfg.RepoURL,
				"source_commit_sha": sourceSHA,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+cfg.RunID+"/diffs" && r.URL.Query().Get("download") != "true":
			diffs := []map[string]any{}
			if len(cfg.Patch) > 0 {
				diffs = append(diffs, map[string]any{
					"id":           diffID,
					"job_id":       cfg.JobID,
					"created_at":   "2026-05-28T00:00:00Z",
					"gzipped_size": len(cfg.Patch),
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"diffs": diffs})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+cfg.RunID+"/diffs" && r.URL.Query().Get("download") == "true":
			w.Header().Set("Content-Type", "application/gzip")
			gz := gzip.NewWriter(w)
			_, _ = gz.Write(cfg.Patch)
			_ = gz.Close()
		default:
			http.NotFound(w, r)
		}
	}))
}

func stageStateForJobStatus(status string) string {
	switch status {
	case domaintypes.JobStatusSuccess.String():
		return "succeeded"
	case domaintypes.JobStatusFail.String():
		return "failed"
	case domaintypes.JobStatusCancelled.String():
		return "cancelled"
	case domaintypes.JobStatusRunning.String():
		return "running"
	case domaintypes.JobStatusQueued.String():
		return "queued"
	case domaintypes.JobStatusCreated.String():
		return "created"
	default:
		return strings.ToLower(status)
	}
}
