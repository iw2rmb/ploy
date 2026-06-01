// Package store provides PostgreSQL-backed data persistence using pgx and sqlc.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrEmptyNodeID is returned when ClaimJob is called with an empty NodeID.
var ErrEmptyNodeID = errors.New("store: ClaimJob requires non-empty nodeID")

// ErrInvalidJSON is returned when a JSONB column receives invalid JSON bytes.
var ErrInvalidJSON = errors.New("store: invalid JSON for JSONB column")

// Store defines the interface for database operations.
// The sqlc-generated Queries type implements the query methods via Querier.
type Store interface {
	Querier
	CancelRun(ctx context.Context, runID types.RunID) error
	CancelWave(ctx context.Context, waveID types.WaveID) error
	CreateWaveWithRuns(ctx context.Context, arg CreateWaveWithRunsParams) (Wave, []Run, error)
	Close()
	Pool() *pgxpool.Pool
}

// CreateWaveWithRunsParams contains the complete DB materialization for one launch.
type CreateWaveWithRunsParams struct {
	Wave CreateWaveParams
	Runs []CreateRunParams
}

// PgStore wraps a pgxpool connection pool and implements Store.
type PgStore struct {
	pool *pgxpool.Pool
	*Queries
}

// NewStore creates a new Store by establishing a connection pool to the PostgreSQL database.
// The dsn parameter should be a valid PostgreSQL connection string.
// Callers must call Close() when done to release resources.
func NewStore(ctx context.Context, dsn string) (Store, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	// Set search_path so unqualified table names resolve to the ploy schema.
	// This ensures correctness regardless of DSN formatting.
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = make(map[string]string)
	}
	config.ConnConfig.RuntimeParams["search_path"] = "ploy, public"

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &PgStore{
		pool:    pool,
		Queries: New(pool),
	}, nil
}

