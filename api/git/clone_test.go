package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initBareRepo initializes a bare git repository at the provided path.
func initBareRepo(t *testing.T, repoPath string) {
	t.Helper()
	parent := filepath.Dir(repoPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatalf("create parent dir: %v", err)
	}
	cmd := exec.Command("git", "init", "--bare", repoPath)
	cmd.Dir = parent
	if err := cmd.Run(); err != nil {
		t.Fatalf("init bare repo: %v", err)
	}
}

// seedRemoteRepo pushes main and feature branches to the bare remote.
func seedRemoteRepo(t *testing.T, remotePath string) {
	t.Helper()
	work := t.TempDir()
	runGit(t, work, "init")
	runGit(t, work, "remote", "add", "origin", remotePath)
	runGit(t, work, "config", "user.name", "Seeder")
	runGit(t, work, "config", "user.email", "seed@example.com")

	mainFile := filepath.Join(work, "main.txt")
	if err := os.WriteFile(mainFile, []byte("main branch\n"), 0o644); err != nil {
		t.Fatalf("write main file: %v", err)
	}
	runGit(t, work, "add", "main.txt")
	runGit(t, work, "commit", "-m", "main commit")
	runGit(t, work, "branch", "-M", "main")
	runGit(t, work, "push", "-u", "origin", "main")

	runGit(t, work, "checkout", "-b", "feature")
	featureFile := filepath.Join(work, "feature.txt")
	if err := os.WriteFile(featureFile, []byte("feature branch\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	runGit(t, work, "add", "feature.txt")
	runGit(t, work, "commit", "-m", "feature commit")
	runGit(t, work, "push", "-u", "origin", "feature")
}

func TestCloneRepositoryChecksOutRequestedBranch(t *testing.T) {
	remoteParent := t.TempDir()
	remotePath := filepath.Join(remoteParent, "origin.git")
	initBareRepo(t, remotePath)
	seedRemoteRepo(t, remotePath)

	cloneDir := filepath.Join(t.TempDir(), "clone")
	svc := NewService(ServiceConfig{})
	if err := svc.CloneRepository(context.Background(), remotePath, "refs/heads/feature", cloneDir); err != nil {
		t.Fatalf("CloneRepository error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cloneDir, "feature.txt"))
	if err != nil {
		t.Fatalf("read feature file: %v", err)
	}
	if !strings.Contains(string(data), "feature branch") {
		t.Fatalf("expected feature branch content, got %q", string(data))
	}

	branch := runGitOutput(t, cloneDir, "rev-parse", "--abbrev-ref", "HEAD")
	if branch != "feature" {
		t.Fatalf("expected checked out branch to be feature, got %q", branch)
	}

	sparse := runGitOutput(t, cloneDir, "config", "core.sparseCheckout")
	if sparse != "false" {
		t.Fatalf("expected sparse checkout disabled, got %q", sparse)
	}
}

func TestCloneRepositoryReturnsErrorForMissingRemote(t *testing.T) {
	cloneDir := filepath.Join(t.TempDir(), "clone")
	svc := NewService(ServiceConfig{})
	err := svc.CloneRepository(context.Background(), filepath.Join(t.TempDir(), "missing.git"), "main", cloneDir)
	if err == nil {
		t.Fatalf("expected error for missing remote")
	}
	if !strings.Contains(err.Error(), "git clone failed") {
		t.Fatalf("expected git clone failure message, got %v", err)
	}
}
