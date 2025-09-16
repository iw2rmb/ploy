package mods

import (
	"context"
	"fmt"
	"path/filepath"
)

// PrepareRepo clones the target repository and creates a workflow branch; returns the repo path and branch name.
func (r *ModRunner) PrepareRepo(ctx context.Context) (string, string, error) {
	repoPath := filepath.Join(r.workspaceDir, "repo-apply")
	if r.repoManager != nil {
		if err := r.repoManager.Clone(ctx, r.config.TargetRepo, r.config.BaseRef, repoPath); err != nil {
			return "", "", fmt.Errorf("clone failed: %w", err)
		}
	} else if err := r.gitOps.CloneRepository(ctx, r.config.TargetRepo, r.config.BaseRef, repoPath); err != nil {
		return "", "", fmt.Errorf("clone failed: %w", err)
	}
	branchName := GenerateBranchName(r.config.ID)
	if r.repoManager != nil {
		if err := r.repoManager.CreateBranch(ctx, repoPath, branchName); err != nil {
			return "", "", fmt.Errorf("branch failed: %w", err)
		}
	} else if err := r.gitOps.CreateBranchAndCheckout(ctx, repoPath, branchName); err != nil {
		return "", "", fmt.Errorf("branch failed: %w", err)
	}
	return repoPath, branchName, nil
}
