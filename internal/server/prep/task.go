package prep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/iw2rmb/ploy/internal/server/scheduler"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	defaultTaskInterval      = 30 * time.Second
	defaultMaxAttempts       = 3
	defaultTaskRetryDelay    = 30 * time.Second
	taskNamePrepOrchestrator = "prep-orchestrator"
)

var (
	// ErrNilStore is returned when prep task is constructed without a store.
	ErrNilStore = errors.New("prep task: store is required")
	// ErrNilRunner is returned when prep task is constructed without a runner.
	ErrNilRunner = errors.New("prep task: runner is required")
)

// Ensure Task implements scheduler.Task.
var _ scheduler.Task = (*Task)(nil)

// Options configures prep task behavior.
type Options struct {
	Store       store.Store
	Runner      Runner
	Interval    time.Duration
	MaxAttempts int
	RetryDelay  time.Duration
	Logger      *slog.Logger
}

// Task polls prep queues, executes attempts, and persists state transitions.
type Task struct {
	store       store.Store
	runner      Runner
	interval    time.Duration
	maxAttempts int32
	retryDelay  time.Duration
	logger      *slog.Logger
}

// NewTask constructs a prep task.
func NewTask(opts Options) (*Task, error) {
	if opts.Store == nil {
		return nil, ErrNilStore
	}
	if opts.Runner == nil {
		return nil, ErrNilRunner
	}

	interval := opts.Interval
	if interval <= 0 {
		interval = defaultTaskInterval
	}

	maxAttempts := opts.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = defaultMaxAttempts
	}

	retryDelay := opts.RetryDelay
	if retryDelay <= 0 {
		retryDelay = defaultTaskRetryDelay
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Task{
		store:       opts.Store,
		runner:      opts.Runner,
		interval:    interval,
		maxAttempts: int32(maxAttempts),
		retryDelay:  retryDelay,
		logger:      logger,
	}, nil
}

// Name returns scheduler task name.
func (t *Task) Name() string {
	return taskNamePrepOrchestrator
}

// Interval returns scheduler interval.
func (t *Task) Interval() time.Duration {
	return t.interval
}

// Run executes one scheduler cycle.
func (t *Task) Run(ctx context.Context) error {
	if t == nil || t.store == nil || t.runner == nil {
		return nil
	}

	processed := 0
	for {
		if ctx.Err() != nil {
			return nil
		}

		repo, found, err := t.claimNextRepo(ctx)
		if err != nil {
			t.logger.Error("prep: claim next repo failed", "err", err)
			return nil
		}
		if !found {
			break
		}

		processed++
		if err := t.processRepo(ctx, repo); err != nil {
			t.logger.Error("prep: process repo failed", "repo_id", repo.ID, "attempt", repo.PrepAttempts, "err", err)
		}
	}

	if processed > 0 {
		t.logger.Info("prep: cycle completed", "repos_processed", processed)
	}

	return nil
}

func (t *Task) claimNextRepo(ctx context.Context) (store.MigRepo, bool, error) {
	repo, err := t.store.ClaimNextPrepRepo(ctx)
	if err == nil {
		return repo, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.MigRepo{}, false, err
	}

	cutoff := pgtype.Timestamptz{Time: time.Now().UTC().Add(-t.retryDelay), Valid: true}
	repo, err = t.store.ClaimNextPrepRetryRepo(ctx, cutoff)
	if err == nil {
		return repo, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return store.MigRepo{}, false, nil
	}
	return store.MigRepo{}, false, err
}

func (t *Task) processRepo(ctx context.Context, repo store.MigRepo) error {
	attempt := repo.PrepAttempts
	initialResult := marshalOrFallback(map[string]any{
		"stage":   "init",
		"repo_id": repo.ID,
		"attempt": attempt,
	})
	_, err := t.store.CreatePrepRun(ctx, store.CreatePrepRunParams{
		RepoID:     repo.ID,
		Attempt:    attempt,
		Status:     store.PrepStatusRunning,
		ResultJson: initialResult,
		LogsRef:    nil,
	})
	if err != nil {
		return t.persistFailureNoRun(ctx, repo, attempt, err.Error(), FailureCodeUnknown)
	}

	runResult, runErr := t.runner.Run(ctx, RunRequest{Repo: repo, Attempt: attempt})
	if runErr != nil {
		failureCode := FailureCodeUnknown
		resultJSON := marshalOrFallback(map[string]any{"error": runErr.Error()})
		logsRef := (*string)(nil)
		if re := (*RunError)(nil); errors.As(runErr, &re) {
			failureCode = normalizeFailureCode(re.FailureCode)
			if len(re.ResultJSON) > 0 {
				resultJSON = re.ResultJSON
			}
			logsRef = re.LogsRef
		}
		if failureCode == FailureCodeUnknown {
			failureCode = classifyRunError(runErr)
		}
		return t.persistFailureWithRun(ctx, repo, attempt, runErr.Error(), failureCode, resultJSON, logsRef)
	}

	if err := validateProfileJSON(runResult.ProfileJSON); err != nil {
		resultJSON := runResult.ResultJSON
		if len(resultJSON) == 0 {
			resultJSON = marshalOrFallback(map[string]any{"error": err.Error()})
		}
		return t.persistFailureWithRun(ctx, repo, attempt, err.Error(), FailureCodeUnknown, resultJSON, runResult.LogsRef)
	}

	if len(runResult.ResultJSON) == 0 {
		runResult.ResultJSON = marshalOrFallback(map[string]any{
			"stage":   "codex",
			"repo_id": repo.ID,
			"attempt": attempt,
		})
	}

	prepArtifacts := marshalOrFallback(map[string]any{
		"logs_ref": runResult.LogsRef,
		"attempt":  attempt,
	})

	return t.persistSuccessWithRun(ctx, repo, attempt, runResult.ProfileJSON, prepArtifacts, runResult.ResultJSON, runResult.LogsRef)
}

