package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func (m *mockStore) CreateDiff(ctx context.Context, params store.CreateDiffParams) (store.Diff, error) {
	m.createDiffCalled = true
	m.createDiffParams = params
	return m.createDiffResult, m.createDiffErr
}

func (m *mockStore) CreateArtifactBundle(ctx context.Context, params store.CreateArtifactBundleParams) (store.ArtifactBundle, error) {
	m.createArtifactBundleCalled = true
	m.createArtifactBundleParams = params
	return m.createArtifactBundleResult, m.createArtifactBundleErr
}

func (m *mockStore) ListArtifactBundlesByCID(ctx context.Context, cid *string) ([]store.ArtifactBundle, error) {
	m.listArtifactBundlesByCIDCalled = true
	m.listArtifactBundlesByCIDParams = cid
	return m.listArtifactBundlesByCIDResult, m.listArtifactBundlesByCIDErr
}

func (m *mockStore) ListArtifactBundlesMetaByCID(ctx context.Context, cid *string) ([]store.ArtifactBundle, error) {
	m.listArtifactBundlesMetaByCIDCalled = true
	m.listArtifactBundlesMetaByCIDParams = cid
	return m.listArtifactBundlesMetaByCIDResult, m.listArtifactBundlesMetaByCIDErr
}

func (m *mockStore) ListArtifactBundlesByRun(ctx context.Context, runID types.RunID) ([]store.ArtifactBundle, error) {
	m.listArtifactBundlesByRunCalled = true
	m.listArtifactBundlesByRunParam = runID.String()
	return m.listArtifactBundlesByRunResult, m.listArtifactBundlesByRunErr
}

func (m *mockStore) ListArtifactBundlesMetaByRun(ctx context.Context, runID types.RunID) ([]store.ArtifactBundle, error) {
	m.listArtifactBundlesMetaByRunCalled = true
	m.listArtifactBundlesMetaByRunParam = runID.String()
	return m.listArtifactBundlesMetaByRunResult, m.listArtifactBundlesMetaByRunErr
}

func (m *mockStore) GetArtifactBundle(ctx context.Context, id pgtype.UUID) (store.ArtifactBundle, error) {
	m.getArtifactBundleCalled = true
	m.getArtifactBundleParams = id
	return m.getArtifactBundleResult, m.getArtifactBundleErr
}

func (m *mockStore) ListArtifactBundlesByRunAndJob(ctx context.Context, arg store.ListArtifactBundlesByRunAndJobParams) ([]store.ArtifactBundle, error) {
	m.listArtifactBundlesByRunAndJobCalled = true
	m.listArtifactBundlesByRunAndJobParams = arg
	return m.listArtifactBundlesByRunAndJobResult, m.listArtifactBundlesByRunAndJobErr
}

func (m *mockStore) ListArtifactBundlesMetaByRunAndJob(ctx context.Context, arg store.ListArtifactBundlesMetaByRunAndJobParams) ([]store.ArtifactBundle, error) {
	m.listArtifactBundlesMetaByRunAndJobCalled = true
	m.listArtifactBundlesMetaByRunAndJobParams = arg
	return m.listArtifactBundlesMetaByRunAndJobResult, m.listArtifactBundlesMetaByRunAndJobErr
}

func (m *mockStore) ListSBOMRowsByJob(ctx context.Context, jobID types.JobID) ([]store.Sbom, error) {
	m.listSBOMRowsByJobCalled = true
	m.listSBOMRowsByJobParam = jobID
	return m.listSBOMRowsByJobResult, m.listSBOMRowsByJobErr
}

func (m *mockStore) HasSBOMEvidenceForStack(ctx context.Context, arg store.HasSBOMEvidenceForStackParams) (bool, error) {
	m.hasSBOMEvidenceForStackCalled = true
	m.hasSBOMEvidenceForStackParams = arg
	return m.hasSBOMEvidenceForStackResult, m.hasSBOMEvidenceForStackErr
}

func (m *mockStore) ListSBOMCompatRows(ctx context.Context, arg store.ListSBOMCompatRowsParams) ([]store.ListSBOMCompatRowsRow, error) {
	m.listSBOMCompatRowsCalled = true
	m.listSBOMCompatRowsParams = arg
	return m.listSBOMCompatRowsResult, m.listSBOMCompatRowsErr
}

func (m *mockStore) ListDiffsByRunRepo(ctx context.Context, arg store.ListDiffsByRunRepoParams) ([]store.Diff, error) {
	m.listDiffsByRunRepoCalled = true
	m.listDiffsByRunRepoParams = arg
	return m.listDiffsByRunRepoResult, m.listDiffsByRunRepoErr
}

// ListDiffsMetaByRunRepo implements the v1 repo-scoped diffs metadata query.
func (m *mockStore) ListDiffsMetaByRunRepo(ctx context.Context, arg store.ListDiffsMetaByRunRepoParams) ([]store.Diff, error) {
	m.listDiffsMetaByRunRepoCalled = true
	m.listDiffsMetaByRunRepoParams = arg
	return m.listDiffsMetaByRunRepoResult, m.listDiffsMetaByRunRepoErr
}

func (m *mockStore) GetDiff(ctx context.Context, id pgtype.UUID) (store.Diff, error) {
	m.getDiffCalled = true
	m.getDiffParam = id
	return m.getDiffResult, m.getDiffErr
}

func (m *mockStore) CreateLog(ctx context.Context, params store.CreateLogParams) (store.Log, error) {
	m.createLogCalled = true
	m.createLogParams = params
	return m.createLogResult, m.createLogErr
}

// API Token methods
