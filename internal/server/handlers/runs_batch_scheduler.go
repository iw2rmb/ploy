package handlers

import (
	"context"
	"fmt"
	"log/slog"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/store/batchscheduler"
)

// StartPendingReposResult is an alias to batchscheduler.StartPendingReposResult.
// This ensures the handlers package and batchscheduler package use the same type,
// allowing BatchRepoStarter to implement the batchscheduler.RepoStarter interface.
type StartPendingReposResult = batchscheduler.StartPendingReposResult

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
// Returns StartPendingReposResult with counts of started, already done, and still pending repos.
// This unified return type ensures consistent semantics between the HTTP handler and
// background scheduler paths.
//
// Errors from individual repos are logged but don't prevent processing other repos.
func (s *BatchRepoStarter) StartPendingRepos(ctx context.Context, runID string) (StartPendingReposResult, error) {
	result := StartPendingReposResult{}

	// Fetch the batch run to get the shared spec.
	batchRun, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return result, fmt.Errorf("get batch run: %w", err)
	}

	// Skip terminal runs — no more repos to start.
	if isTerminalRunStatus(batchRun.Status) {
		return result, nil
	}

	// Fetch all repos for this batch to compute counts.
	// We need this to determine AlreadyDone and the initial Pending count for the response.
	allRepos, err := s.store.ListRunReposByRun(ctx, runID)
	if err != nil {
		return result, fmt.Errorf("list run repos: %w", err)
	}

	// Count repos by status to populate AlreadyDone and initial Pending.
	var initialPending int
	for _, repo := range allRepos {
		if isTerminalRunRepoStatus(repo.Status) {
			result.AlreadyDone++
		} else if repo.Status == store.RunRepoStatusPending {
			initialPending++
		}
		// Running repos are not counted in AlreadyDone or Pending.
	}

	// Fetch pending repos that need to start execution.
	pendingRepos, err := s.store.ListPendingRunReposByRun(ctx, runID)
	if err != nil {
		return result, fmt.Errorf("list pending repos: %w", err)
	}

	// If no pending repos, return early with current counts.
	if len(pendingRepos) == 0 {
		result.Pending = initialPending
		return result, nil
	}

	// Start execution for each pending repo.
	for _, repo := range pendingRepos {
		// Create child execution run for this repo.
		// The child run inherits the spec and commit_sha from the batch but has
		// repo-specific URL/refs.
		childRunID := domaintypes.NewRunID()
		childRun, err := s.store.CreateRun(ctx, store.CreateRunParams{
			ID:        string(childRunID),
			Name:      nil, // Child runs don't need a name; batch name is on parent.
			RepoUrl:   repo.RepoUrl,
			Spec:      batchRun.Spec,
			CreatedBy: batchRun.CreatedBy,
			Status:    store.RunStatusQueued,
			BaseRef:   repo.BaseRef,
			TargetRef: repo.TargetRef,
			CommitSha: batchRun.CommitSha,
		})
		if err != nil {
			slog.Error("start pending repos: create child run failed",
				"run_id", runID,
				"repo_id", repo.ID,
				"repo_url", repo.RepoUrl,
				"err", err,
			)
			continue // Skip this repo but try others.
		}

		// Create jobs from the batch spec for this child run.
		if err := createJobsFromSpec(ctx, s.store, domaintypes.RunID(childRun.ID), batchRun.Spec); err != nil {
			slog.Error("start pending repos: create jobs failed",
				"run_id", runID,
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
			slog.Error("start pending repos: link repo to child run failed",
				"run_id", runID,
				"repo_id", repo.ID,
				"child_run_id", childRun.ID,
				"err", err,
			)
			// The child run exists but isn't linked; it will still execute.
			// Log but count as started since jobs were created.
		}

		result.Started++
		slog.Info("start pending repos: repo execution started",
			"run_id", runID,
			"repo_id", repo.ID,
			"child_run_id", childRun.ID,
			"repo_url", repo.RepoUrl,
		)
	}

	// Calculate remaining pending repos after starting.
	result.Pending = initialPending - result.Started

	// Update batch run status to running if we started at least one repo
	// and the batch is still in queued state.
	if result.Started > 0 && batchRun.Status == store.RunStatusQueued {
		if err := s.store.AckRunStart(ctx, runID); err != nil {
			slog.Warn("start pending repos: failed to update batch status to running",
				"run_id", runID,
				"err", err,
			)
		}
	}

	return result, nil
}
