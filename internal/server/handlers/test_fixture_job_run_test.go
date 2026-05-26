package handlers

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// Artifact/Diff methods

func (m *jobStore) CreateDiff(ctx context.Context, params store.CreateDiffParams) (store.Diff, error) {
	return m.createDiff.record(params)
}

func (m *jobStore) DeleteDiff(ctx context.Context, id pgtype.UUID) error {
	_, err := m.deleteDiff.record(id)
	return err
}

func (m *jobStore) GetLatestDiffByJob(ctx context.Context, jobID *types.JobID) (store.Diff, error) {
	if m.getLatestDiffByJobError != nil {
		return store.Diff{}, m.getLatestDiffByJobError
	}
	if jobID != nil && len(m.getLatestDiffByJobByID) > 0 {
		if diff, ok := m.getLatestDiffByJobByID[*jobID]; ok {
			m.getLatestDiffByJob.called = true
			m.getLatestDiffByJob.params = jobID
			return diff, nil
		}
	}
	return m.getLatestDiffByJob.record(jobID)
}

func (m *jobStore) CreateArtifactBundle(ctx context.Context, params store.CreateArtifactBundleParams) (store.ArtifactBundle, error) {
	return m.createArtifactBundle.ret()
}

func (m *jobStore) ListArtifactBundlesByRunAndJob(ctx context.Context, arg store.ListArtifactBundlesByRunAndJobParams) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesByRunAndJob.record(arg)
}

// Run query methods

func (m *jobStore) GetRun(ctx context.Context, id types.RunID) (store.Run, error) {
	return m.getRun.record(id.String())
}

func (m *jobStore) GetSpec(ctx context.Context, id types.SpecID) (store.Spec, error) {
	return m.getSpec.record(id.String())
}

func (m *jobStore) GetSpecBundle(ctx context.Context, id string) (store.SpecBundle, error) {
	bundle, err := m.getSpecBundle.record(id)
	if err != nil {
		return store.SpecBundle{}, err
	}
	if bundle.ID == "" && bundle.ObjectKey == nil {
		return store.SpecBundle{}, errors.New("mock GetSpecBundle not configured")
	}
	return bundle, nil
}

func (m *jobStore) GetRunTiming(ctx context.Context, id types.RunID) (store.RunsTiming, error) {
	return m.getRunTiming.record(id.String())
}

func (m *jobStore) ListRunsTimings(ctx context.Context, arg store.ListRunsTimingsParams) ([]store.RunsTiming, error) {
	return m.listRunsTimings.ret()
}

func (m *jobStore) AckRunStart(ctx context.Context, id string) error {
	return m.ackRunStart.err
}

func (m *jobStore) UpdateRunCompletion(ctx context.Context, params store.UpdateRunCompletionParams) error {
	return m.updateRunCompletion.err
}

func (m *jobStore) UpdateRunStatus(ctx context.Context, params store.UpdateRunStatusParams) error {
	_, err := m.updateRunStatus.record(params)
	return err
}

func (m *jobStore) CancelRunV1(ctx context.Context, runID types.RunID) error {
	_, err := m.cancelRunV1.record(runID.String())
	return err
}

func (m *jobStore) UpdateRunResume(ctx context.Context, id types.RunID) error {
	return m.updateRunResume.err
}
