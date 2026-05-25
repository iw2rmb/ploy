package pull

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/assertx"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
)

// =============================================================================
// Routing and Flag Parsing Tests
// =============================================================================

func executePullTestCmd(args []string, stderr io.Writer) error {
	if len(args) >= 2 && args[0] == "run" && args[1] == "pull" {
		return HandleRunPull(args[2:], stderr)
	}
	if len(args) >= 2 && args[0] == "mig" && args[1] == "pull" {
		return HandleMigPull(args[2:], stderr)
	}
	return fmt.Errorf("unsupported pull test args: %v", args)
}

type pullUsageErrorCase struct {
	name      string
	args      []string
	wantErr   string
	wantUsage bool
}

func runPullUsageErrorCases(t *testing.T, usage string, tests []pullUsageErrorCase) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := clienv.RunExpectError(t, executePullTestCmd, tc.args, tc.wantErr)
			if tc.wantUsage {
				assertx.Contains(t, out, usage)
			}
		})
	}
}

// TestRunPullRouting validates that `ploy run pull` routes to HandleRunPull.
func TestRunPullRouting(t *testing.T) {
	requireGit(t)

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "pull without run-id", args: []string{"run", "pull"}, wantErr: "run-id required"},
		{name: "pull with empty run-id", args: []string{"run", "pull", "   "}, wantErr: "run-id required"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clienv.RunExpectError(t, executePullTestCmd, tc.args, tc.wantErr)
		})
	}
}

// TestRunPullUsageErrors validates that invalid flag combinations return appropriate errors.
func TestRunPullUsageErrors(t *testing.T) {
	t.Parallel()

	runPullUsageErrorCases(t, "Usage: ploy run pull", []pullUsageErrorCase{
		{name: "unknown flag", args: []string{"run", "pull", "--unknown", "my-run"}, wantErr: "flag provided but not defined", wantUsage: true},
		{name: "origin flag without value", args: []string{"run", "pull", "--origin"}, wantErr: "flag needs an argument", wantUsage: true},
		{name: "extra positional argument", args: []string{"run", "pull", "my-run", "extra-arg"}, wantErr: "unexpected argument: extra-arg"},
	})
}

// TestRunPullUsageHelp validates that the usage text contains expected content.
func TestRunPullUsageHelp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printRunPullUsage(&buf)
	out := buf.String()

	for _, want := range []string{
		"Usage: ploy run pull",
		"--origin",
		"--dry-run",
		"<run-id>",
		"Examples:",
		"Pulls Migs diffs from a run",
	} {
		assertx.Contains(t, out, want)
	}
}

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
