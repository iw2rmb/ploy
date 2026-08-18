package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// Stale recovery methods

func (m *handlerStore) ListStaleRunningJobs(ctx context.Context, lastHeartbeat pgtype.Timestamptz) ([]store.ListStaleRunningJobsRow, error) {
	return m.listStaleRunningJobs.record(lastHeartbeat)
}

func (m *handlerStore) CountStaleNodesWithRunningJobs(ctx context.Context, lastHeartbeat pgtype.Timestamptz) (int64, error) {
	return m.countStaleNodesWithRunningJobs.ret()
}

// Node methods (for claim)

func (m *handlerStore) ListLogsByRun(ctx context.Context, runID types.RunID) ([]store.Log, error) {
	return m.listLogsByRun.record(runID.String())
}

func (m *handlerStore) ListLogsByRunAndJob(ctx context.Context, arg store.ListLogsByRunAndJobParams) ([]store.Log, error) {
	return m.listLogsByRunAndJob.record(arg)
}

// Spec/Mig/Run creation methods (for migs_ticket flow)
