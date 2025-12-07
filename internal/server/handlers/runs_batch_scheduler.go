package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/iw2rmb/ploy/internal/store"
)

// BatchRepoStarter starts execution for pending repos in batch runs.
// It implements the batchscheduler.RepoStarter interface.
type BatchRepoStarter struct {
	store store.Store
}

// NewBatchRepoStarter creates a new BatchRepoStarter with the given store.
func NewBatchRepoStarter(st store.Store) *BatchRepoStarter {
	return &BatchRepoStarter{store: st}
}

// StartPendingRepos starts execution for all pending repos in a batch run.
// For each pending repo:
//  1. Creates a child execution run with the batch spec
//  2. Creates jobs from the spec
//  3. Links the repo to the child run and marks it as running
//
// Returns the number of repos that were successfully started.
// Errors from individual repos are logged but don't prevent processing other repos.
func (s *BatchRepoStarter) StartPendingRepos(ctx context.Context, runID string) (int, error) {
	runIDStr := runID

	// Fetch the batch run to get the shared spec.
	batchRun, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return 0, fmt.Errorf("get batch run: %w", err)
	}

	// Skip terminal runs — no more repos to start.
	if isTerminalRunStatus(batchRun.Status) {
		return 0, nil
	}

	// Fetch pending repos for this batch.
	pendingRepos, err := s.store.ListPendingRunReposByRun(ctx, runID)
	if err != nil {
		return 0, fmt.Errorf("list pending repos: %w", err)
	}

	if len(pendingRepos) == 0 {
		return 0, nil
	}

	// Start execution for each pending repo.
	var started int
	for _, repo := range pendingRepos {
		// Create child execution run for this repo.
		// The child run inherits the spec from the batch but has repo-specific URL/refs.
		childRun, err := s.store.CreateRun(ctx, store.CreateRunParams{
			Name:      nil, // Child runs don't need a name; batch name is on parent.
			RepoUrl:   repo.RepoUrl,
			Spec:      batchRun.Spec,
			CreatedBy: batchRun.CreatedBy,
			Status:    store.RunStatusQueued,
			BaseRef:   repo.BaseRef,
			TargetRef: repo.TargetRef,
			CommitSha: nil, // Commit SHA resolved at execution time by node.
		})
		if err != nil {
			slog.Error("batch-scheduler: create child run failed",
				"run_id", runIDStr,
				"repo_id", repo.ID,
				"repo_url", repo.RepoUrl,
				"err", err,
			)
			continue // Skip this repo but try others.
		}

		// Create jobs from the batch spec for this child run.
		if err := createJobsFromSpec(ctx, s.store, childRun.ID, batchRun.Spec); err != nil {
			slog.Error("batch-scheduler: create jobs failed",
				"run_id", runIDStr,
				"child_run_id", childRun.ID,
				"repo_url", repo.RepoUrl,
				"err", err,
			)
			// Clean up the orphaned child run.
			_ = s.store.DeleteRun(ctx, childRun.ID)
			continue // Skip this repo but try others.
		}

		// Link the repo entry to its child execution run and mark as running.
		err = s.store.SetRunRepoExecutionRun(ctx, store.SetRunRepoExecutionRunParams{
			ID:             repo.ID,
			ExecutionRunID: &childRun.ID,
		})
		if err != nil {
			slog.Error("batch-scheduler: link repo to child run failed",
				"run_id", runIDStr,
				"repo_id", repo.ID,
				"child_run_id", childRun.ID,
				"err", err,
			)
			// The child run exists but isn't linked; it will still execute.
			// Log but count as started since jobs were created.
		}

		started++
		slog.Info("batch-scheduler: repo execution started",
			"run_id", runIDStr,
			"repo_id", repo.ID,
			"child_run_id", childRun.ID,
			"repo_url", repo.RepoUrl,
		)
	}

	// Update batch run status to running if we started at least one repo
	// and the batch is still in queued state.
	if started > 0 && batchRun.Status == store.RunStatusQueued {
		if err := s.store.AckRunStart(ctx, runID); err != nil {
			slog.Warn("batch-scheduler: failed to update batch status to running",
				"run_id", runIDStr,
				"err", err,
			)
		}
	}

	return started, nil
}
