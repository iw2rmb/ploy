package nodeagent

import (
	"path/filepath"
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

// initRepoWithFile initializes a git repo, writes a single file, and commits.
func initRepoWithFile(t *testing.T, dir, file, content string) {
	t.Helper()
	initGitRepo(t, dir)
	writeFile(t, filepath.Join(dir, file), content)
	gitCommit(t, dir, "initial commit")
}
