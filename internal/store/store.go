// Package store provides PostgreSQL-backed data persistence using pgx and sqlc.
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store defines the interface for database operations.
// The sqlc-generated Queries type will implement the query methods.
type Store interface {
	Querier
	Close()
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
