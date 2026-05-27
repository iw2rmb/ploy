package pull

import (
	"os"
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/clienv"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
)

func TestMigPullRejectsMutuallyExclusiveSelectionFlags(t *testing.T) {
	t.Parallel()

	clienv.RunExpectError(t, HandleMigPull, []string{"--last-failed", "--last-succeeded"}, "mutually exclusive")
}

// =============================================================================
// Git Worktree Precondition Tests
// =============================================================================

// TestHandleMigPull_OutsideGitRepo verifies that HandleMigPull fails outside a git repo.
func TestHandleMigPull_OutsideGitRepo(t *testing.T) {
	requireGit(t)
	gitrepo.WithCWD(t, t.TempDir())
	clienv.RunExpectError(t, HandleMigPull, []string{"my-mig"}, "must be run inside a git repository")
}

// TestHandleMigPull_DirtyWorkingTree verifies that HandleMigPull fails on a dirty worktree.
func TestHandleMigPull_DirtyWorkingTree(t *testing.T) {
	requireGit(t)
	repoDir := gitrepo.SetupWithRemote(t, "https://github.com/example/repo.git")
	if err := os.WriteFile(repoDir+"/dirty.txt", []byte("dirty content\n"), 0644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	gitrepo.WithCWD(t, repoDir)
	clienv.RunExpectError(t, HandleMigPull, []string{"my-mig"}, "working tree must be clean")
}

// TestHandleMigPull_MissingRemote verifies that HandleMigPull fails on a missing remote.
func TestHandleMigPull_MissingRemote(t *testing.T) {
	requireGit(t)
	repoDir := gitrepo.SetupWithRemote(t, "https://github.com/example/repo.git")
	gitrepo.WithCWD(t, repoDir)
	clienv.RunExpectError(t, HandleMigPull,
		[]string{"--origin", "nonexistent", "my-mig"},
		`git remote "nonexistent" not found`)
}
