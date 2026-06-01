package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// artifactStore is a focused mock for artifact download, repo artifacts, and diffs.
type artifactStore struct {
	store.Store

	// Artifact bundle queries
	listArtifactBundlesByCID       mockResult[[]store.ArtifactBundle]
	listArtifactBundlesByRun       mockResult[[]store.ArtifactBundle]
	getArtifactBundle              mockResult[store.ArtifactBundle]
	listArtifactBundlesByRunAndJob mockResult[[]store.ArtifactBundle]

	// Diff queries
	getLatestDiffByJob      mockCall[*types.JobID, store.Diff]
	getLatestDiffByJobByID  map[types.JobID]store.Diff
	getLatestDiffByJobError error

	// Job lookup (for run artifact/diff lookup)
	getJob     mockCall[types.JobID, store.Job]
	getJobByID map[types.JobID]store.Job

	// Run lookup (for run-scoped queries)
	getRun               mockCall[types.RunID, store.Run]
	listJobsByRunAttempt mockCall[store.ListJobsByRunAttemptParams, []store.Job]
}

func (m *artifactStore) ListArtifactBundlesByCID(ctx context.Context, cid *string) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesByCID.ret()
}

func (m *artifactStore) ListArtifactBundlesByRun(ctx context.Context, runID types.RunID) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesByRun.ret()
}

func (m *artifactStore) GetArtifactBundle(ctx context.Context, id pgtype.UUID) (store.ArtifactBundle, error) {
	return m.getArtifactBundle.ret()
}

func (m *artifactStore) ListArtifactBundlesByRunAndJob(ctx context.Context, arg store.ListArtifactBundlesByRunAndJobParams) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesByRunAndJob.ret()
}

func (m *artifactStore) GetLatestDiffByJob(ctx context.Context, jobID *types.JobID) (store.Diff, error) {
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

func (m *artifactStore) GetJob(ctx context.Context, id types.JobID) (store.Job, error) {
	if len(m.getJobByID) > 0 {
		if result, ok := m.getJobByID[id]; ok {
			m.getJob.called = true
			m.getJob.params = id
			return result, m.getJob.err
		}
	}
	return m.getJob.record(id)
}

func (m *artifactStore) GetRun(ctx context.Context, arg types.RunID) (store.Run, error) {
	return m.getRun.record(arg)
}

func (m *artifactStore) ListJobsByRunAttempt(ctx context.Context, arg store.ListJobsByRunAttemptParams) ([]store.Job, error) {
	return m.listJobsByRunAttempt.record(arg)
}
