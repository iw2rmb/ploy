// Package ttlworker provides a background worker for purging expired data from the database.
package ttlworker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// ErrNilStore is returned when New is called with a nil Store.
var ErrNilStore = errors.New("ttlworker: store is required")

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
	// DropPartitions enables dropping entire monthly partitions instead of
	// row-by-row deletion when the operator configures partitioned tables
	// out-of-band (e.g., on logs/events/artifact_bundles). Default: false.
	DropPartitions bool
}

// Worker is a background task that purges expired data from the database.
type Worker struct {
	store          store.Store
	ttl            time.Duration
	interval       time.Duration
	logger         *slog.Logger
	dropPartitions bool
}

// New constructs a new TTL worker.
// Returns ErrNilStore if opts.Store is nil.
func New(opts Options) (*Worker, error) {
	if opts.Store == nil {
		return nil, ErrNilStore
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
		store:          opts.Store,
		ttl:            ttl,
		interval:       interval,
		logger:         logger,
		dropPartitions: opts.DropPartitions,
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

// deleteOp describes a TTL deletion operation.
type deleteOp struct {
	name string
	fn   func(context.Context, pgtype.Timestamptz) (int64, error)
}

// Run executes the TTL cleanup logic.
// Returns an aggregated error if any delete operations fail.
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

	w.logger.Info("ttl-worker: starting cleanup", "cutoff", cutoff.Format(time.RFC3339), "drop_partitions", w.dropPartitions)

	var errs []error

	// Drop old partitions if enabled.
	if w.dropPartitions {
		if err := DropOldPartitions(ctx, w.store.Pool(), w.store, cutoff, w.logger); err != nil {
			w.logger.Error("ttl-worker: partition dropper failed", "err", err)
			errs = append(errs, err)
		}
	}

	// Define delete operations.
	ops := []deleteOp{
		{"logs", w.store.DeleteExpiredLogs},
		{"events", w.store.DeleteExpiredEvents},
		{"diffs", w.store.DeleteExpiredDiffs},
		{"artifact_bundles", w.store.DeleteExpiredArtifactBundles},
	}

	// Execute each delete operation and collect results.
	counts := make(map[string]int64, len(ops))
	for _, op := range ops {
		deleted, err := op.fn(ctx, cutoffTS)
		if err != nil {
			w.logger.Error("ttl-worker: delete expired "+op.name, "err", err)
			errs = append(errs, err)
		} else {
			w.logger.Info("ttl-worker: deleted expired "+op.name, "rows", deleted)
			counts[op.name] = deleted
		}
	}

	w.logger.Info("ttl-worker: cleanup completed",
		"logs", counts["logs"],
		"events", counts["events"],
		"diffs", counts["diffs"],
		"artifacts", counts["artifact_bundles"],
	)

	return errors.Join(errs...)
}
