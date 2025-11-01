// Package ttlworker provides a background worker for purging expired data from the database.
package ttlworker

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// Options configures the TTL worker.
type Options struct {
	// Store is the database store for executing TTL cleanup queries.
	Store store.Store
	// TTL is the time-to-live duration for logs, events, diffs, and artifact bundles.
	// Rows older than TTL will be deleted. Default: 30 days.
	TTL time.Duration
	// Interval is how often the worker runs. Default: 1 hour.
	Interval time.Duration
	// Logger is used for structured logging. If nil, a default logger is used.
	Logger *slog.Logger
}

// Worker is a background task that purges expired data from the database.
type Worker struct {
	store    store.Store
	ttl      time.Duration
	interval time.Duration
	logger   *slog.Logger
}

// New constructs a new TTL worker.
func New(opts Options) (*Worker, error) {
	if opts.Store == nil {
		return nil, nil
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour // 30 days default
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = time.Hour // 1 hour default
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		store:    opts.Store,
		ttl:      ttl,
		interval: interval,
		logger:   logger,
	}, nil
}

// Name returns the task name for the scheduler.
func (w *Worker) Name() string {
	return "ttl-worker"
}

// Interval returns how often the task should run.
func (w *Worker) Interval() time.Duration {
	return w.interval
}

// Run executes the TTL cleanup logic.
func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.store == nil {
		return nil
	}

    cutoff := time.Now().Add(-w.ttl)
    // cutoffTS follows initialism casing rules (ST1003).
    cutoffTS := pgtype.Timestamptz{
        Time:  cutoff,
        Valid: true,
    }

	w.logger.Info("ttl-worker: starting cleanup", "cutoff", cutoff.Format(time.RFC3339))

	// Delete expired logs.
    logsDeleted, err := w.store.DeleteExpiredLogs(ctx, cutoffTS)
	if err != nil {
		w.logger.Error("ttl-worker: delete expired logs", "err", err)
	} else {
		w.logger.Info("ttl-worker: deleted expired logs", "rows", logsDeleted)
	}

	// Delete expired events.
    eventsDeleted, err := w.store.DeleteExpiredEvents(ctx, cutoffTS)
	if err != nil {
		w.logger.Error("ttl-worker: delete expired events", "err", err)
	} else {
		w.logger.Info("ttl-worker: deleted expired events", "rows", eventsDeleted)
	}

	// Delete expired diffs.
    diffsDeleted, err := w.store.DeleteExpiredDiffs(ctx, cutoffTS)
	if err != nil {
		w.logger.Error("ttl-worker: delete expired diffs", "err", err)
	} else {
		w.logger.Info("ttl-worker: deleted expired diffs", "rows", diffsDeleted)
	}

	// Delete expired artifact bundles.
    artifactsDeleted, err := w.store.DeleteExpiredArtifactBundles(ctx, cutoffTS)
	if err != nil {
		w.logger.Error("ttl-worker: delete expired artifact bundles", "err", err)
	} else {
		w.logger.Info("ttl-worker: deleted expired artifact bundles", "rows", artifactsDeleted)
	}

	w.logger.Info("ttl-worker: cleanup completed",
		"logs", logsDeleted,
		"events", eventsDeleted,
		"diffs", diffsDeleted,
		"artifacts", artifactsDeleted,
	)

	return nil
}
