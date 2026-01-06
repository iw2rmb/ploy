package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupTestGitRepoForPull creates a minimal git repository for CLI pull tests.
// The repository contains an initial commit on main.
func setupTestGitRepoForPull(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()

	runGitCmdForPull(t, repoDir, "init")
	runGitCmdForPull(t, repoDir, "checkout", "-b", "main")
	runGitCmdForPull(t, repoDir, "config", "user.name", "Test User")
	runGitCmdForPull(t, repoDir, "config", "user.email", "test@example.com")

	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test Repo\n"), 0o644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	runGitCmdForPull(t, repoDir, "add", "README.md")
	runGitCmdForPull(t, repoDir, "commit", "-m", "Initial commit")

	return repoDir
}

// setupTestGitRepoWithRemote creates a git repository with an "origin" remote configured.
func setupTestGitRepoWithRemote(t *testing.T, remoteURL string) string {
	t.Helper()

	repoDir := setupTestGitRepoForPull(t)
	runGitCmdForPull(t, repoDir, "remote", "add", "origin", remoteURL)
	return repoDir
}

func runGitCmdForPull(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (output: %s)", args, err, string(output))
	}
}
