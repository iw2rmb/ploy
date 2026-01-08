// Package store provides PostgreSQL-backed data persistence using pgx and sqlc.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrEmptyNodeID is returned when ClaimJob is called with an empty NodeID.
var ErrEmptyNodeID = errors.New("store: ClaimJob requires non-empty nodeID")

// Store defines the interface for database operations.
// The sqlc-generated Queries type will implement the query methods.
type Store interface {
	Querier
	// ClaimJob atomically claims the next claimable job for a node.
	// Requires a non-empty nodeID; returns ErrEmptyNodeID if empty.
	ClaimJob(ctx context.Context, nodeID types.NodeID) (Job, error)
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
	return s.claimJobInternal(ctx, &nodeID)
}
