package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CommitChanges creates a commit with the current changes.
func (g *Service) CommitChanges(ctx context.Context, repoPath, message string) error {
	if err := g.ensureGitConfig(ctx, repoPath); err != nil {
		return fmt.Errorf("failed to configure git: %w", err)
	}

	addCmd := exec.CommandContext(ctx, "git", "add", "-A")
	addCmd.Dir = repoPath
	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("failed to stage changes: %w", err)
	}

	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	commitCmd.Dir = repoPath
	var stderr, stdout bytes.Buffer
	commitCmd.Stderr = &stderr
	commitCmd.Stdout = &stdout
	if err := commitCmd.Run(); err != nil {
		output := stderr.String() + " " + stdout.String()
		if strings.Contains(output, "nothing to commit") || strings.Contains(output, "working tree clean") {
			return nil
		}
		return fmt.Errorf("failed to commit changes: %v - stderr: %s - stdout: %s", err, stderr.String(), stdout.String())
	}

	return nil
}

// ensureGitConfig seeds default author metadata when unset in the repository.
func (g *Service) ensureGitConfig(ctx context.Context, repoPath string) error {
	nameCmd := exec.CommandContext(ctx, "git", "config", "user.name")
	nameCmd.Dir = repoPath
	if err := nameCmd.Run(); err != nil {
		setNameCmd := exec.CommandContext(ctx, "git", "config", "user.name", "Ploy Transflow")
		setNameCmd.Dir = repoPath
		if err := setNameCmd.Run(); err != nil {
			return fmt.Errorf("failed to set git user.name: %w", err)
		}
	}

	emailCmd := exec.CommandContext(ctx, "git", "config", "user.email")
	emailCmd.Dir = repoPath
	if err := emailCmd.Run(); err != nil {
		setEmailCmd := exec.CommandContext(ctx, "git", "config", "user.email", "mods@ploy.automation")
		setEmailCmd.Dir = repoPath
		if err := setEmailCmd.Run(); err != nil {
			return fmt.Errorf("failed to set git user.email: %w", err)
		}
	}

	return nil
}

// GetCommitHash returns the current commit hash.
func (g *Service) GetCommitHash(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// CreateBranch creates a new branch from the current HEAD.
func (g *Service) CreateBranch(ctx context.Context, repoPath, branchName string) error {
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", branchName)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create branch %s: %w", branchName, err)
	}
	return nil
}

// ResetToCommit resets the repository to a specific commit.
func (g *Service) ResetToCommit(ctx context.Context, repoPath, commitHash string) error {
	cmd := exec.CommandContext(ctx, "git", "reset", "--hard", commitHash)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reset to commit %s: %w", commitHash, err)
	}
	return nil
}
