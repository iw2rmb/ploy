package run

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
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

func TestRunApplyAppliesStoredPatch(t *testing.T) {
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
	staged := gitrepo.Run(t, repoDir, "diff", "--cached", "--binary")
	gitrepo.WriteFile(t, filepath.Join(repoDir, "LOCAL.txt"), "unstaged local change\n")
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
	gitrepo.AssertFileContent(t, filepath.Join(repoDir, "LOCAL.txt"), "unstaged local change\n")
	if got := gitrepo.Run(t, repoDir, "diff", "--cached", "--binary"); !bytes.Equal(got, staged) {
		t.Fatalf("staged changes changed: %s", got)
	}
}

func newRunApplyServer(t *testing.T, runID, repoID, jobID, sourceSHA string, patches ...[]byte) *httptest.Server {
	t.Helper()
	items := make([]map[string]any, 0, len(patches))
	byID := make(map[string][]byte, len(patches))
	for i, patch := range patches {
		id := fmt.Sprintf("%08d-1111-1111-1111-111111111111", i+1)
		items = append(items, map[string]any{"id": id, "job_id": jobID, "created_at": "2026-05-28T00:00:00Z", "gzipped_size": len(patch)})
		byID[id] = patch
	}
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
			_ = json.NewEncoder(w).Encode(map[string]any{"diffs": items})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID+"/diffs" && r.URL.Query().Get("download") == "true":
			if r.URL.Query().Get("accumulated") != "" {
				t.Errorf("unexpected download query: %s", r.URL.RawQuery)
				http.Error(w, "unexpected download query", http.StatusBadRequest)
				return
			}
			patch, ok := byID[r.URL.Query().Get("diff_id")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			if patch == nil {
				http.Error(w, "patch storage unavailable", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/gzip")
			gz := gzip.NewWriter(w)
			_, _ = gz.Write(patch)
			_ = gz.Close()
		default:
			http.NotFound(w, r)
		}
	}))
}

// Dependent edits, reversions, binaries, and deletions are replayed in list order.
func TestRunApplyReplaysStoredPatchesLocally(t *testing.T) {
	repo := gitrepo.SetupWithRemote(t, "https://gitlab.example.com/acme/service.git")
	source := gitrepo.RevParse(t, repo, "HEAD")
	var patches [][]byte
	for _, text := range []string{"intermediate\n", "final\n", "# Test Repo\n"} {
		gitrepo.WriteFile(t, filepath.Join(repo, "README.md"), text)
		gitrepo.WriteFile(t, filepath.Join(repo, "binary.dat"), "\x00"+text)
		gitrepo.Run(t, repo, "add", "-A")
		patches = append(patches, gitrepo.Run(t, repo, "diff", "--cached", "--binary", "--full-index", "HEAD"))
		gitrepo.CommitAll(t, repo, "step")
	}
	if err := os.Remove(filepath.Join(repo, "README.md")); err != nil {
		t.Fatal(err)
	}
	gitrepo.Run(t, repo, "add", "-A")
	patches = append(patches, gitrepo.Run(t, repo, "diff", "--cached", "--binary", "HEAD"))
	wantTree := strings.TrimSpace(string(gitrepo.Run(t, repo, "write-tree")))
	gitrepo.Run(t, repo, "reset", "--hard", source)
	runID := domaintypes.NewRunID().String()
	server := newRunApplyServer(t, runID, domaintypes.NewRepoID().String(), domaintypes.NewJobID().String(), source, patches...)
	defer server.Close()
	clienv.UseControlPlaneEnv(t, server.URL)
	if err := RunApply(context.Background(), ApplyOptions{RunID: runID, RepoPath: repo}); err != nil {
		t.Fatal(err)
	}
	if staged := gitrepo.Run(t, repo, "diff", "--cached"); len(staged) != 0 {
		t.Fatalf("index changed: %s", staged)
	}
	gitrepo.Run(t, repo, "add", "-A")
	if got := strings.TrimSpace(string(gitrepo.Run(t, repo, "write-tree"))); got != wantTree {
		t.Fatalf("tree = %s, want %s", got, wantTree)
	}
}

// A failed later patch or download preserves both local files and the staging area.
func TestRunApplySequenceFailurePreservesCheckout(t *testing.T) {
	for _, failure := range []string{"replay", "download"} {
		t.Run(failure, func(t *testing.T) {
			repo := gitrepo.SetupWithRemote(t, "https://gitlab.example.com/acme/service.git")
			source := gitrepo.RevParse(t, repo, "HEAD")
			gitrepo.WriteFile(t, filepath.Join(repo, "README.md"), "first change\n")
			first := gitrepo.Run(t, repo, "diff", "--binary", "HEAD")
			gitrepo.Run(t, repo, "reset", "--hard", source)
			gitrepo.WriteFile(t, filepath.Join(repo, "LOCAL.txt"), "staged\n")
			gitrepo.Run(t, repo, "add", "LOCAL.txt")
			gitrepo.WriteFile(t, filepath.Join(repo, "LOCAL.txt"), "unstaged\n")
			before := gitrepo.Run(t, repo, "diff", "--cached", "--binary")
			second := []byte("invalid patch")
			if failure == "download" {
				second = nil
			}
			runID := domaintypes.NewRunID().String()
			server := newRunApplyServer(t, runID, domaintypes.NewRepoID().String(), domaintypes.NewJobID().String(), source, first, second)
			defer server.Close()
			clienv.UseControlPlaneEnv(t, server.URL)
			err := RunApply(context.Background(), ApplyOptions{RunID: runID, RepoPath: repo, Force: true})
			if err == nil || !strings.Contains(err.Error(), "00000002-1111-1111-1111-111111111111") {
				t.Fatalf("expected second patch failure, got %v", err)
			}
			gitrepo.AssertFileContent(t, filepath.Join(repo, "README.md"), "# Test Repo\n")
			gitrepo.AssertFileContent(t, filepath.Join(repo, "LOCAL.txt"), "unstaged\n")
			if got := gitrepo.Run(t, repo, "diff", "--cached", "--binary"); !bytes.Equal(got, before) {
				t.Fatalf("index changed: %s", got)
			}
		})
	}
}
