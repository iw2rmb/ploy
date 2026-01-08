// Package batchscheduler provides a background worker for scheduling queued repos within batch runs.
//
// The batch scheduler continuously monitors batch runs (parent runs with associated run_repos)
// and starts execution for any queued repos. This eliminates the need for manual POST /v1/runs/{id}/start
// calls and ensures repos are processed automatically after being added to a batch.
//
// State transitions:
//   - Batch run: Started → Finished/Cancelled (terminal)
//   - Run repo: Queued → Running → Success/Fail/Cancelled (terminal)
//
// The scheduler focuses on per-batch processing without cross-batch FIFO ordering.
package batchscheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// StartPendingReposResult contains the result of starting pending repos in a batch run.
// This type is defined here to avoid circular imports with the handlers package.
// It mirrors handlers.StartPendingReposResult for interface compatibility.
type StartPendingReposResult struct {
	// Started is the number of repos that were successfully started in this call.
	Started int
	// AlreadyDone is the number of repos already in a terminal state.
	AlreadyDone int
	// Pending is the number of repos still in pending state after this call.
	Pending int
}

// RepoStarter is the interface for starting execution of pending repos.
// Implemented by handlers.BatchRepoStarter to decouple scheduler from HTTP layer.
type RepoStarter interface {
	// StartPendingRepos starts execution for all pending repos in a batch run.
	// runID is a types.RunID (KSUID-backed) identifier for the batch run.
	// Returns StartPendingReposResult with counts of started, already done, and pending repos.
	StartPendingRepos(ctx context.Context, runID types.RunID) (StartPendingReposResult, error)
}

// Options configures the batch scheduler.
type Options struct {
	// Store is the database store for querying batch runs and repos.
	Store store.Store
	// RepoStarter handles the actual execution start logic.
	RepoStarter RepoStarter
	// Interval is how often the scheduler checks for pending repos. Default: 5 seconds.
	Interval time.Duration
	// Logger is used for structured logging. If nil, a default logger is used.
	Logger *slog.Logger
}

// Scheduler is a background task that processes pending repos within batch runs.
// It implements the scheduler.Task interface for integration with the server's
// task scheduler.
type Scheduler struct {
	store       store.Store
	repoStarter RepoStarter
	interval    time.Duration
	logger      *slog.Logger
}

// New constructs a new batch scheduler.
// Returns nil if store or repoStarter is nil.
func New(opts Options) (*Scheduler, error) {
	if opts.Store == nil || opts.RepoStarter == nil {
		return nil, nil
	}

	interval := opts.Interval
	if interval <= 0 {
		interval = 5 * time.Second // Default: poll every 5 seconds
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Scheduler{
		store:       opts.Store,
		repoStarter: opts.RepoStarter,
		interval:    interval,
		logger:      logger,
	}, nil
}

// Name returns the task name for the scheduler.
func (s *Scheduler) Name() string {
	return "batch-scheduler"
}

// Interval returns how often the task should run.
func (s *Scheduler) Interval() time.Duration {
	return s.interval
}

// Run executes one cycle of the batch scheduler.
//
// The scheduler loop:
// 1. Query for batch runs with pending repos (ListBatchRunsWithPendingRepos)
// 2. For each batch, start execution for pending repos via RepoStarter
// 3. Track and log progress
//
// Errors from individual batch processing are logged but don't stop the scheduler.
func (s *Scheduler) Run(ctx context.Context) error {
	if s == nil || s.store == nil || s.repoStarter == nil {
		return nil
	}

	// Find runs with queued repos.
	runIDs, err := s.store.ListRunsWithQueuedRepos(ctx)
	if err != nil {
		s.logger.Error("batch-scheduler: failed to list runs with queued repos", "err", err)
		return nil // Non-fatal; retry on next cycle.
	}

	if len(runIDs) == 0 {
		// No work to do; quiet exit.
		return nil
	}

	s.logger.Debug("batch-scheduler: found runs with pending repos", "count", len(runIDs))

	// Process each batch run.
	// runIDs are KSUID-backed strings from ListBatchRunsWithPendingRepos.
	var totalStarted int
	for _, runID := range runIDs {
		result, err := s.repoStarter.StartPendingRepos(ctx, runID)
		if err != nil {
			s.logger.Error("batch-scheduler: failed to start repos",
				"run_id", runID,
				"err", err,
			)
			continue // Try other runs.
		}

		if result.Started > 0 {
			s.logger.Info("batch-scheduler: started repos",
				"run_id", runID,
				"started", result.Started,
				"already_done", result.AlreadyDone,
				"pending", result.Pending,
			)
			totalStarted += result.Started
		}
	}

	if totalStarted > 0 {
		s.logger.Info("batch-scheduler: cycle completed",
			"runs_processed", len(runIDs),
			"repos_started", totalStarted,
		)
	}

	return nil
}
