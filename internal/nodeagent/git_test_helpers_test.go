package nodeagent

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initGitRepo initializes a git repository with user configuration for testing.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.name", "Test User")
	gitRun(t, dir, "config", "user.email", "test@example.com")
}

// gitCommit stages all changes and creates a commit with the specified message.
func gitCommit(t *testing.T, dir, message string) {
	t.Helper()
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", message)
}

// gitRun executes a git command in the specified directory and fails on error.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v (output: %s)", args[0], err, string(output))
	}
}

// writeFile writes content to a file, creating parent directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s failed: %v", path, err)
	}
}

// assertFileContent verifies file content matches expected value.
func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s failed: %v", path, err)
	}
	if string(content) != expected {
		t.Errorf("file %s content = %q, want %q", path, string(content), expected)
	}
}

// assertGitRepo verifies the directory is a valid git repository.
func assertGitRepo(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Errorf("directory %s is not a git repo: %v", dir, err)
	}
}

// generateGitDiff generates a unified diff for the workspace using "git diff HEAD".
func generateGitDiff(t *testing.T, workspace string) []byte {
	t.Helper()
	cmd := exec.Command("git", "diff", "HEAD")
	cmd.Dir = workspace
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 0 {
			t.Fatalf("git diff failed: %v (output: %s)", err, string(output))
		}
	}
	return output
}

// setupGitRepoWithChange initializes a git repo with an initial commit and an uncommitted change.
func setupGitRepoWithChange(t *testing.T, workspace string) {
	t.Helper()
	initGitRepo(t, workspace)
	writeFile(t, filepath.Join(workspace, "test.txt"), "initial content\n")
	gitCommit(t, workspace, "Initial commit")
	writeFile(t, filepath.Join(workspace, "test.txt"), "modified content\n")
}
