package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// artifactStore is a focused mock for artifact download, repo artifacts, diffs, and SBOM compat handler tests.
type artifactStore struct {
	store.Store

	// Artifact bundle queries
	listArtifactBundlesByCID       mockResult[[]store.ArtifactBundle]
	listArtifactBundlesByRun       mockResult[[]store.ArtifactBundle]
	getArtifactBundle              mockResult[store.ArtifactBundle]
	listArtifactBundlesByRunAndJob mockResult[[]store.ArtifactBundle]

	// Diff queries
	listDiffsByRunRepo mockCall[store.ListDiffsByRunRepoParams, []store.Diff]
	getDiff            mockCall[pgtype.UUID, store.Diff]

	// Job lookup (for repo-scoped artifact/diff filtering)
	getJobCalled bool
	getJobParams string
	getJobResult store.Job
	getJobErr    error

	// RunRepo lookup (for repo-scoped queries)
	getRunRepoResult         store.RunRepo
	getRunRepoErr            error
	listJobsByRunRepoAttempt mockCall[store.ListJobsByRunRepoAttemptParams, []store.Job]

	// SBOM compat
	hasSBOMEvidenceForStack mockResult[bool]
	listSBOMCompatRows      mockCall[store.ListSBOMCompatRowsParams, []store.ListSBOMCompatRowsRow]
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

func (m *artifactStore) ListDiffsByRunRepo(ctx context.Context, arg store.ListDiffsByRunRepoParams) ([]store.Diff, error) {
	return m.listDiffsByRunRepo.record(arg)
}

func (m *artifactStore) GetDiff(ctx context.Context, id pgtype.UUID) (store.Diff, error) {
	return m.getDiff.record(id)
}

func (m *artifactStore) GetJob(ctx context.Context, id types.JobID) (store.Job, error) {
	m.getJobCalled = true
	m.getJobParams = id.String()
	return m.getJobResult, m.getJobErr
}

func (m *artifactStore) GetRunRepo(ctx context.Context, arg store.GetRunRepoParams) (store.RunRepo, error) {
	if m.getRunRepoErr != nil {
		return store.RunRepo{}, m.getRunRepoErr
	}
	return m.getRunRepoResult, nil
}

func (m *artifactStore) ListJobsByRunRepoAttempt(ctx context.Context, arg store.ListJobsByRunRepoAttemptParams) ([]store.Job, error) {
	return m.listJobsByRunRepoAttempt.record(arg)
}

func (m *artifactStore) HasSBOMEvidenceForStack(ctx context.Context, arg store.HasSBOMEvidenceForStackParams) (bool, error) {
	return m.hasSBOMEvidenceForStack.ret()
}

func (m *artifactStore) ListSBOMCompatRows(ctx context.Context, arg store.ListSBOMCompatRowsParams) ([]store.ListSBOMCompatRowsRow, error) {
	return m.listSBOMCompatRows.record(arg)
}
