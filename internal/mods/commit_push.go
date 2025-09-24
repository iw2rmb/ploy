package mods

import (
	"context"
	"fmt"

	gitapi "github.com/iw2rmb/ploy/api/git"
)

// runCommitStep performs commit logic considering whether changes exist and whether HEAD moved already.
// Returns committed=true when a commit was created or already exists; message provides a human summary.
func (r *ModRunner) runCommitStep(ctx context.Context, repoPath, initialHead string) (bool, string, error) {
	r.reapplyTransformationDiffs(ctx, repoPath)
	// If HEAD already moved, assume apply step committed and skip additional commits
	headAfter, _ := getHeadHashFn(repoPath)
	if headAfter != "" && initialHead != "" && headAfter != initialHead {
		return true, "Changes already committed by apply step", nil
	}
	changed, _ := hasRepoChangesFn(repoPath)
	if !changed {
		return false, "No changes to commit", fmt.Errorf("no changes produced by transformation")
	}
	if err := r.gitOps.CommitChanges(ctx, repoPath, fmt.Sprintf("Applied recipe transformations for workflow %s", r.config.ID)); err != nil {
		return false, "Commit failed", fmt.Errorf("failed to commit changes: %w", err)
	}
	return true, "Committed changes", nil
}

// runPushStep pushes the branch with a timeout.
func (r *ModRunner) runPushStep(ctx context.Context, repoPath, branchName string) error {
	pusher := r.gitPushClient()
	if pusher == nil {
		r.emit(ctx, "git", "push", "error", "no git pusher configured")
		return fmt.Errorf("git pusher not configured")
	}
	op := pusher.PushBranchAsync(ctx, repoPath, r.config.TargetRepo, branchName)
	var events <-chan gitapi.Event
	if op != nil {
		events = op.Events()
	}
	if events == nil {
		ch := make(chan gitapi.Event)
		close(ch)
		events = ch
	}
	for event := range events {
		level := "info"
		switch event.Type {
		case gitapi.EventFailed:
			level = "error"
		case gitapi.EventProgress:
			level = "debug"
		case gitapi.EventStarted:
			level = "info"
		case gitapi.EventCompleted:
			level = "info"
		}
		message := event.Message
		if message == "" {
			message = fmt.Sprintf("git push %s", event.Type)
		}
		r.emit(ctx, "git", "push", level, message)
	}
	if op != nil {
		if err := op.Err(); err != nil {
			return err
		}
	}
	return nil
}
