package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// WorkspaceStatus returns the output of `git status --porcelain` for the given directory.
func WorkspaceStatus(ctx context.Context, repoDir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = repoDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git status --porcelain failed: %w (stderr=%s)", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
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

	addArgs := append([]string{"add", "-A", "--", "."}, excludedBuildPathspecs...)
	if err := runGitCommand(ctx, repoDir, nil, addArgs...); err != nil {
		return false, fmt.Errorf("git add: %w", err)
	}
	if err := runGitCommand(ctx, repoDir, nil, "commit", "-m", message); err != nil {
		return false, fmt.Errorf("git commit: %w", err)
	}
	return true, nil
}
