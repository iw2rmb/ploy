package gitrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func Run(t testing.TB, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (output: %s)", args, err, string(output))
	}
	return output
}

func Init(t testing.TB, dir string) {
	t.Helper()
	Run(t, dir, "init")
	Run(t, dir, "config", "user.name", "Test User")
	Run(t, dir, "config", "user.email", "test@example.com")
	// Disable global hooks (e.g. contents.md pre-commit hook) in test repos.
	Run(t, dir, "config", "core.hooksPath", "/dev/null")
}

func InitMainBranch(t testing.TB, dir string) {
	t.Helper()
	Init(t, dir)
	Run(t, dir, "checkout", "-b", "main")
}

func CommitAll(t testing.TB, dir, message string) {
	t.Helper()
	Run(t, dir, "add", ".")
	Run(t, dir, "commit", "-m", message)
}

func WriteFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s failed: %v", path, err)
	}
}

func AssertFileContent(t testing.TB, path, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s failed: %v", path, err)
	}
	if string(content) != expected {
		t.Fatalf("file %s content = %q, want %q", path, string(content), expected)
	}
}

func AssertRepo(t testing.TB, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("directory %s is not a git repo: %v", dir, err)
	}
}

func DiffHEAD(t testing.TB, workspace string) []byte {
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

func SetupWithChange(t testing.TB, workspace string) {
	t.Helper()
	Init(t, workspace)
	WriteFile(t, filepath.Join(workspace, "test.txt"), "initial content\n")
	CommitAll(t, workspace, "Initial commit")
	WriteFile(t, filepath.Join(workspace, "test.txt"), "modified content\n")
}

func SetupBasic(t testing.TB) string {
	t.Helper()
	repoDir := t.TempDir()
	InitMainBranch(t, repoDir)
	WriteFile(t, filepath.Join(repoDir, "README.md"), "# Test Repo\n")
	Run(t, repoDir, "add", "README.md")
	Run(t, repoDir, "commit", "-m", "Initial commit")
	return repoDir
}

func SetupWithRemote(t testing.TB, remoteURL string) string {
	t.Helper()
	repoDir := SetupBasic(t)
	Run(t, repoDir, "remote", "add", "origin", remoteURL)
	return repoDir
}

func RevParse(t testing.TB, dir, rev string) string {
	t.Helper()
	output := Run(t, dir, "rev-parse", rev)
	return strings.TrimSpace(string(output))
}
