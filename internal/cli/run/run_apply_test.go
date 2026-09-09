package run

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
)

func TestRunApplyAppliesAccumulatedPatch(t *testing.T) {
	repoDir := gitrepo.SetupWithRemote(t, "https://gitlab.example.com/acme/service.git")
	runID := domaintypes.NewRunID().String()
	repoID := domaintypes.NewRepoID().String()
	jobID := domaintypes.NewJobID().String()
	sourceSHA := gitrepo.RevParse(t, repoDir, "HEAD")
	patch := []byte("diff --git a/README.md b/README.md\nindex 5b4f9e0..98a5560 100644\n--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-# Test Repo\n+# Updated Repo\n")

	server := newRunApplyServer(t, runID, repoID, jobID, sourceSHA, patch)
	defer server.Close()
	clienv.UseControlPlaneEnv(t, server.URL)

	var out bytes.Buffer
	if err := RunApply(context.Background(), ApplyOptions{RunID: runID, RepoPath: repoDir, Output: &out}); err != nil {
		t.Fatalf("RunApply: %v", err)
	}
	gitrepo.AssertFileContent(t, filepath.Join(repoDir, "README.md"), "# Updated Repo\n")
	if !strings.Contains(out.String(), "Applied patch from run "+runID) {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRunApplyAcceptsEquivalentSSHAndHTTPSOrigins(t *testing.T) {
	repoDir := gitrepo.SetupWithRemote(t, "ssh://git@gitlab.example.com:7999/acme/service.git")
	runID := domaintypes.NewRunID().String()
	repoID := domaintypes.NewRepoID().String()
	jobID := domaintypes.NewJobID().String()
	sourceSHA := gitrepo.RevParse(t, repoDir, "HEAD")
	patch := []byte("diff --git a/README.md b/README.md\nindex 5b4f9e0..98a5560 100644\n--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-# Test Repo\n+# Updated Repo\n")
	server := newRunApplyServer(t, runID, repoID, jobID, sourceSHA, patch)
	defer server.Close()
	clienv.UseControlPlaneEnv(t, server.URL)

	if err := RunApply(context.Background(), ApplyOptions{RunID: runID, RepoPath: repoDir}); err != nil {
		t.Fatalf("RunApply: %v", err)
	}
	gitrepo.AssertFileContent(t, filepath.Join(repoDir, "README.md"), "# Updated Repo\n")
}

func TestRunApplyRejectsDifferentRepositoryWithoutForce(t *testing.T) {
	repoDir := gitrepo.SetupWithRemote(t, "ssh://git@gitlab.example.com:7999/acme/other.git")
	runID := domaintypes.NewRunID().String()
	repoID := domaintypes.NewRepoID().String()
	jobID := domaintypes.NewJobID().String()
	sourceSHA := gitrepo.RevParse(t, repoDir, "HEAD")
	server := newRunApplyServer(t, runID, repoID, jobID, sourceSHA, nil)
	defer server.Close()
	clienv.UseControlPlaneEnv(t, server.URL)

	err := RunApply(context.Background(), ApplyOptions{RunID: runID, RepoPath: repoDir})
	if err == nil || !strings.Contains(err.Error(), "does not match run metadata repo_url") {
		t.Fatalf("expected repository mismatch error, got %v", err)
	}
}

func TestRunApplyRejectsDirtyIndexWithoutForce(t *testing.T) {
	repoDir := gitrepo.SetupWithRemote(t, "https://gitlab.example.com/acme/service.git")
	gitrepo.WriteFile(t, filepath.Join(repoDir, "README.md"), "# staged\n")
	gitrepo.Run(t, repoDir, "add", "README.md")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be called for dirty repo: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()
	clienv.UseControlPlaneEnv(t, server.URL)

	err := RunApply(context.Background(), ApplyOptions{RunID: "run-123", RepoPath: repoDir})
	if err == nil || !strings.Contains(err.Error(), "working tree must have no staged or unstaged diff") {
		t.Fatalf("expected dirty worktree error, got %v", err)
	}
}

func TestRunApplyRejectsSHAMismatchWithoutForce(t *testing.T) {
	repoDir := gitrepo.SetupWithRemote(t, "https://gitlab.example.com/acme/service.git")
	runID := domaintypes.NewRunID().String()
	repoID := domaintypes.NewRepoID().String()
	jobID := domaintypes.NewJobID().String()
	patch := []byte("diff --git a/README.md b/README.md\nindex 5b4f9e0..98a5560 100644\n--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-# Test Repo\n+# Forced Repo\n")
	server := newRunApplyServer(t, runID, repoID, jobID, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", patch)
	defer server.Close()
	clienv.UseControlPlaneEnv(t, server.URL)

	err := RunApply(context.Background(), ApplyOptions{RunID: runID, RepoPath: repoDir})
	if err == nil || !strings.Contains(err.Error(), "does not match run source_commit_sha") {
		t.Fatalf("expected sha mismatch error, got %v", err)
	}
}

func TestRunApplyForceBypassesAllSafetyChecks(t *testing.T) {
	repoDir := gitrepo.SetupWithRemote(t, "ssh://git@other.example.com:7999/other/service.git")
	gitrepo.Run(t, repoDir, "remote", "remove", "origin")
	gitrepo.WriteFile(t, filepath.Join(repoDir, "LOCAL.txt"), "staged local change\n")
	gitrepo.Run(t, repoDir, "add", "LOCAL.txt")
	runID := domaintypes.NewRunID().String()
	repoID := domaintypes.NewRepoID().String()
	jobID := domaintypes.NewJobID().String()
	patch := []byte("diff --git a/README.md b/README.md\nindex 5b4f9e0..98a5560 100644\n--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-# Test Repo\n+# Forced Repo\n")
	server := newRunApplyServer(t, runID, repoID, jobID, "", patch)
	defer server.Close()
	clienv.UseControlPlaneEnv(t, server.URL)

	if err := RunApply(context.Background(), ApplyOptions{RunID: runID, RepoPath: repoDir, Force: true}); err != nil {
		t.Fatalf("RunApply --force: %v", err)
	}
	gitrepo.AssertFileContent(t, filepath.Join(repoDir, "README.md"), "# Forced Repo\n")
	gitrepo.AssertFileContent(t, filepath.Join(repoDir, "LOCAL.txt"), "staged local change\n")
}

func newRunApplyServer(t *testing.T, runID, repoID, jobID, sourceSHA string, patch []byte) *httptest.Server {
	t.Helper()
	diffID := "11111111-1111-1111-1111-111111111111"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/"+runID+"/pull":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"run_id":            runID,
				"repo_id":           repoID,
				"repo_url":          "https://gitlab.example.com/acme/service.git",
				"source_commit_sha": sourceSHA,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID+"/diffs" && r.URL.Query().Get("download") != "true":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"diffs": []map[string]any{{
					"id":           diffID,
					"job_id":       jobID,
					"created_at":   "2026-05-28T00:00:00Z",
					"gzipped_size": len(patch),
				}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID+"/diffs" && r.URL.Query().Get("download") == "true":
			w.Header().Set("Content-Type", "application/gzip")
			gz := gzip.NewWriter(w)
			_, _ = gz.Write(patch)
			_ = gz.Close()
		default:
			http.NotFound(w, r)
		}
	}))
}