func (t *Task) persistSuccessWithRun(ctx context.Context, repo store.MigRepo, attempt int32, profileJSON, artifactsJSON, resultJSON []byte, logsRef *string) error {
	return t.withWriter(ctx, func(w prepWriter) error {
		if _, err := w.FinishPrepRun(ctx, store.FinishPrepRunParams{
			RepoID:     repo.ID,
			Attempt:    attempt,
			Status:     store.PrepStatusReady,
			ResultJson: resultJSON,
			LogsRef:    logsRef,
		}); err != nil {
			return fmt.Errorf("finish prep run: %w", err)
		}
		if err := w.SaveMigRepoPrepProfile(ctx, store.SaveMigRepoPrepProfileParams{
			ID:            repo.ID,
			PrepProfile:   profileJSON,
			PrepArtifacts: artifactsJSON,
		}); err != nil {
			return fmt.Errorf("save prep profile: %w", err)
		}
		return nil
	})
}

func (t *Task) persistFailureNoRun(ctx context.Context, repo store.MigRepo, attempt int32, errMsg, failureCode string) error {
	nextState := t.nextFailureState(attempt)
	failureCode = normalizeFailureCode(failureCode)
	return t.store.UpdateMigRepoPrepState(ctx, store.UpdateMigRepoPrepStateParams{
		ID:              repo.ID,
		PrepStatus:      nextState,
		PrepLastError:   strPtr(errMsg),
		PrepFailureCode: strPtr(failureCode),
	})
}

func (t *Task) persistFailureWithRun(ctx context.Context, repo store.MigRepo, attempt int32, errMsg, failureCode string, resultJSON []byte, logsRef *string) error {
	nextState := t.nextFailureState(attempt)
	failureCode = normalizeFailureCode(failureCode)
	return t.withWriter(ctx, func(w prepWriter) error {
		if _, err := w.FinishPrepRun(ctx, store.FinishPrepRunParams{
			RepoID:     repo.ID,
			Attempt:    attempt,
			Status:     store.PrepStatusFailed,
			ResultJson: resultJSON,
			LogsRef:    logsRef,
		}); err != nil {
			return fmt.Errorf("finish prep run: %w", err)
		}
		if err := w.UpdateMigRepoPrepState(ctx, store.UpdateMigRepoPrepStateParams{
			ID:              repo.ID,
			PrepStatus:      nextState,
			PrepLastError:   strPtr(errMsg),
			PrepFailureCode: strPtr(failureCode),
		}); err != nil {
			return fmt.Errorf("update prep state: %w", err)
		}
		return nil
	})
}

func (t *Task) nextFailureState(attempt int32) store.PrepStatus {
	if attempt < t.maxAttempts {
		return store.PrepStatusRetryScheduled
	}
	return store.PrepStatusFailed
}

type prepWriter interface {
	FinishPrepRun(ctx context.Context, arg store.FinishPrepRunParams) (store.PrepRun, error)
	SaveMigRepoPrepProfile(ctx context.Context, arg store.SaveMigRepoPrepProfileParams) error
	UpdateMigRepoPrepState(ctx context.Context, arg store.UpdateMigRepoPrepStateParams) error
}

func (t *Task) withWriter(ctx context.Context, fn func(prepWriter) error) error {
	pool := t.store.Pool()
	if pool == nil {
		return fn(t.store)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	qtx := store.New(tx)
	if err := fn(qtx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	committed = true
	return nil
}

func marshalOrFallback(payload map[string]any) []byte {
	b, err := json.Marshal(payload)
	if err != nil {
		return []byte(`{"error":"failed to marshal prep payload"}`)
	}
	return b
}

func classifyRunError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureCodeTimeout
	}
	return FailureCodeUnknown
}
