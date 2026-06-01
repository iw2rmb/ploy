// Package wavescheduler provides a background worker for scheduling queued runs within waves.
//
// The scheduler continuously monitors waves and starts execution for queued runs.
//
// State transitions:
//   - Wave: Started → Finished/Cancelled (terminal)
//   - Run: Queued → Running → Success/Fail/Cancelled (terminal)
//
// The scheduler focuses on per-wave processing without cross-wave FIFO ordering.
package wavescheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// ErrNilStore is returned when New is called with a nil Store.
var ErrNilStore = errors.New("wavescheduler: store is required")

// ErrNilRunStarter is returned when New is called with a nil RunStarter.
var ErrNilRunStarter = errors.New("wavescheduler: run starter is required")

// StartQueuedRunsResult contains the result of starting queued runs in a wave.
// This type is defined here to avoid circular imports with the handlers package.
// It mirrors handlers.StartQueuedRunsResult for interface compatibility.
type StartQueuedRunsResult struct {
	// Started is the number of runs that were successfully started in this call.
	Started int
	// AlreadyDone is the number of runs already in a terminal state.
	AlreadyDone int
	// Pending is the number of runs still queued after this call.
	Pending int
}

// RunStarter is the interface for starting execution of queued runs.
// Implemented by handlers.WaveRunStarter to decouple scheduler from HTTP layer.
type RunStarter interface {
	// StartQueuedRuns starts execution for all queued runs in a wave.
	// Returns StartQueuedRunsResult with counts of started, already done, and queued runs.
	StartQueuedRuns(ctx context.Context, waveID types.WaveID) (StartQueuedRunsResult, error)
}

// Options configures the wave scheduler.
type Options struct {
	// Store is the database store for querying waves and runs.
	Store store.Store
	// RunStarter handles the actual execution start logic.
	RunStarter RunStarter
	// Interval is how often the scheduler checks for queued runs. Default: 5 seconds.
	Interval time.Duration
	// Logger is used for structured logging. If nil, a default logger is used.
	Logger *slog.Logger
}

// Scheduler is a background task that processes queued runs within waves.
// It implements the scheduler.Task interface for integration with the server's
// task scheduler.
type Scheduler struct {
	store      store.Store
	runStarter RunStarter
	interval   time.Duration
	logger     *slog.Logger
}

// New constructs a new wave scheduler.
// Returns ErrNilStore if opts.Store is nil, or ErrNilRunStarter if opts.RunStarter is nil.
func New(opts Options) (*Scheduler, error) {
	if opts.Store == nil {
		return nil, ErrNilStore
	}
	if opts.RunStarter == nil {
		return nil, ErrNilRunStarter
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
		store:      opts.Store,
		runStarter: opts.RunStarter,
		interval:   interval,
		logger:     logger,
	}, nil
}

// Name returns the task name for the scheduler.
func (s *Scheduler) Name() string {
	return "wave-scheduler"
}

// Interval returns how often the task should run.
func (s *Scheduler) Interval() time.Duration {
	return s.interval
}

// Run executes one cycle of the wave scheduler.
//
// The scheduler loop:
// 1. Query for waves with queued runs (ListWavesWithQueuedRuns)
// 2. For each wave, start execution for queued runs via RunStarter
// 3. Track and log progress
//
// Errors from individual wave processing are logged but don't stop the scheduler.
func (s *Scheduler) Run(ctx context.Context) error {
	if s == nil || s.store == nil || s.runStarter == nil {
		return nil
	}

	waveIDs, err := s.store.ListWavesWithQueuedRuns(ctx)
	if err != nil {
		s.logger.Error("wave-scheduler: failed to list waves with queued runs", "err", err)
		return nil // Non-fatal; retry on next cycle.
	}

	if len(waveIDs) == 0 {
		// No work to do; quiet exit.
		return nil
	}

	s.logger.Debug("wave-scheduler: found waves with queued runs", "count", len(waveIDs))

	// Process each wave.
	// waveIDs are KSUID-backed strings from ListWavesWithQueuedRuns.
	var totalStarted int
	for _, waveID := range waveIDs {
		result, err := s.runStarter.StartQueuedRuns(ctx, waveID)
		if err != nil {
			s.logger.Error("wave-scheduler: failed to start runs",
				"wave_id", waveID,
				"err", err,
			)
			continue // Try other runs.
		}

		if result.Started > 0 {
			s.logger.Info("wave-scheduler: started runs",
				"wave_id", waveID,
				"started", result.Started,
				"already_done", result.AlreadyDone,
				"pending", result.Pending,
			)
			totalStarted += result.Started
		}
	}

	if totalStarted > 0 {
		s.logger.Info("wave-scheduler: cycle completed",
			"waves_processed", len(waveIDs),
			"runs_started", totalStarted,
		)
	}

	return nil
}
