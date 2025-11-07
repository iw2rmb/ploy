package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// EnsureCommit stages and commits all changes in the repository when any exist.
// Returns true when a commit was created.
func EnsureCommit(ctx context.Context, repoDir, userName, userEmail, message string) (bool, error) {
	// Quick check: is there anything to commit?
	changed, err := hasWorkspaceChanges(ctx, repoDir)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}

	// Configure identity (best effort; ignore errors and let commit surface them).
	_ = runGitCommand(ctx, repoDir, nil, "config", "user.name", userName)
	_ = runGitCommand(ctx, repoDir, nil, "config", "user.email", userEmail)

	// Stage all changes except build outputs like Maven 'target/'.
	// Use pathspec excludes so we don't rely on repo .gitignore.
	if err := runGitCommand(ctx, repoDir, nil, "add", "-A", "--", ".",
		":(exclude)**/target/**", ":(exclude)target/"); err != nil {
		return false, fmt.Errorf("git add: %w", err)
	}
	if err := runGitCommand(ctx, repoDir, nil, "commit", "-m", message); err != nil {
		return false, fmt.Errorf("git commit: %w", err)
	}
	return true, nil
}

func hasWorkspaceChanges(ctx context.Context, repoDir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = repoDir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return out.Len() > 0, nil
}
