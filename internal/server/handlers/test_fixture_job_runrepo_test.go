package handlers

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// Run mutation and action methods

func (m *jobStore) UpdateRunError(ctx context.Context, params store.UpdateRunErrorParams) error {
	_, err := m.updateRunRepoError.record(params)
	return err
}

func (m *jobStore) UpdateRunBaseRef(ctx context.Context, params store.UpdateRunBaseRefParams) error {
	_, err := m.updateRunRepoBaseRef.record(params)
	return err
}

func (m *jobStore) IncrementRunAttempt(ctx context.Context, arg types.RunID) error {
	_, err := m.incrementRunRepoAttempt.record(arg)
	return err
}

func (m *jobStore) ListRunsByWave(ctx context.Context, waveID types.WaveID) ([]store.Run, error) {
	return m.listRunReposByRun.record(waveID.String())
}

func (m *jobStore) ListQueuedRunsByWave(ctx context.Context, waveID types.WaveID) ([]store.Run, error) {
	return m.listQueuedRunReposByRun.record(waveID.String())
}

func (m *jobStore) ListRunsWithURLByWave(ctx context.Context, waveID types.WaveID) ([]store.ListRunsWithURLByWaveRow, error) {
	return m.listRunReposWithURLByRun.record(waveID.String())
}

func (m *jobStore) CountRunsByWaveStatus(ctx context.Context, waveID types.WaveID) ([]store.CountRunsByWaveStatusRow, error) {
	return m.countRunReposByStatus.ret()
}

func (m *jobStore) CancelActiveJobsByRunAttempt(ctx context.Context, params store.CancelActiveJobsByRunAttemptParams) (int64, error) {
	return m.cancelActiveJobsByRunRepoAttempt.record(params)
}

func (m *jobStore) GetLatestRunByMigAndRepoStatus(ctx context.Context, arg store.GetLatestRunByMigAndRepoStatusParams) (store.GetLatestRunByMigAndRepoStatusRow, error) {
	return m.getLatestRunRepoByMigAndRepoStatus.record(arg)
}

func (m *jobStore) CreateRunAction(ctx context.Context, params store.CreateRunActionParams) (store.RunAction, error) {
	m.createRunAction.called = true
	m.createRunAction.params = params
	if m.createRunAction.err != nil {
		return store.RunAction{}, m.createRunAction.err
	}
	result := m.createRunAction.val
	if result.ID.IsZero() {
		result.ID = params.ID
	}
	if result.RunID.IsZero() {
		result.RunID = params.RunID
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

func (m *jobStore) GetRunAction(ctx context.Context, id types.JobID) (store.RunAction, error) {
	m.getRunAction.called = true
	m.getRunAction.params = id
	if m.getRunAction.err != nil {
		return store.RunAction{}, m.getRunAction.err
	}
	if m.getRunAction.val.ID.IsZero() {
		return store.RunAction{}, pgx.ErrNoRows
	}
	return m.getRunAction.val, nil
}

func (m *jobStore) GetRunActionByKey(ctx context.Context, arg store.GetRunActionByKeyParams) (store.RunAction, error) {
	m.getRunActionByKey.called = true
	m.getRunActionByKey.params = arg
	if m.getRunActionByKey.err != nil {
		return store.RunAction{}, m.getRunActionByKey.err
	}
	if m.getRunActionByKey.val.ID.IsZero() {
		return store.RunAction{}, pgx.ErrNoRows
	}
	return m.getRunActionByKey.val, nil
}

func (m *jobStore) UpdateRunActionCompletion(ctx context.Context, params store.UpdateRunActionCompletionParams) error {
	_, err := m.updateRunActionCompletion.record(params)
	return err
}

func (m *jobStore) ListRunActionsByRunAttempt(ctx context.Context, arg store.ListRunActionsByRunAttemptParams) ([]store.RunAction, error) {
	return m.listRunActionsByRunRepoAttempt.record(arg)
}

func (m *jobStore) GetNodeAction(ctx context.Context, id types.JobID) (store.NodeAction, error) {
	return m.getNodeAction.record(id)
}

func (m *jobStore) UpdateNodeActionCompletion(ctx context.Context, params store.UpdateNodeActionCompletionParams) error {
	_, err := m.updateNodeActionCompletion.record(params)
	return err
}
