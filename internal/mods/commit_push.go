package mods

import (
	"context"
	"fmt"
	"time"
)

// runCommitStep performs commit logic considering whether changes exist and whether HEAD moved already.
// Returns committed=true when a commit was created or already exists; message provides a human summary.
func (r *ModRunner) runCommitStep(ctx context.Context, repoPath, initialHead string) (bool, string, error) {
	changed, _ := hasRepoChangesFn(repoPath)
	if !changed {
		headAfter, _ := getHeadHashFn(repoPath)
		if headAfter != "" && initialHead != "" && headAfter != initialHead {
			return true, "Changes already committed by apply step", nil
		}
		return false, "No changes to commit", fmt.Errorf("no changes produced by transformation")
	}
	if err := r.gitOps.CommitChanges(ctx, repoPath, fmt.Sprintf("Applied recipe transformations for workflow %s", r.config.ID)); err != nil {
		return false, "Commit failed", fmt.Errorf("failed to commit changes: %w", err)
	}
	return true, "Committed changes", nil
}

// runPushStep pushes the branch with a timeout.
func (r *ModRunner) runPushStep(ctx context.Context, repoPath, branchName string) error {
	pushTimeout := 3 * time.Minute
	pushCtx, cancel := context.WithTimeout(ctx, pushTimeout)
	defer cancel()
	return r.gitOps.PushBranch(pushCtx, repoPath, r.config.TargetRepo, branchName)
}
