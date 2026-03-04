package nodeagent

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
)

// initGitRepo initializes a git repository with user configuration for testing.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	gitrepo.Init(t, dir)
}

// gitCommit stages all changes and creates a commit with the specified message.
func gitCommit(t *testing.T, dir, message string) {
	t.Helper()
	gitrepo.CommitAll(t, dir, message)
}

// gitRun executes a git command in the specified directory and fails on error.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	gitrepo.Run(t, dir, args...)
}

// writeFile writes content to a file, creating parent directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	gitrepo.WriteFile(t, path, content)
}

// assertFileContent verifies file content matches expected value.
func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()
	gitrepo.AssertFileContent(t, path, expected)
}

// assertGitRepo verifies the directory is a valid git repository.
func assertGitRepo(t *testing.T, dir string) {
	t.Helper()
	gitrepo.AssertRepo(t, dir)
}

// generateGitDiff generates a unified diff for the workspace using "git diff HEAD".
func generateGitDiff(t *testing.T, workspace string) []byte {
	t.Helper()
	return gitrepo.DiffHEAD(t, workspace)
}

// setupGitRepoWithChange initializes a git repo with an initial commit and an uncommitted change.
func setupGitRepoWithChange(t *testing.T, workspace string) {
	t.Helper()
	gitrepo.SetupWithChange(t, workspace)
}
