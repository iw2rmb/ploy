package handlers

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// Stale recovery methods

func (m *jobStore) ListStaleRunningJobs(ctx context.Context, lastHeartbeat pgtype.Timestamptz) ([]store.ListStaleRunningJobsRow, error) {
	return m.listStaleRunningJobs.record(lastHeartbeat)
}

func (m *jobStore) CountStaleNodesWithRunningJobs(ctx context.Context, lastHeartbeat pgtype.Timestamptz) (int64, error) {
	return m.countStaleNodesWithRunningJobs.ret()
}

// Node methods (for claim)

func (m *jobStore) GetNode(ctx context.Context, id types.NodeID) (store.Node, error) {
	return m.getNode.record(id.String())
}

func (m *jobStore) UpdateNodeHeartbeat(ctx context.Context, params store.UpdateNodeHeartbeatParams) error {
	_, err := m.updateNodeHeartbeat.record(params)
	return err
}

// Mig repo methods (for claim spec merge)

func (m *jobStore) GetMigRepo(ctx context.Context, id types.MigRepoID) (store.MigRepo, error) {
	return m.getMigRepo.ret()
}

func (m *jobStore) ListMigReposByMig(ctx context.Context, migID types.MigID) ([]store.MigRepo, error) {
	return m.listMigReposByMigResult, nil
}

// Event methods

func (m *jobStore) CreateEvent(ctx context.Context, params store.CreateEventParams) (store.Event, error) {
	return m.createEvent.record(params)
}

// Ingest methods

func (m *jobStore) CreateLog(ctx context.Context, params store.CreateLogParams) (store.Log, error) {
	return m.createLog.ret()
}

func (m *jobStore) ListLogsByRun(ctx context.Context, runID types.RunID) ([]store.Log, error) {
	return m.listLogsByRun.record(runID.String())
}

func (m *jobStore) ListLogsByRunAndJob(ctx context.Context, arg store.ListLogsByRunAndJobParams) ([]store.Log, error) {
	return m.listLogsByRunAndJob.record(arg)
}

// Spec/Mig/Run creation methods (for migs_ticket flow)

func (m *jobStore) CreateSpec(ctx context.Context, params store.CreateSpecParams) (store.Spec, error) {
	m.createSpecCalled = true
	m.createSpecParams = params
	result := store.Spec{ID: params.ID, Spec: params.Spec, CreatedBy: params.CreatedBy}
	return result, m.createSpecErr
}

func (m *jobStore) CreateMig(ctx context.Context, params store.CreateMigParams) (store.Mig, error) {
	m.createMigCalled = true
	m.createMigParams = params
	return store.Mig{ID: params.ID, Name: params.Name, SpecID: params.SpecID, CreatedBy: params.CreatedBy}, nil
}

func (m *jobStore) CreateMigRepo(ctx context.Context, params store.CreateMigRepoParams) (store.MigRepo, error) {
	m.createMigRepoCalled = true
	return store.MigRepo{ID: params.ID, MigID: params.MigID, RepoID: types.NewRepoID(), BaseRef: params.BaseRef, TargetRef: params.TargetRef}, nil
}

func (m *jobStore) GetRepo(ctx context.Context, id types.RepoID) (store.Repo, error) {
	if !id.IsZero() {
		return store.Repo{ID: id, Url: "https://github.com/user/repo.git"}, nil
	}
	return store.Repo{}, pgx.ErrNoRows
}

func (m *jobStore) CreateRun(ctx context.Context, params store.CreateRunParams) (store.Run, error) {
	m.createRunCalled = true
	m.createRunParams = params
	result := m.createRunResult
	if result.ID.IsZero() {
		result.ID = params.ID
	}
	if result.MigID.IsZero() {
		result.MigID = params.MigID
	}
	if result.SpecID.IsZero() {
		result.SpecID = params.SpecID
	}
	result.CreatedBy = params.CreatedBy
	return result, nil
}

func (m *jobStore) ListRuns(ctx context.Context, params store.ListRunsParams) ([]store.Run, error) {
	if int(params.Offset) >= len(m.listRunsResult) {
		return []store.Run{}, nil
	}
	end := int(params.Offset) + int(params.Limit)
	if end > len(m.listRunsResult) {
		end = len(m.listRunsResult)
	}
	return m.listRunsResult[params.Offset:end], nil
}
