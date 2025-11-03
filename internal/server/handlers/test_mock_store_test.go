package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// mockStore is a minimal Store implementation for testing handlers.
type mockStore struct {
	store.Store
	updateCertMetadataCalled bool
	updateCertMetadataParams store.UpdateNodeCertMetadataParams
	updateCertMetadataErr    error

	createRepoCalled bool
	createRepoParams store.CreateRepoParams
	createRepoResult store.Repo
	createRepoErr    error

	listReposCalled bool
	listReposResult []store.Repo
	listReposErr    error

	createModCalled bool
	createModParams store.CreateModParams
	createModResult store.Mod
	createModErr    error

	listModsCalled bool
	listModsResult []store.Mod
	listModsErr    error

	listModsByRepoCalled bool
	listModsByRepoParams pgtype.UUID
	listModsByRepoResult []store.Mod
	listModsByRepoErr    error

	createRunCalled bool
	createRunParams store.CreateRunParams
	createRunResult store.Run
	createRunErr    error

	getRunCalled bool
	getRunParams pgtype.UUID
	getRunResult store.Run
	getRunErr    error

	getRunTimingCalled bool
	getRunTimingParams pgtype.UUID
	getRunTimingResult store.RunsTiming
	getRunTimingErr    error

	listRunsTimingsCalled bool
	listRunsTimingsParams store.ListRunsTimingsParams
	listRunsTimingsResult []store.RunsTiming
	listRunsTimingsErr    error

	deleteRunCalled bool
	deleteRunParams pgtype.UUID
	deleteRunErr    error

	claimRunCalled bool
	claimRunParams pgtype.UUID
	claimRunResult store.Run
	claimRunErr    error

	getNodeCalled bool
	getNodeParams pgtype.UUID
	getNodeResult store.Node
	getNodeErr    error

	updateNodeHeartbeatCalled bool
	updateNodeHeartbeatParams store.UpdateNodeHeartbeatParams
	updateNodeHeartbeatErr    error

	createEventCalled bool
	createEventParams store.CreateEventParams
	createEventResult store.Event
	createEventErr    error

	getStageCalled bool
	getStageParams pgtype.UUID
	getStageResult store.Stage
	getStageErr    error

	createDiffCalled bool
	createDiffParams store.CreateDiffParams
	createDiffResult store.Diff
	createDiffErr    error

	createArtifactBundleCalled bool
	createArtifactBundleParams store.CreateArtifactBundleParams
	createArtifactBundleResult store.ArtifactBundle
	createArtifactBundleErr    error

	// AckRunStart tracking
	ackRunStartCalled bool
	ackRunStartParam  pgtype.UUID
	ackRunStartErr    error

	// UpdateRunCompletion tracking
	updateRunCompletionCalled bool
	updateRunCompletionParams store.UpdateRunCompletionParams
	updateRunCompletionErr    error
}

func (m *mockStore) UpdateNodeCertMetadata(ctx context.Context, params store.UpdateNodeCertMetadataParams) error {
	m.updateCertMetadataCalled = true
	m.updateCertMetadataParams = params
	return m.updateCertMetadataErr
}

func (m *mockStore) CreateRepo(ctx context.Context, params store.CreateRepoParams) (store.Repo, error) {
	m.createRepoCalled = true
	m.createRepoParams = params
	return m.createRepoResult, m.createRepoErr
}

func (m *mockStore) ListRepos(ctx context.Context) ([]store.Repo, error) {
	m.listReposCalled = true
	return m.listReposResult, m.listReposErr
}

func (m *mockStore) CreateMod(ctx context.Context, params store.CreateModParams) (store.Mod, error) {
	m.createModCalled = true
	m.createModParams = params
	return m.createModResult, m.createModErr
}

func (m *mockStore) ListMods(ctx context.Context) ([]store.Mod, error) {
	m.listModsCalled = true
	return m.listModsResult, m.listModsErr
}

func (m *mockStore) ListModsByRepo(ctx context.Context, repoID pgtype.UUID) ([]store.Mod, error) {
	m.listModsByRepoCalled = true
	m.listModsByRepoParams = repoID
	return m.listModsByRepoResult, m.listModsByRepoErr
}

func (m *mockStore) CreateRun(ctx context.Context, params store.CreateRunParams) (store.Run, error) {
	m.createRunCalled = true
	m.createRunParams = params
	return m.createRunResult, m.createRunErr
}

func (m *mockStore) GetRun(ctx context.Context, id pgtype.UUID) (store.Run, error) {
	m.getRunCalled = true
	m.getRunParams = id
	return m.getRunResult, m.getRunErr
}

func (m *mockStore) GetRunTiming(ctx context.Context, id pgtype.UUID) (store.RunsTiming, error) {
	m.getRunTimingCalled = true
	m.getRunTimingParams = id
	return m.getRunTimingResult, m.getRunTimingErr
}

func (m *mockStore) ListRunsTimings(ctx context.Context, arg store.ListRunsTimingsParams) ([]store.RunsTiming, error) {
	m.listRunsTimingsCalled = true
	m.listRunsTimingsParams = arg
	return m.listRunsTimingsResult, m.listRunsTimingsErr
}

func (m *mockStore) DeleteRun(ctx context.Context, id pgtype.UUID) error {
	m.deleteRunCalled = true
	m.deleteRunParams = id
	return m.deleteRunErr
}

func (m *mockStore) ClaimRun(ctx context.Context, nodeID pgtype.UUID) (store.Run, error) {
	m.claimRunCalled = true
	m.claimRunParams = nodeID
	return m.claimRunResult, m.claimRunErr
}

func (m *mockStore) GetNode(ctx context.Context, id pgtype.UUID) (store.Node, error) {
	m.getNodeCalled = true
	m.getNodeParams = id
	return m.getNodeResult, m.getNodeErr
}

func (m *mockStore) UpdateNodeHeartbeat(ctx context.Context, params store.UpdateNodeHeartbeatParams) error {
	m.updateNodeHeartbeatCalled = true
	m.updateNodeHeartbeatParams = params
	return m.updateNodeHeartbeatErr
}

func (m *mockStore) CreateEvent(ctx context.Context, params store.CreateEventParams) (store.Event, error) {
	m.createEventCalled = true
	m.createEventParams = params
	return m.createEventResult, m.createEventErr
}

func (m *mockStore) GetStage(ctx context.Context, id pgtype.UUID) (store.Stage, error) {
	m.getStageCalled = true
	m.getStageParams = id
	return m.getStageResult, m.getStageErr
}

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

func (m *mockStore) AckRunStart(ctx context.Context, id pgtype.UUID) error {
	m.ackRunStartCalled = true
	m.ackRunStartParam = id
	return m.ackRunStartErr
}

func (m *mockStore) UpdateRunCompletion(ctx context.Context, params store.UpdateRunCompletionParams) error {
	m.updateRunCompletionCalled = true
	m.updateRunCompletionParams = params
	return m.updateRunCompletionErr
}
