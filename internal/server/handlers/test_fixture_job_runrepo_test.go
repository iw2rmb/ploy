package handlers

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// Run repo methods

func (m *jobStore) GetRunRepo(ctx context.Context, arg store.GetRunRepoParams) (store.RunRepo, error) {
	m.getRunRepoCalled = true
	m.getRunRepoParam = arg
	if m.getRunRepoErr != nil {
		return store.RunRepo{}, m.getRunRepoErr
	}
	if len(m.getRunRepoResults) > 0 {
		idx := m.getRunRepoCalls
		if idx >= len(m.getRunRepoResults) {
			idx = len(m.getRunRepoResults) - 1
		}
		m.getRunRepoCalls++
		return m.getRunRepoResults[idx], nil
	}
	return m.getRunRepoResult, nil
}

func (m *jobStore) UpdateRunRepoStatus(ctx context.Context, params store.UpdateRunRepoStatusParams) error {
	_, err := m.updateRunRepoStatus.record(params)
	return err
}

func (m *jobStore) UpdateRunRepoError(ctx context.Context, params store.UpdateRunRepoErrorParams) error {
	_, err := m.updateRunRepoError.record(params)
	return err
}

func (m *jobStore) UpdateRunRepoRefs(ctx context.Context, params store.UpdateRunRepoRefsParams) error {
	_, err := m.updateRunRepoRefs.record(params)
	return err
}

func (m *jobStore) IncrementRunRepoAttempt(ctx context.Context, arg store.IncrementRunRepoAttemptParams) error {
	_, err := m.incrementRunRepoAttempt.record(arg)
	return err
}

func (m *jobStore) CreateRunRepo(ctx context.Context, params store.CreateRunRepoParams) (store.RunRepo, error) {
	m.createRunRepoCalled = true
	m.createRunRepoParams = params
	result := m.createRunRepoResult
	if result.MigID.IsZero() {
		result.MigID = params.MigID
	}
	if result.RunID.IsZero() {
		result.RunID = params.RunID
	}
	if result.RepoID.IsZero() {
		result.RepoID = params.RepoID
	}
	if result.RepoBaseRef == "" {
		result.RepoBaseRef = params.RepoBaseRef
	}
	if result.RepoTargetRef == "" {
		result.RepoTargetRef = params.RepoTargetRef
	}
	if result.Status == "" {
		result.Status = types.RunRepoStatusQueued
	}
	if result.Attempt == 0 {
		result.Attempt = 1
	}
	return result, m.createRunRepoErr
}

func (m *jobStore) ListRunReposByRun(ctx context.Context, runID types.RunID) ([]store.RunRepo, error) {
	return m.listRunReposByRun.record(runID.String())
}

func (m *jobStore) ListQueuedRunReposByRun(ctx context.Context, runID types.RunID) ([]store.RunRepo, error) {
	return m.listQueuedRunReposByRun.record(runID.String())
}

func (m *jobStore) ListRunReposWithURLByRun(ctx context.Context, runID types.RunID) ([]store.ListRunReposWithURLByRunRow, error) {
	return m.listRunReposWithURLByRun.record(runID.String())
}

func (m *jobStore) CountRunReposByStatus(ctx context.Context, runID types.RunID) ([]store.CountRunReposByStatusRow, error) {
	return m.countRunReposByStatus.ret()
}

func (m *jobStore) CancelActiveJobsByRunRepoAttempt(ctx context.Context, params store.CancelActiveJobsByRunRepoAttemptParams) (int64, error) {
	return m.cancelActiveJobsByRunRepoAttempt.record(params)
}

func (m *jobStore) GetLatestRunRepoByMigAndRepoStatus(ctx context.Context, arg store.GetLatestRunRepoByMigAndRepoStatusParams) (store.GetLatestRunRepoByMigAndRepoStatusRow, error) {
	return m.getLatestRunRepoByMigAndRepoStatus.record(arg)
}

func (m *jobStore) CreateRunRepoAction(ctx context.Context, params store.CreateRunRepoActionParams) (store.RunRepoAction, error) {
	m.createRunRepoAction.called = true
	m.createRunRepoAction.params = params
	if m.createRunRepoAction.err != nil {
		return store.RunRepoAction{}, m.createRunRepoAction.err
	}
	result := m.createRunRepoAction.val
	if result.ID.IsZero() {
		result.ID = params.ID
	}
	if result.RunID.IsZero() {
		result.RunID = params.RunID
	}
	if result.RepoID.IsZero() {
		result.RepoID = params.RepoID
	}
	if result.Attempt == 0 {
		result.Attempt = params.Attempt
	}
	if result.ActionType == "" {
		result.ActionType = params.ActionType
	}
	if result.Status == "" {
		result.Status = params.Status
	}
	if len(result.Meta) == 0 {
		result.Meta = params.Meta
	}
	return result, nil
}

func (m *jobStore) GetRunRepoAction(ctx context.Context, id types.JobID) (store.RunRepoAction, error) {
	m.getRunRepoAction.called = true
	m.getRunRepoAction.params = id
	if m.getRunRepoAction.err != nil {
		return store.RunRepoAction{}, m.getRunRepoAction.err
	}
	if m.getRunRepoAction.val.ID.IsZero() {
		return store.RunRepoAction{}, pgx.ErrNoRows
	}
	return m.getRunRepoAction.val, nil
}

func (m *jobStore) GetRunRepoActionByKey(ctx context.Context, arg store.GetRunRepoActionByKeyParams) (store.RunRepoAction, error) {
	m.getRunRepoActionByKey.called = true
	m.getRunRepoActionByKey.params = arg
	if m.getRunRepoActionByKey.err != nil {
		return store.RunRepoAction{}, m.getRunRepoActionByKey.err
	}
	if m.getRunRepoActionByKey.val.ID.IsZero() {
		return store.RunRepoAction{}, pgx.ErrNoRows
	}
	return m.getRunRepoActionByKey.val, nil
}

func (m *jobStore) UpdateRunRepoActionCompletion(ctx context.Context, params store.UpdateRunRepoActionCompletionParams) error {
	_, err := m.updateRunRepoActionCompletion.record(params)
	return err
}

func (m *jobStore) ListRunRepoActionsByRunRepoAttempt(ctx context.Context, arg store.ListRunRepoActionsByRunRepoAttemptParams) ([]store.RunRepoAction, error) {
	return m.listRunRepoActionsByRunRepoAttempt.record(arg)
}
