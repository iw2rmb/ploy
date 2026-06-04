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

func TestRunSubmitSelectorCases(t *testing.T) {
	t.Setenv("USER", "test-user")

	specPath := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(specPath, []byte("steps:\n  - image: alpine:latest\n    command: echo hello\n"), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	localRepo := gitrepo.SetupWithRemote(t, "https://gitlab.example.com/acme/local.git")
	localSHA := gitrepo.RevParse(t, localRepo, "HEAD")
	remoteSHA := "0123456789abcdef0123456789abcdef01234567"

	tests := []struct {
		name            string
		selector        string
		resolveResponse map[string]any
		wantResolve     map[string]string
		wantRepoURL     string
		wantRef         string
		wantCommitSHA   string
	}{
		{
			name:     "remote ref selector",
			selector: "acme/service:feature/test",
			resolveResponse: map[string]any{
				"repo_url":   "https://gitlab.example.com/acme/service.git",
				"ref":        "feature/test",
				"ref_is_sha": false,
			},
			wantResolve: map[string]string{"selector": "acme/service", "ref": "feature/test"},
			wantRepoURL: "https://gitlab.example.com/acme/service.git",
			wantRef:     "feature/test",
		},
		{
			name:     "remote sha selector",
			selector: "acme/service:" + remoteSHA,
			resolveResponse: map[string]any{
				"repo_url":   "https://gitlab.example.com/acme/service.git",
				"ref":        remoteSHA,
				"ref_is_sha": true,
			},
			wantResolve:   map[string]string{"selector": "acme/service", "ref": remoteSHA},
			wantRepoURL:   "https://gitlab.example.com/acme/service.git",
			wantRef:       remoteSHA,
			wantCommitSHA: remoteSHA,
		},
		{
			name:          "local repo selector",
			selector:      localRepo,
			wantRepoURL:   "https://gitlab.example.com/acme/local.git",
			wantRef:       localSHA,
			wantCommitSHA: localSHA,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runID := domaintypes.NewRunID().String()
			migID := domaintypes.NewMigID().String()
			specID := domaintypes.NewSpecID().String()
			var capturedResolve map[string]any
			var capturedSubmit map[string]any

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve":
					if tc.wantResolve == nil {
						t.Fatalf("remote resolver should not be called for local selector")
					}
					if err := json.NewDecoder(r.Body).Decode(&capturedResolve); err != nil {
						t.Fatalf("decode resolve request: %v", err)
					}
					_ = json.NewEncoder(w).Encode(tc.resolveResponse)
				case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
					if err := json.NewDecoder(r.Body).Decode(&capturedSubmit); err != nil {
						t.Fatalf("decode submit request: %v", err)
					}
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"run_id":  runID,
						"mig_id":  migID,
						"spec_id": specID,
					})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			clienv.UseServerDescriptor(t, server.URL)

			var buf bytes.Buffer
			if err := executeCmd([]string{"run", specPath, tc.selector}, &buf); err != nil {
				t.Fatalf("run submit error: %v", err)
			}

			if tc.wantResolve != nil {
				if capturedResolve["selector"] != tc.wantResolve["selector"] || capturedResolve["ref"] != tc.wantResolve["ref"] {
					t.Fatalf("unexpected resolve request: %#v", capturedResolve)
				}
			}
			if capturedSubmit["repo_url"] != tc.wantRepoURL {
				t.Fatalf("repo_url = %v", capturedSubmit["repo_url"])
			}
			if capturedSubmit["ref"] != tc.wantRef {
				t.Fatalf("ref = %v", capturedSubmit["ref"])
			}
			if tc.wantCommitSHA == "" {
				if _, ok := capturedSubmit["commit_sha"]; ok {
					t.Fatalf("submit request must not contain commit_sha: %#v", capturedSubmit)
				}
			} else if capturedSubmit["commit_sha"] != tc.wantCommitSHA {
				t.Fatalf("commit_sha = %v", capturedSubmit["commit_sha"])
			}
			if _, ok := capturedSubmit["base_ref"]; ok {
				t.Fatalf("submit request must not contain base_ref: %#v", capturedSubmit)
			}
			if capturedSubmit["created_by"] != "test-user" {
				t.Fatalf("created_by = %v", capturedSubmit["created_by"])
			}
			if !strings.Contains(buf.String(), "run_id: "+runID) || !strings.Contains(buf.String(), "mig_id: "+migID) {
				t.Fatalf("unexpected output: %q", buf.String())
			}
		})
	}
}

func TestRunSubmitSpecDirectoryUsesMigYAML(t *testing.T) {
	specDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(specDir, "mig.yaml"), []byte("steps:\n  - image: alpine:latest\n    command: echo dir\n"), 0o644); err != nil {
		t.Fatalf("write mig.yaml: %v", err)
	}

	var capturedSubmit map[string]any
	runID := domaintypes.NewRunID().String()
	migID := domaintypes.NewMigID().String()
	specID := domaintypes.NewSpecID().String()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repo_url":   "https://gitlab.example.com/acme/service.git",
				"ref":        "master",
				"ref_is_sha": false,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			if err := json.NewDecoder(r.Body).Decode(&capturedSubmit); err != nil {
				t.Fatalf("decode submit request: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"run_id": runID, "mig_id": migID, "spec_id": specID})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	clienv.UseServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	if err := executeCmd([]string{"run", specDir, "acme/service"}, &buf); err != nil {
		t.Fatalf("run submit dir spec error: %v", err)
	}
	spec, ok := capturedSubmit["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec object, got %#v", capturedSubmit["spec"])
	}
	steps := spec["steps"].([]any)
	step := steps[0].(map[string]any)
	if step["command"] != "echo dir" {
		t.Fatalf("expected mig.yaml content, got %#v", step)
	}
}

func TestRunSubmitPullDownloadsFinalArtifacts(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(specPath, []byte("steps:\n  - image: alpine\n"), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}
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
	clienv.UseServerDescriptor(t, server.URL)

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

func TestRunSubmitFollowUsesStatusFollowRenderer(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(specPath, []byte("steps:\n  - image: alpine\n"), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}
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
	clienv.UseServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	if err := executeCmd([]string{"run", "--follow", specPath, "acme/service"}, &buf); err != nil {
		t.Fatalf("run --follow error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "acme/service") {
		t.Fatalf("expected final status snapshot, got %q", out)
	}
	if strings.Contains(out, "run_id: "+runID) {
		t.Fatalf("expected follow output without submit id prelude, got %q", out)
	}
}

func TestRunSubmitApplyAppliesFinalPatch(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(specPath, []byte("steps:\n  - image: alpine\n"), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}
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
	clienv.UseServerDescriptor(t, server.URL)

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
	Patch     []byte
}

func newSuccessfulRunSubmitServer(t *testing.T, cfg successfulRunSubmitConfig) *httptest.Server {
	t.Helper()
	diffID := "11111111-1111-1111-1111-111111111111"
	sourceSHA := cfg.SourceSHA
	if sourceSHA == "" {
		sourceSHA = "0123456789abcdef0123456789abcdef01234567"
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
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
				"status":            "Success",
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
				"state":  "succeeded",
				"stages": map[string]any{
					cfg.JobID: map[string]any{
						"state":     "succeeded",
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
					"status":    "Success",
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
