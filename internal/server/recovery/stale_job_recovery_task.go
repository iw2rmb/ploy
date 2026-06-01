package recovery

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	defaultStaleJobRecoveryInterval = 30 * time.Second
	defaultNodeStaleAfter           = time.Minute
)

// ErrNilStore is returned when NewStaleJobRecoveryTask is called with nil Store.
var ErrNilStore = errors.New("stale-job-recovery: store is required")

// Options configures stale running-job recovery task behavior.
type Options struct {
	Store          store.Store
	EventsService  *events.Service
	Interval       time.Duration
	NodeStaleAfter time.Duration
	Logger         *slog.Logger
}

// StaleJobRecoveryTask scans for stale Running jobs and reconciles run/wave state.
type StaleJobRecoveryTask struct {
	store          store.Store
	eventsService  *events.Service
	interval       time.Duration
	nodeStaleAfter time.Duration
	logger         *slog.Logger
}

// NewStaleJobRecoveryTask constructs a stale recovery task.
func NewStaleJobRecoveryTask(opts Options) (*StaleJobRecoveryTask, error) {
	if opts.Store == nil {
		return nil, ErrNilStore
	}

	interval := opts.Interval
	if interval <= 0 {
		interval = defaultStaleJobRecoveryInterval
	}

	nodeStaleAfter := opts.NodeStaleAfter
	if nodeStaleAfter <= 0 {
		nodeStaleAfter = defaultNodeStaleAfter
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &StaleJobRecoveryTask{
		store:          opts.Store,
		eventsService:  opts.EventsService,
		interval:       interval,
		nodeStaleAfter: nodeStaleAfter,
		logger:         logger,
	}, nil
}

// Name returns the task name for the scheduler.
func (t *StaleJobRecoveryTask) Name() string {
	return "stale-job-recovery"
}

// Interval returns how often the recovery cycle should run.
func (t *StaleJobRecoveryTask) Interval() time.Duration {
	return t.interval
}

// Run executes one stale-job recovery cycle.
func (t *StaleJobRecoveryTask) Run(ctx context.Context) error {
	if t == nil || t.store == nil {
		return nil
	}

	cutoffTS := pgtype.Timestamptz{
		Time:  time.Now().UTC().Add(-t.nodeStaleAfter),
		Valid: true,
	}

	staleRows, err := t.store.ListStaleRunningJobs(ctx, cutoffTS)
	if err != nil {
		t.logger.Error("stale-job-recovery: list stale running jobs failed", "err", err)
		return nil
	}
	if len(staleRows) == 0 {
		return nil
	}

	var (
		staleNodes    int64
		cancelledJobs int64
		runsUpdated   int
		runsFinalized int
	)

	staleNodes, err = t.store.CountStaleNodesWithRunningJobs(ctx, cutoffTS)
	if err != nil {
		t.logger.Error("stale-job-recovery: count stale nodes failed", "err", err)
		staleNodes = 0
	}

	finalizedRuns := make(map[string]struct{})
	for _, stale := range staleRows {
		affected, err := t.store.CancelActiveJobsByRunAttempt(ctx, store.CancelActiveJobsByRunAttemptParams{
			RunID:   stale.RunID,
			Attempt: stale.Attempt,
		})
		if err != nil {
			t.logger.Error("stale-job-recovery: cancel active jobs failed",
				"run_id", stale.RunID,
				"attempt", stale.Attempt,
				"err", err,
			)
			continue
		}
		cancelledJobs += affected

		runUpdated, err := MaybeUpdateRunStatus(ctx, t.store, stale.RunID, stale.Attempt)
		if err != nil {
			t.logger.Error("stale-job-recovery: reconcile run status failed",
				"run_id", stale.RunID,
				"attempt", stale.Attempt,
				"err", err,
			)
			continue
		}
		if !runUpdated {
			continue
		}
		runsUpdated++
		if _, ok := finalizedRuns[stale.RunID.String()]; ok {
			continue
		}

		run, err := t.store.GetRun(ctx, stale.RunID)
		if err != nil {
			t.logger.Error("stale-job-recovery: load run failed",
				"run_id", stale.RunID,
				"err", err,
			)
			continue
		}

		finalized, err := MaybeCompleteRunIfAllReposTerminal(ctx, t.store, t.eventsService, run)
		if err != nil {
			t.logger.Error("stale-job-recovery: reconcile run status failed",
				"run_id", stale.RunID,
				"attempt", stale.Attempt,
				"err", err,
			)
			continue
		}
		if finalized {
			finalizedRuns[stale.RunID.String()] = struct{}{}
			runsFinalized++
			t.logger.Info("stale-job-recovery: run finalized",
				"run_id", stale.RunID,
				"attempt", stale.Attempt,
			)
		}
	}

	t.logger.Info("stale-job-recovery: cycle completed",
		"stale_nodes", staleNodes,
		"stale_attempts", len(staleRows),
		"runs_updated", runsUpdated,
		"jobs_cancelled", cancelledJobs,
		"runs_finalized", runsFinalized,
	)
	return nil
}
