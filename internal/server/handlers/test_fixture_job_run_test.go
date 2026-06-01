package handlers

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// Run mutation and action methods

func (m *jobStore) UpdateRunError(ctx context.Context, params store.UpdateRunErrorParams) error {
	_, err := m.updateRunError.record(params)
	return err
}

func (m *jobStore) UpdateRunBaseRef(ctx context.Context, params store.UpdateRunBaseRefParams) error {
	_, err := m.updateRunBaseRef.record(params)
	return err
}

func (m *jobStore) IncrementRunAttempt(ctx context.Context, arg types.RunID) error {
	_, err := m.incrementRunAttempt.record(arg)
	return err
}

func (m *jobStore) ListRunsByWave(ctx context.Context, waveID types.WaveID) ([]store.Run, error) {
	return m.listRunsByWave.record(waveID.String())
}

func (m *jobStore) ListQueuedRunsByWave(ctx context.Context, waveID types.WaveID) ([]store.Run, error) {
	return m.listQueuedRunsByWave.record(waveID.String())
}

func (m *jobStore) ListRunsWithURLByWave(ctx context.Context, waveID types.WaveID) ([]store.ListRunsWithURLByWaveRow, error) {
	return m.listRunsWithURLByWave.record(waveID.String())
}

func (m *jobStore) CountRunsByWaveStatus(ctx context.Context, waveID types.WaveID) ([]store.CountRunsByWaveStatusRow, error) {
	return m.countRunsByStatus.ret()
}

func (m *jobStore) CancelActiveJobsByRunAttempt(ctx context.Context, params store.CancelActiveJobsByRunAttemptParams) (int64, error) {
	return m.cancelActiveJobsByRunAttempt.record(params)
}

func (m *jobStore) GetLatestRunByMigAndRepoStatus(ctx context.Context, arg store.GetLatestRunByMigAndRepoStatusParams) (store.GetLatestRunByMigAndRepoStatusRow, error) {
	return m.getLatestRunByMigAndRepoStatus.record(arg)
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
	return m.listRunActionsByRunAttempt.record(arg)
}

func (m *jobStore) GetRun(ctx context.Context, id types.RunID) (store.Run, error) {
	return m.getRun.record(id.String())
}

func (m *jobStore) GetSpec(ctx context.Context, id types.SpecID) (store.Spec, error) {
	return m.getSpec.record(id.String())
}

func (m *jobStore) GetWave(_ context.Context, id types.WaveID) (store.Wave, error) {
	return m.getWave.record(id.String())
}

func (m *jobStore) AckRunStart(ctx context.Context, id types.RunID) error {
	_, err := m.ackRunStart.ret()
	return err
}

func (m *jobStore) UpdateRunStatus(ctx context.Context, params store.UpdateRunStatusParams) error {
	_, err := m.updateRunStatus.record(params)
	return err
}

func (m *jobStore) UpdateRunCompletion(ctx context.Context, id types.RunID) error {
	_, err := m.updateRunCompletion.ret()
	return err
}

func (m *jobStore) UpdateRunResume(ctx context.Context, id types.RunID) error {
	_, err := m.updateRunResume.ret()
	return err
}

func (m *jobStore) UpdateWaveStatus(ctx context.Context, params store.UpdateWaveStatusParams) error {
	_, err := m.updateWaveStatus.record(params)
	return err
}

func (m *jobStore) ListArtifactBundlesByRunAndJob(ctx context.Context, arg store.ListArtifactBundlesByRunAndJobParams) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesByRunAndJob.record(arg)
}

func (m *jobStore) GetNodeAction(ctx context.Context, id types.JobID) (store.NodeAction, error) {
	return m.getNodeAction.record(id)
}

func (m *jobStore) UpdateNodeActionCompletion(ctx context.Context, params store.UpdateNodeActionCompletionParams) error {
	_, err := m.updateNodeActionCompletion.record(params)
	return err
}
