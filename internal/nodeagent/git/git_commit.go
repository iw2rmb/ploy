package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/gitexec"
)

// WorkspaceStatus returns the output of `git status --porcelain` for the given directory.
func WorkspaceStatus(ctx context.Context, repoDir string) (string, error) {
	result, err := gitexec.Execute(ctx, gitexec.Request{Dir: repoDir, Args: []string{"status", "--porcelain"}})
	if err != nil {
		return "", fmt.Errorf("git status --porcelain failed: %w (stderr=%s)", err, strings.TrimSpace(string(result.Stderr)))
	}
	return string(result.Stdout), nil
}

// EnsureCommit stages and commits all changes in the repository when any exist.
// Returns true when a commit was created.
func EnsureCommit(ctx context.Context, repoDir, userName, userEmail, message string) (bool, error) {
	status, err := WorkspaceStatus(ctx, repoDir)
	if err != nil {
		return false, err
	}
	if len(status) == 0 {
		return false, nil
	}

	_ = runGitCommand(ctx, repoDir, nil, "config", "user.name", userName)
	_ = runGitCommand(ctx, repoDir, nil, "config", "user.email", userEmail)

	if err := runGitCommand(ctx, repoDir, nil, "add", "-A", "--", "."); err != nil {
		return false, fmt.Errorf("git add: %w", err)
	}
	if err := runGitCommand(ctx, repoDir, nil, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-m", message); err != nil {
		return false, fmt.Errorf("git commit: %w", err)
	}
	return true, nil
}
