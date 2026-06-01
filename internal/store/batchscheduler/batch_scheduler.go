// Package batchscheduler provides a background worker for scheduling queued runs within waves.
//
// The scheduler continuously monitors waves and starts execution for queued runs.
//
// State transitions:
//   - Wave: Started → Finished/Cancelled (terminal)
//   - Run: Queued → Running → Success/Fail/Cancelled (terminal)
//
// The scheduler focuses on per-batch processing without cross-batch FIFO ordering.
package batchscheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// ErrNilStore is returned when New is called with a nil Store.
var ErrNilStore = errors.New("batchscheduler: store is required")

// ErrNilRepoStarter is returned when New is called with a nil RepoStarter.
var ErrNilRepoStarter = errors.New("batchscheduler: repo starter is required")

// StartPendingReposResult contains the result of starting pending runs in a wave.
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
	// StartPendingRepos starts execution for all pending runs in a wave.
	// Returns StartPendingReposResult with counts of started, already done, and pending repos.
	StartPendingRepos(ctx context.Context, waveID types.WaveID) (StartPendingReposResult, error)
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
// Returns ErrNilStore if opts.Store is nil, or ErrNilRepoStarter if opts.RepoStarter is nil.
func New(opts Options) (*Scheduler, error) {
	if opts.Store == nil {
		return nil, ErrNilStore
	}
	if opts.RepoStarter == nil {
		return nil, ErrNilRepoStarter
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

	waveIDs, err := s.store.ListWavesWithQueuedRuns(ctx)
	if err != nil {
		s.logger.Error("batch-scheduler: failed to list waves with queued runs", "err", err)
		return nil // Non-fatal; retry on next cycle.
	}

	if len(waveIDs) == 0 {
		// No work to do; quiet exit.
		return nil
	}

	s.logger.Debug("batch-scheduler: found waves with pending runs", "count", len(waveIDs))

	// Process each batch run.
	// runIDs are KSUID-backed strings from ListBatchRunsWithPendingRepos.
	var totalStarted int
	for _, waveID := range waveIDs {
		result, err := s.repoStarter.StartPendingRepos(ctx, waveID)
		if err != nil {
			s.logger.Error("batch-scheduler: failed to start runs",
				"wave_id", waveID,
				"err", err,
			)
			continue // Try other runs.
		}

		if result.Started > 0 {
			s.logger.Info("batch-scheduler: started repos",
				"wave_id", waveID,
				"started", result.Started,
				"already_done", result.AlreadyDone,
				"pending", result.Pending,
			)
			totalStarted += result.Started
		}
	}

	if totalStarted > 0 {
		s.logger.Info("batch-scheduler: cycle completed",
			"waves_processed", len(waveIDs),
			"repos_started", totalStarted,
		)
	}

	return nil
}
