package handlers

import (
	"context"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// Run query methods

func (m *handlerStore) ListRunsWithMetadata(ctx context.Context, params store.ListRunsWithMetadataParams) ([]store.ListRunsWithMetadataRow, error) {
	return m.listRunsWithMetadata.record(params)
}

func (m *handlerStore) DeleteRun(ctx context.Context, id types.RunID) error {
	_, err := m.deleteRun.record(id.String())
	return err
}

func (m *handlerStore) CancelRun(ctx context.Context, runID types.RunID) error {
	_, err := m.cancelRun.record(runID.String())
	return err
}

func (m *handlerStore) RestartRun(ctx context.Context, arg store.RestartRunParams) (store.Run, error) {
	return m.restartRun.record(arg)
}

func (m *handlerStore) ListRunSBOMRowsByJobType(ctx context.Context, arg store.ListRunSBOMRowsByJobTypeParams) ([]store.ListRunSBOMRowsByJobTypeRow, error) {
	m.listRunSBOMRowsByJobType.called = true
	m.listRunSBOMRowsByJobType.params = arg
	m.listRunSBOMRowsByJobType.calls = append(m.listRunSBOMRowsByJobType.calls, arg)
	if m.listRunSBOMRowsByJobType.err != nil {
		return nil, m.listRunSBOMRowsByJobType.err
	}
	if m.sbomRowsByJobType != nil {
		return m.sbomRowsByJobType[arg.JobType], nil
	}
	return m.listRunSBOMRowsByJobType.val, nil
}

// Ingest methods

func (m *handlerStore) CreateDiff(ctx context.Context, params store.CreateDiffParams) (store.Diff, error) {
	return m.createDiff.record(params)
}
