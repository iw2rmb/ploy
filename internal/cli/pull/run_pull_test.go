package pull

import (
	"os"
	"os/exec"
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/clienv"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
)

// =============================================================================
// Git Worktree Precondition Tests
// =============================================================================

// TestHandleRunPull_OutsideGitRepo verifies that HandleRunPull fails outside a git repo.
func TestHandleRunPull_OutsideGitRepo(t *testing.T) {
	requireGit(t)
	gitrepo.WithCWD(t, t.TempDir())
	clienv.RunExpectError(t, HandleRunPull, []string{"my-run"}, "must be run inside a git repository")
}

// TestHandleRunPull_DirtyWorkingTree verifies that HandleRunPull fails on a dirty worktree.
func TestHandleRunPull_DirtyWorkingTree(t *testing.T) {
	requireGit(t)
	repoDir := gitrepo.SetupWithRemote(t, "https://github.com/example/repo.git")
	if err := os.WriteFile(repoDir+"/dirty.txt", []byte("dirty content\n"), 0644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	gitrepo.WithCWD(t, repoDir)
	clienv.RunExpectError(t, HandleRunPull, []string{"my-run"}, "working tree must be clean")
}

// TestHandleRunPull_MissingRemote verifies that HandleRunPull fails on a missing remote.
func TestHandleRunPull_MissingRemote(t *testing.T) {
	requireGit(t)
	repoDir := gitrepo.SetupWithRemote(t, "https://github.com/example/repo.git")
	gitrepo.WithCWD(t, repoDir)
	clienv.RunExpectError(t, HandleRunPull,
		[]string{"--origin", "nonexistent", "my-run"},
		`git remote "nonexistent" not found`)
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}
}
