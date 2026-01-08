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
// The sqlc-generated Queries type will implement the query methods.
type Store interface {
	Querier
	Close()
	Pool() *pgxpool.Pool
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
	config.ConnConfig.RuntimeParams["search_path"] = "ploy,public"

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// Verify connectivity.
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

// ClaimJob atomically claims the next claimable job for a node.
// Requires a non-empty nodeID; returns ErrEmptyNodeID if the nodeID is empty.
// This prevents jobs from entering Running state with node_id=NULL.
func (s *PgStore) ClaimJob(ctx context.Context, nodeID types.NodeID) (Job, error) {
	if nodeID.IsZero() {
		return Job{}, ErrEmptyNodeID
	}
	return s.Queries.ClaimJob(ctx, nodeID)
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
// Returns ErrInvalidJSON if Meta contains invalid JSON bytes.
func (s *PgStore) CreateJob(ctx context.Context, arg CreateJobParams) (Job, error) {
	if err := validateJSONB(arg.Meta); err != nil {
		return Job{}, fmt.Errorf("jobs.meta: %w", err)
	}
	return s.Queries.CreateJob(ctx, arg)
}

// CreateSpec validates the Spec JSONB field and creates a new spec.
// Returns ErrInvalidJSON if Spec contains invalid JSON bytes.
func (s *PgStore) CreateSpec(ctx context.Context, arg CreateSpecParams) (Spec, error) {
	if err := validateJSONB(arg.Spec); err != nil {
		return Spec{}, fmt.Errorf("specs.spec: %w", err)
	}
	return s.Queries.CreateSpec(ctx, arg)
}

// CreateDiff validates the Summary JSONB field and creates a new diff.
// Returns ErrInvalidJSON if Summary contains invalid JSON bytes.
func (s *PgStore) CreateDiff(ctx context.Context, arg CreateDiffParams) (Diff, error) {
	if err := validateJSONB(arg.Summary); err != nil {
		return Diff{}, fmt.Errorf("diffs.summary: %w", err)
	}
	return s.Queries.CreateDiff(ctx, arg)
}

// UpdateJobMeta validates the Meta JSONB field and updates job metadata.
// Returns ErrInvalidJSON if Meta contains invalid JSON bytes.
func (s *PgStore) UpdateJobMeta(ctx context.Context, arg UpdateJobMetaParams) error {
	if err := validateJSONB(arg.Meta); err != nil {
		return fmt.Errorf("jobs.meta: %w", err)
	}
	return s.Queries.UpdateJobMeta(ctx, arg)
}

// UpdateJobCompletionWithMeta validates the Meta JSONB field and completes a job with metadata.
// Returns ErrInvalidJSON if Meta contains invalid JSON bytes.
func (s *PgStore) UpdateJobCompletionWithMeta(ctx context.Context, arg UpdateJobCompletionWithMetaParams) error {
	if err := validateJSONB(arg.Meta); err != nil {
		return fmt.Errorf("jobs.meta: %w", err)
	}
	return s.Queries.UpdateJobCompletionWithMeta(ctx, arg)
}

// UpdateRunCompletion validates the Stats JSONB field and completes a run.
// Returns ErrInvalidJSON if Stats contains invalid JSON bytes.
func (s *PgStore) UpdateRunCompletion(ctx context.Context, arg UpdateRunCompletionParams) error {
	if err := validateJSONB(arg.Stats); err != nil {
		return fmt.Errorf("runs.stats: %w", err)
	}
	return s.Queries.UpdateRunCompletion(ctx, arg)
}
