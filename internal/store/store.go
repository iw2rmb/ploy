// Package store provides PostgreSQL-backed data persistence using pgx and sqlc.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrEmptyNodeID is returned when ClaimJob is called with an empty NodeID.
var ErrEmptyNodeID = errors.New("store: ClaimJob requires non-empty nodeID")

// ErrInvalidJSON is returned when a JSONB column receives invalid JSON bytes.
var ErrInvalidJSON = errors.New("store: invalid JSON for JSONB column")

// ErrRunRestartActive is returned when a non-terminal run is restarted.
var ErrRunRestartActive = errors.New("store: only terminal runs can be restarted")

// ErrRunRestartWaveCancelled is returned when the owning wave was cancelled.
var ErrRunRestartWaveCancelled = errors.New("store: cannot restart a run in a cancelled wave")

// ErrBootstrapTokenInvalid is returned when a bootstrap token is missing,
// revoked, already consumed, or bound to a different node.
var ErrBootstrapTokenInvalid = errors.New("store: bootstrap token invalid")

// ErrBootstrapNodeExists is returned when bootstrap completion targets an
// already-enrolled node ID.
var ErrBootstrapNodeExists = errors.New("store: bootstrap node already exists")

// Store defines the interface for database operations.
// The sqlc-generated Queries type implements the query methods via Querier.
type Store interface {
	Querier
	CancelRun(ctx context.Context, runID types.RunID) error
	CancelWave(ctx context.Context, waveID types.WaveID) error
	CompleteBootstrapEnrollment(ctx context.Context, arg CompleteBootstrapEnrollmentParams) error
	CreateWaveWithRuns(ctx context.Context, arg CreateWaveWithRunsParams) (Wave, []RunMaterialization, error)
	RestartRun(ctx context.Context, arg RestartRunParams) (Run, error)
	Close()
	Pool() *pgxpool.Pool
}

// CompleteBootstrapEnrollmentParams contains the certificate metadata recorded
// when a one-time bootstrap token successfully enrolls a new node.
type CompleteBootstrapEnrollmentParams struct {
	TokenID         string
	NodeID          types.NodeID
	CertSerial      string
	CertFingerprint string
	CertNotBefore   time.Time
	CertNotAfter    time.Time
}

// JobPlan contains intrinsic job data. Its position in a Jobs slice defines
// the initial status and next-job link when the store materializes the chain.
type JobPlan struct {
	Name     string
	JobType  types.JobType
	JobImage string
	Meta     []byte
}

// RunPlan contains repository-specific run data and its reusable job template.
// CreateWaveWithRuns supplies wave-wide fields and materializes job identities.
type RunPlan struct {
	ID              types.RunID
	RepoID          types.RepoID
	RepoBaseRef     string
	SourceCommitSha string
	RepoSha0        string
	Stats           []byte
	Jobs            []JobPlan
}

// RunMaterialization contains the persisted run and its ordered job chain.
type RunMaterialization struct {
	Run  Run
	Jobs []Job
}

// CreateWaveWithRunsParams contains the complete durable materialization for one launch.
type CreateWaveWithRunsParams struct {
	Wave CreateWaveParams
	Runs []RunPlan
}

