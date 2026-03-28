package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func (m *mockStore) CreateDiff(ctx context.Context, params store.CreateDiffParams) (store.Diff, error) {
	return m.createDiff.record(params)
}

func (m *mockStore) CreateArtifactBundle(ctx context.Context, params store.CreateArtifactBundleParams) (store.ArtifactBundle, error) {
	return m.createArtifactBundle.ret()
}

func (m *mockStore) ListArtifactBundlesByCID(ctx context.Context, cid *string) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesByCID.ret()
}

func (m *mockStore) ListArtifactBundlesMetaByCID(ctx context.Context, cid *string) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesMetaByCID.ret()
}

func (m *mockStore) ListArtifactBundlesByRun(ctx context.Context, runID types.RunID) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesByRun.ret()
}

func (m *mockStore) ListArtifactBundlesMetaByRun(ctx context.Context, runID types.RunID) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesMetaByRun.ret()
}

func (m *mockStore) GetArtifactBundle(ctx context.Context, id pgtype.UUID) (store.ArtifactBundle, error) {
	return m.getArtifactBundle.ret()
}

func (m *mockStore) ListArtifactBundlesByRunAndJob(ctx context.Context, arg store.ListArtifactBundlesByRunAndJobParams) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesByRunAndJob.ret()
}

func (m *mockStore) ListArtifactBundlesMetaByRunAndJob(ctx context.Context, arg store.ListArtifactBundlesMetaByRunAndJobParams) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesMetaByRunAndJob.ret()
}

func (m *mockStore) ListSBOMRowsByJob(ctx context.Context, jobID types.JobID) ([]store.Sbom, error) {
	return m.listSBOMRowsByJob.ret()
}

func (m *mockStore) HasSBOMEvidenceForStack(ctx context.Context, arg store.HasSBOMEvidenceForStackParams) (bool, error) {
	return m.hasSBOMEvidenceForStack.ret()
}

func (m *mockStore) ListSBOMCompatRows(ctx context.Context, arg store.ListSBOMCompatRowsParams) ([]store.ListSBOMCompatRowsRow, error) {
	return m.listSBOMCompatRows.record(arg)
}

func (m *mockStore) ListDiffsByRunRepo(ctx context.Context, arg store.ListDiffsByRunRepoParams) ([]store.Diff, error) {
	return m.listDiffsByRunRepo.ret()
}

func (m *mockStore) ListDiffsMetaByRunRepo(ctx context.Context, arg store.ListDiffsMetaByRunRepoParams) ([]store.Diff, error) {
	return m.listDiffsMetaByRunRepo.record(arg)
}

func (m *mockStore) GetDiff(ctx context.Context, id pgtype.UUID) (store.Diff, error) {
	return m.getDiff.record(id)
}

func (m *mockStore) CreateLog(ctx context.Context, params store.CreateLogParams) (store.Log, error) {
	return m.createLog.ret()
}
