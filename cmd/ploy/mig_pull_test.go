package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/assertx"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
)

// =============================================================================
// Routing and Flag Parsing Tests
// =============================================================================

// TestMigPullRouting validates that `ploy mig pull` routes to handleMigPull.
// The test triggers the git-worktree check by running from a non-git directory.
func TestMigPullRouting(t *testing.T) {
	requireGit(t)

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "mig pull routes correctly", args: []string{"mig", "pull"}, wantErr: "must be run inside a git repository"},
		{name: "mig pull with mig-id routes correctly", args: []string{"mig", "pull", "my-mig"}, wantErr: "must be run inside a git repository"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gitrepo.WithCWD(t, t.TempDir())
			clienv.RunExpectError(t, executeCmd, tc.args, tc.wantErr)
		})
	}
}

// TestMigPullUsageErrors validates that invalid flag combinations return appropriate errors.
func TestMigPullUsageErrors(t *testing.T) {
	t.Parallel()

	runPullUsageErrorCases(t, "Usage: ploy mig pull", []pullUsageErrorCase{
		{name: "unknown flag", args: []string{"mig", "pull", "--unknown"}, wantErr: "flag provided but not defined", wantUsage: true},
		{name: "origin flag without value", args: []string{"mig", "pull", "--origin"}, wantErr: "flag needs an argument", wantUsage: true},
		{name: "extra positional argument", args: []string{"mig", "pull", "my-mig", "extra-arg"}, wantErr: "unexpected argument: extra-arg"},
		{name: "mutually exclusive flags", args: []string{"mig", "pull", "--last-failed", "--last-succeeded"}, wantErr: "mutually exclusive", wantUsage: true},
	})
}

// TestMigPullUsageHelp validates that the usage text contains expected content.
func TestMigPullUsageHelp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printMigPullUsage(&buf)
	out := buf.String()

	for _, want := range []string{
		"Usage: ploy mig pull",
		"--origin",
		"--dry-run",
		"--last-failed",
		"--last-succeeded",
		"[<mig-id|name>]",
		"Examples:",
		"Pulls Migs diffs from a mig",
	} {
		assertx.Contains(t, out, want)
	}
}

// =============================================================================
// Git Worktree Precondition Tests
// =============================================================================

// TestHandleMigPull_OutsideGitRepo verifies that handleMigPull fails outside a git repo.
func TestHandleMigPull_OutsideGitRepo(t *testing.T) {
	requireGit(t)
	gitrepo.WithCWD(t, t.TempDir())
	clienv.RunExpectError(t, handleMigPull, []string{"my-mig"}, "must be run inside a git repository")
}

// TestHandleMigPull_DirtyWorkingTree verifies that handleMigPull fails on a dirty worktree.
func TestHandleMigPull_DirtyWorkingTree(t *testing.T) {
	requireGit(t)
	repoDir := gitrepo.SetupWithRemote(t, "https://github.com/example/repo.git")
	if err := os.WriteFile(repoDir+"/dirty.txt", []byte("dirty content\n"), 0644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	gitrepo.WithCWD(t, repoDir)
	clienv.RunExpectError(t, handleMigPull, []string{"my-mig"}, "working tree must be clean")
}

// TestHandleMigPull_MissingRemote verifies that handleMigPull fails on a missing remote.
func TestHandleMigPull_MissingRemote(t *testing.T) {
	requireGit(t)
	repoDir := gitrepo.SetupWithRemote(t, "https://github.com/example/repo.git")
	gitrepo.WithCWD(t, repoDir)
	clienv.RunExpectError(t, handleMigPull,
		[]string{"--origin", "nonexistent", "my-mig"},
		`git remote "nonexistent" not found`)
}