// Close releases all resources held by the store.
func (s *PgStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// Pool returns the underlying connection pool.
// This is useful for operations that need direct pool access,
// such as partition management.
func (s *PgStore) Pool() *pgxpool.Pool {
	return s.pool
}

// CreateWaveWithRuns atomically creates a wave and its selected run rows.
func (s *PgStore) CreateWaveWithRuns(ctx context.Context, arg CreateWaveWithRunsParams) (Wave, []Run, error) {
	if len(arg.Runs) == 0 {
		return Wave{}, nil, errors.New("create wave with runs: runs required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Wave{}, nil, fmt.Errorf("create wave with runs: begin tx: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	qtx := s.Queries.WithTx(tx)

	wave, err := qtx.CreateWave(ctx, arg.Wave)
	if err != nil {
		return Wave{}, nil, fmt.Errorf("create wave with runs: create wave: %w", err)
	}

	runs := make([]Run, 0, len(arg.Runs))
	for _, runParams := range arg.Runs {
		run, err := qtx.CreateRun(ctx, runParams)
		if err != nil {
			return Wave{}, nil, fmt.Errorf("create wave with runs: create run %s: %w", runParams.ID, err)
		}
		runs = append(runs, run)
	}

	if err := tx.Commit(ctx); err != nil {
		return Wave{}, nil, fmt.Errorf("create wave with runs: commit tx: %w", err)
	}

	committed = true
	return wave, runs, nil
}

// CancelRun atomically cancels one run and all active child jobs.
func (s *PgStore) CancelRun(ctx context.Context, runID types.RunID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cancel run: begin tx: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	qtx := s.Queries.WithTx(tx)

	run, err := qtx.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("cancel run: get run: %w", err)
	}

	if run.Status != types.RunStatusSuccess && run.Status != types.RunStatusFail && run.Status != types.RunStatusCancelled {
		if err := qtx.UpdateRunStatus(ctx, UpdateRunStatusParams{
			ID:     runID,
			Status: types.RunStatusCancelled,
		}); err != nil {
			return fmt.Errorf("cancel run: update run status: %w", err)
		}
	}

	if _, err := qtx.CancelActiveJobsByRun(ctx, runID); err != nil {
		return fmt.Errorf("cancel run: cancel active jobs: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cancel run: commit tx: %w", err)
	}

	committed = true
	return nil
}

// CancelWave atomically cancels one wave and all active child runs/jobs.
func (s *PgStore) CancelWave(ctx context.Context, waveID types.WaveID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cancel wave: begin tx: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	qtx := s.Queries.WithTx(tx)
	wave, err := qtx.GetWave(ctx, waveID)
	if err != nil {
		return fmt.Errorf("cancel wave: get wave: %w", err)
	}
	if wave.Status != types.WaveStatusFinished && wave.Status != types.WaveStatusCancelled {
		if err := qtx.UpdateWaveStatus(ctx, UpdateWaveStatusParams{ID: waveID, Status: types.WaveStatusCancelled}); err != nil {
			return fmt.Errorf("cancel wave: update wave status: %w", err)
		}
	}
	if _, err := qtx.CancelActiveRunsByWave(ctx, waveID); err != nil {
		return fmt.Errorf("cancel wave: cancel active runs: %w", err)
	}
	runs, err := qtx.ListRunsByWave(ctx, waveID)
	if err != nil {
		return fmt.Errorf("cancel wave: list runs: %w", err)
	}
	for _, run := range runs {
		if _, err := qtx.CancelActiveJobsByRun(ctx, run.ID); err != nil {
			return fmt.Errorf("cancel wave: cancel jobs for run %s: %w", run.ID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cancel wave: commit tx: %w", err)
	}
	committed = true
	return nil
}

// ClaimJob atomically claims the next claimable job for a node.
// Requires a non-empty nodeID; returns ErrEmptyNodeID if the nodeID is empty.
// This prevents jobs from entering Running state with node_id=NULL.
func (s *PgStore) ClaimJob(ctx context.Context, nodeID types.NodeID) (Job, error) {
	if nodeID.IsZero() {
		return Job{}, ErrEmptyNodeID
	}
	return s.Queries.ClaimJob(ctx, nodeID)
}

// UnclaimJob reverts a claimed Running job back to claimable Queued state.
// The update is guarded by both job ID and node ID to avoid stealing claims.
func (s *PgStore) UnclaimJob(ctx context.Context, arg UnclaimJobParams) error {
	if arg.ID.IsZero() {
		return errors.New("store: UnclaimJob requires non-empty job ID")
	}
	if arg.NodeID.IsZero() {
		return errors.New("store: UnclaimJob requires non-empty node ID")
	}
	if err := s.Queries.UnclaimJob(ctx, arg); err != nil {
		return fmt.Errorf("unclaim job: %w", err)
	}
	return nil
}

// validateJSONB validates that non-empty byte slices contain valid JSON.
// Empty/nil slices are allowed (treated as NULL in the database).
func validateJSONB(raw []byte) error {
	if len(raw) > 0 && !json.Valid(raw) {
		return ErrInvalidJSON
	}
	return nil
}

// CreateJob validates the Meta JSONB field and creates a new job.
func (s *PgStore) CreateJob(ctx context.Context, arg CreateJobParams) (Job, error) {
	if err := validateJSONB(arg.Meta); err != nil {
		return Job{}, fmt.Errorf("jobs.meta: %w", err)
	}
	return s.Queries.CreateJob(ctx, arg)
}

// CreateSpec validates the Spec JSONB field and creates a new spec.
func (s *PgStore) CreateSpec(ctx context.Context, arg CreateSpecParams) (Spec, error) {
	if err := validateJSONB(arg.Spec); err != nil {
		return Spec{}, fmt.Errorf("specs.spec: %w", err)
	}
	return s.Queries.CreateSpec(ctx, arg)
}

// CreateDiff validates the Summary JSONB field and creates a new diff.
func (s *PgStore) CreateDiff(ctx context.Context, arg CreateDiffParams) (Diff, error) {
	if err := validateJSONB(arg.Summary); err != nil {
		return Diff{}, fmt.Errorf("diffs.summary: %w", err)
	}
	return s.Queries.CreateDiff(ctx, arg)
}

// UpsertNodeDiagnostic validates the Details JSONB field and stores daemon state.
func (s *PgStore) UpsertNodeDiagnostic(ctx context.Context, arg UpsertNodeDiagnosticParams) (NodeDiagnostic, error) {
	if err := validateJSONB(arg.Details); err != nil {
		return NodeDiagnostic{}, fmt.Errorf("node_diagnostics.details: %w", err)
	}
	return s.Queries.UpsertNodeDiagnostic(ctx, arg)
}

// CreateNodeAction validates metadata before enqueueing node-scoped maintenance.
func (s *PgStore) CreateNodeAction(ctx context.Context, arg CreateNodeActionParams) (NodeAction, error) {
	if err := validateJSONB(arg.Meta); err != nil {
		return NodeAction{}, fmt.Errorf("node_actions.meta: %w", err)
	}
	return s.Queries.CreateNodeAction(ctx, arg)
}

// UpdateNodeActionCompletion validates action result JSON before persistence.
func (s *PgStore) UpdateNodeActionCompletion(ctx context.Context, arg UpdateNodeActionCompletionParams) error {
	if err := validateJSONB(arg.Result); err != nil {
		return fmt.Errorf("node_actions.result: %w", err)
	}
	return s.Queries.UpdateNodeActionCompletion(ctx, arg)
}

// UpdateJobMeta validates the Meta JSONB field and updates job metadata.
func (s *PgStore) UpdateJobMeta(ctx context.Context, arg UpdateJobMetaParams) error {
	if err := validateJSONB(arg.Meta); err != nil {
		return fmt.Errorf("jobs.meta: %w", err)
	}
	return s.Queries.UpdateJobMeta(ctx, arg)
}

// UpdateJobCompletionWithMeta validates the Meta JSONB field and completes a job with metadata.
func (s *PgStore) UpdateJobCompletionWithMeta(ctx context.Context, arg UpdateJobCompletionWithMetaParams) error {
	if err := validateJSONB(arg.Meta); err != nil {
		return fmt.Errorf("jobs.meta: %w", err)
	}
	return s.Queries.UpdateJobCompletionWithMeta(ctx, arg)
}

// UpdateWaveCompletion validates the Stats JSONB field and completes a wave.
func (s *PgStore) UpdateWaveCompletion(ctx context.Context, arg UpdateWaveCompletionParams) error {
	if err := validateJSONB(arg.Stats); err != nil {
		return fmt.Errorf("waves.stats: %w", err)
	}
	return s.Queries.UpdateWaveCompletion(ctx, arg)
}