type RestartRunParams struct {
	RunID           types.RunID
	ExpectedAttempt int32
	Stats           []byte
	Jobs            []JobPlan
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

// CompleteBootstrapEnrollment atomically consumes one bootstrap token and
// creates the node it was minted for.
func (s *PgStore) CompleteBootstrapEnrollment(ctx context.Context, arg CompleteBootstrapEnrollmentParams) error {
	if arg.TokenID == "" || arg.NodeID.IsZero() {
		return ErrBootstrapTokenInvalid
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("complete bootstrap enrollment: begin tx: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	qtx := s.Queries.WithTx(tx)

	var storedNodeID pgtype.Text
	var usedAt pgtype.Timestamptz
	var revokedAt pgtype.Timestamptz
	err = tx.QueryRow(ctx, `
SELECT node_id, used_at, revoked_at
FROM bootstrap_tokens
WHERE token_id = $1
FOR UPDATE
`, arg.TokenID).Scan(&storedNodeID, &usedAt, &revokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBootstrapTokenInvalid
		}
		return fmt.Errorf("complete bootstrap enrollment: lock bootstrap token: %w", err)
	}
	if !storedNodeID.Valid || storedNodeID.String != arg.NodeID.String() || usedAt.Valid || revokedAt.Valid {
		return ErrBootstrapTokenInvalid
	}

	var existing int
	err = tx.QueryRow(ctx, `SELECT 1 FROM nodes WHERE id = $1 LIMIT 1`, arg.NodeID).Scan(&existing)
	if err == nil {
		return ErrBootstrapNodeExists
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("complete bootstrap enrollment: check node: %w", err)
	}

	ipAddr, _ := netip.ParseAddr("0.0.0.0")
	if _, err := qtx.CreateNode(ctx, CreateNodeParams{
		ID:          arg.NodeID,
		Name:        "node-" + arg.NodeID.String(),
		IpAddress:   ipAddr,
		Version:     nil,
		Concurrency: 1,
	}); err != nil {
		if isNodeIdentityUniqueViolation(err) {
			return ErrBootstrapNodeExists
		}
		return fmt.Errorf("complete bootstrap enrollment: create node: %w", err)
	}

	if err := qtx.UpdateNodeCertMetadata(ctx, UpdateNodeCertMetadataParams{
		ID:              arg.NodeID,
		CertSerial:      &arg.CertSerial,
		CertFingerprint: &arg.CertFingerprint,
		CertNotBefore:   pgtype.Timestamptz{Time: arg.CertNotBefore, Valid: true},
		CertNotAfter:    pgtype.Timestamptz{Time: arg.CertNotAfter, Valid: true},
	}); err != nil {
		return fmt.Errorf("complete bootstrap enrollment: update cert metadata: %w", err)
	}

	if _, err := tx.Exec(ctx, `
UPDATE bootstrap_tokens
SET used_at = NOW(), cert_issued_at = NOW()
WHERE token_id = $1
`, arg.TokenID); err != nil {
		return fmt.Errorf("complete bootstrap enrollment: consume bootstrap token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("complete bootstrap enrollment: commit tx: %w", err)
	}

	committed = true
	return nil
}

func isNodeIdentityUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	return pgErr.ConstraintName == "nodes_pkey" || pgErr.ConstraintName == "nodes_name_key"
}

// CreateWaveWithRuns atomically creates a wave and every selected run job chain.
func (s *PgStore) CreateWaveWithRuns(ctx context.Context, arg CreateWaveWithRunsParams) (Wave, []RunMaterialization, error) {
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

	materialized := make([]RunMaterialization, 0, len(arg.Runs))
	for _, runPlan := range arg.Runs {
		run, err := insertRun(ctx, qtx, arg.Wave, runPlan)
		if err != nil {
			return Wave{}, nil, fmt.Errorf("create wave with runs: create run %s: %w", runPlan.ID, err)
		}
		jobs, err := createPlannedJobs(ctx, qtx, run, runPlan.Jobs)
		if err != nil {
			return Wave{}, nil, fmt.Errorf("create wave with runs: create jobs for run %s: %w", run.ID, err)
		}
		materialized = append(materialized, RunMaterialization{Run: run, Jobs: jobs})
	}

	if err := tx.Commit(ctx); err != nil {
		return Wave{}, nil, fmt.Errorf("create wave with runs: commit tx: %w", err)
	}

	committed = true
	return wave, materialized, nil
}

// Keep run insertion private so every production caller must materialize the
// run and its job chain through one transaction.
func insertRun(ctx context.Context, q *Queries, wave CreateWaveParams, arg RunPlan) (Run, error) {
	if err := validateJSONB(arg.Stats); err != nil {
		return Run{}, fmt.Errorf("runs.stats: %w", err)
	}
	row := q.db.QueryRow(ctx, `
		INSERT INTO runs (
			id, wave_id, mig_id, spec_id, repo_id, repo_base_ref,
			source_commit_sha, repo_sha0, created_by, status, started_at, stats
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'Running', now(), COALESCE($10::jsonb, '{}'::jsonb))
		RETURNING id, wave_id, mig_id, spec_id, repo_id, repo_base_ref, source_commit_sha, repo_sha0,
			created_by, status, attempt, last_error, created_at, started_at, finished_at, stats
	`, arg.ID, wave.ID, wave.MigID, wave.SpecID, arg.RepoID, arg.RepoBaseRef,
		arg.SourceCommitSha, arg.RepoSha0, wave.CreatedBy, arg.Stats)

	var run Run
	err := row.Scan(
		&run.ID,
		&run.WaveID,
		&run.MigID,
		&run.SpecID,
		&run.RepoID,
		&run.RepoBaseRef,
		&run.SourceCommitSha,
		&run.RepoSha0,
		&run.CreatedBy,
		&run.Status,
		&run.Attempt,
		&run.LastError,
		&run.CreatedAt,
		&run.StartedAt,
		&run.FinishedAt,
		&run.Stats,
	)
	return run, err
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

// RestartRun atomically resets one terminal run and creates its next-attempt job chain.
func (s *PgStore) RestartRun(ctx context.Context, arg RestartRunParams) (Run, error) {
	runID := arg.RunID
	stats := arg.Stats
	if len(stats) == 0 {
		stats = nil
	}
	if err := validateJSONB(stats); err != nil {
		return Run{}, fmt.Errorf("restart run: runs.stats: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Run{}, fmt.Errorf("restart run: begin tx: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	qtx := s.Queries.WithTx(tx)

	run, err := qtx.GetRunForUpdate(ctx, runID)
	if err != nil {
		return Run{}, fmt.Errorf("restart run: get run: %w", err)
	}
	if run.Attempt != arg.ExpectedAttempt || (run.Status != types.RunStatusSuccess && run.Status != types.RunStatusFail && run.Status != types.RunStatusCancelled) {
		return Run{}, ErrRunRestartActive
	}

	wave, err := qtx.GetWave(ctx, run.WaveID)
	if err != nil {
		return Run{}, fmt.Errorf("restart run: get wave: %w", err)
	}
	if wave.Status == types.WaveStatusCancelled {
		return Run{}, ErrRunRestartWaveCancelled
	}

	if _, err := qtx.CancelActiveJobsByRunAttempt(ctx, CancelActiveJobsByRunAttemptParams{
		RunID:   runID,
		Attempt: run.Attempt,
	}); err != nil {
		return Run{}, fmt.Errorf("restart run: cancel active jobs: %w", err)
	}

	if err := incrementRunAttempt(ctx, qtx, runID, stats); err != nil {
		return Run{}, fmt.Errorf("restart run: increment attempt: %w", err)
	}

	if wave.Status == types.WaveStatusFinished {
		if err := qtx.UpdateWaveStatus(ctx, UpdateWaveStatusParams{ID: wave.ID, Status: types.WaveStatusStarted}); err != nil {
			return Run{}, fmt.Errorf("restart run: revive wave: %w", err)
		}
	}

	updated, err := qtx.GetRun(ctx, runID)
	if err != nil {
		return Run{}, fmt.Errorf("restart run: reload run: %w", err)
	}
	if _, err := createPlannedJobs(ctx, qtx, updated, arg.Jobs); err != nil {
		return Run{}, fmt.Errorf("restart run: create jobs: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Run{}, fmt.Errorf("restart run: commit tx: %w", err)
	}

	committed = true
	return updated, nil
}

func incrementRunAttempt(ctx context.Context, q *Queries, runID types.RunID, stats []byte) error {
	_, err := q.db.Exec(ctx, `
		UPDATE runs
		SET attempt = attempt + 1,
			status = 'Running',
			last_error = NULL,
			started_at = now(),
			finished_at = NULL,
			stats = COALESCE($2, '{}'::jsonb)
		WHERE id = $1
	`, runID, stats)
	return err
}

func createPlannedJobs(ctx context.Context, q *Queries, run Run, jobs []JobPlan) ([]Job, error) {
	if len(jobs) == 0 {
		return nil, errors.New("jobs required")
	}
	for i, job := range jobs {
		if err := validateJSONB(job.Meta); err != nil {
			return nil, fmt.Errorf("job %d meta: %w", i, err)
		}
	}
	jobIDs := make([]types.JobID, len(jobs))
	for i := range jobIDs {
		jobIDs[i] = types.NewJobID()
	}
	materialized := make([]Job, len(jobs))
	// Insert tail-first because jobs.next_id references a row in the same table.
	for i := len(jobs) - 1; i >= 0; i-- {
		job := jobs[i]
		status := types.JobStatusCreated
		if i == 0 {
			status = types.JobStatusQueued
		}
		var nextID *types.JobID
		if i+1 < len(jobs) {
			nextID = &jobIDs[i+1]
		}
		repoSHAIn := ""
		if i == 0 {
			repoSHAIn = run.RepoSha0
		}
		created, err := q.CreateJob(ctx, CreateJobParams{
			ID:          jobIDs[i],
			RunID:       run.ID,
			RepoID:      run.RepoID,
			RepoBaseRef: run.RepoBaseRef,
			Attempt:     run.Attempt,
			Status:      status,
			JobType:     job.JobType,
			JobImage:    job.JobImage,
			NextID:      nextID,
			Name:        job.Name,
			Meta:        job.Meta,
			RepoShaIn:   repoSHAIn,
		})
		if err != nil {
			return nil, fmt.Errorf("create job %q type=%s id=%s: %w", job.Name, job.JobType, jobIDs[i], err)
		}
		materialized[i] = created
	}
	return materialized, nil
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

// CreateGitSpecSnapshot validates JSONB fields and creates an immutable snapshot.
func (s *PgStore) CreateGitSpecSnapshot(ctx context.Context, arg CreateGitSpecSnapshotParams) (Spec, error) {
	if err := validateJSONB(arg.Source); err != nil {
		return Spec{}, fmt.Errorf("specs.source: %w", err)
	}
	if err := validateJSONB(arg.Spec); err != nil {
		return Spec{}, fmt.Errorf("specs.spec: %w", err)
	}
	return s.Queries.CreateGitSpecSnapshot(ctx, arg)
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
