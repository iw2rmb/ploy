package handlers

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// mockStore is a minimal Store implementation for testing handlers.
type mockStore struct {
	store.Store
	updateCertMetadataCalled bool
	updateCertMetadataParams store.UpdateNodeCertMetadataParams
	updateCertMetadataErr    error

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

	// UpdateRunStatus tracking
	updateRunStatusCalled bool
	updateRunStatusParams store.UpdateRunStatusParams
	updateRunStatusErr    error

	// Node drain/undrain tracking
	updateNodeDrainedCalled bool
	updateNodeDrainedParams store.UpdateNodeDrainedParams
	updateNodeDrainedErr    error

	// ListNodes tracking
	listNodesCalled bool
	listNodesResult []store.Node
	listNodesErr    error

	// ListArtifactBundlesByCID tracking
	listArtifactBundlesByCIDCalled bool
	listArtifactBundlesByCIDParams *string
	listArtifactBundlesByCIDResult []store.ArtifactBundle
	listArtifactBundlesByCIDErr    error

	// GetArtifactBundle tracking
	getArtifactBundleCalled bool
	getArtifactBundleParams pgtype.UUID
	getArtifactBundleResult store.ArtifactBundle
	getArtifactBundleErr    error

	// ListArtifactBundlesByRunAndStage tracking
	listArtifactBundlesByRunAndStageCalled bool
	listArtifactBundlesByRunAndStageParams store.ListArtifactBundlesByRunAndStageParams
	listArtifactBundlesByRunAndStageResult []store.ArtifactBundle
	listArtifactBundlesByRunAndStageErr    error

	// CreateStage tracking
	createStageCalled    bool
	createStageCallCount int
	createStageParams    []store.CreateStageParams
	createStageResult    store.Stage
	createStageErr       error

	// ListStagesByRun tracking
	listStagesByRunCalled bool
	listStagesByRunParam  pgtype.UUID
	listStagesByRunResult []store.Stage
	listStagesByRunErr    error

	// UpdateStageStatus tracking
	updateStageStatusCalled bool
	updateStageStatusParams store.UpdateStageStatusParams
	updateStageStatusCalls  []store.UpdateStageStatusParams
	updateStageStatusErr    error

	// ListDiffsByRun tracking
	listDiffsByRunCalled bool
	listDiffsByRunParam  pgtype.UUID
	listDiffsByRunResult []store.Diff
	listDiffsByRunErr    error

	// GetDiff tracking
	getDiffCalled bool
	getDiffParam  pgtype.UUID
	getDiffResult store.Diff
	getDiffErr    error

	// Buildgate job tracking
	createBGJobCalled bool
	createBGJobParam  []byte
	createBGJobResult store.BuildgateJob
	createBGJobErr    error

	getBGJobCalled bool
	getBGJobParam  pgtype.UUID
	getBGJobResult store.BuildgateJob
	getBGJobErr    error

	claimBGJobCalled bool
	claimBGJobParam  pgtype.UUID
	claimBGJobResult store.BuildgateJob
	claimBGJobErr    error

	ackBGStartCalled bool
	ackBGStartParam  pgtype.UUID
	ackBGStartErr    error

	updateBGCompleteCalled bool
	updateBGCompleteParams store.UpdateBuildGateJobCompletionParams
	updateBGCompleteErr    error

	// CreateLog tracking
	createLogCalled bool
	createLogParams store.CreateLogParams
	createLogResult store.Log
	createLogErr    error

	// API Token tracking
	insertAPITokenCalled bool
	insertAPITokenParams store.InsertAPITokenParams
	insertAPITokenErr    error

	listAPITokensCalled bool
	listAPITokensParams string // cluster_id
	listAPITokensResult []store.ListAPITokensRow
	listAPITokensErr    error

	revokeAPITokenCalled bool
	revokeAPITokenParam  string // token_id
	revokeAPITokenErr    error

	checkAPITokenRevokedCalled bool
	checkAPITokenRevokedParam  string
	checkAPITokenRevokedResult pgtype.Timestamptz
	checkAPITokenRevokedErr    error

	updateAPITokenLastUsedCalled bool
	updateAPITokenLastUsedParam  string
	updateAPITokenLastUsedErr    error

	// Bootstrap Token tracking
	insertBootstrapTokenCalled bool
	insertBootstrapTokenParams store.InsertBootstrapTokenParams
	insertBootstrapTokenErr    error

	getBootstrapTokenCalled bool
	getBootstrapTokenParam  string
	getBootstrapTokenResult store.GetBootstrapTokenRow
	getBootstrapTokenErr    error

	checkBootstrapTokenRevokedCalled bool
	checkBootstrapTokenRevokedParam  string
	checkBootstrapTokenRevokedResult pgtype.Timestamptz
	checkBootstrapTokenRevokedErr    error

	updateBootstrapTokenLastUsedCalled bool
	updateBootstrapTokenLastUsedParam  string
	updateBootstrapTokenLastUsedErr    error

	markBootstrapTokenUsedCalled bool
	markBootstrapTokenUsedParam  string
	markBootstrapTokenUsedErr    error
}

func (m *mockStore) UpdateNodeCertMetadata(ctx context.Context, params store.UpdateNodeCertMetadataParams) error {
	m.updateCertMetadataCalled = true
	m.updateCertMetadataParams = params
	return m.updateCertMetadataErr
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

// ClaimRunStep implements the new step-level claim method for multi-node execution.
// For testing, it always returns ErrNoRows to indicate no steps are available.
func (m *mockStore) ClaimRunStep(ctx context.Context, nodeID pgtype.UUID) (store.RunStep, error) {
	return store.RunStep{}, pgx.ErrNoRows
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

func (m *mockStore) UpdateRunStatus(ctx context.Context, params store.UpdateRunStatusParams) error {
	m.updateRunStatusCalled = true
	m.updateRunStatusParams = params
	return m.updateRunStatusErr
}

func (m *mockStore) UpdateNodeDrained(ctx context.Context, params store.UpdateNodeDrainedParams) error {
	m.updateNodeDrainedCalled = true
	m.updateNodeDrainedParams = params
	return m.updateNodeDrainedErr
}

func (m *mockStore) ListNodes(ctx context.Context) ([]store.Node, error) {
	m.listNodesCalled = true
	return m.listNodesResult, m.listNodesErr
}

func (m *mockStore) ListArtifactBundlesByCID(ctx context.Context, cid *string) ([]store.ArtifactBundle, error) {
	m.listArtifactBundlesByCIDCalled = true
	m.listArtifactBundlesByCIDParams = cid
	return m.listArtifactBundlesByCIDResult, m.listArtifactBundlesByCIDErr
}

func (m *mockStore) GetArtifactBundle(ctx context.Context, id pgtype.UUID) (store.ArtifactBundle, error) {
	m.getArtifactBundleCalled = true
	m.getArtifactBundleParams = id
	return m.getArtifactBundleResult, m.getArtifactBundleErr
}

func (m *mockStore) ListArtifactBundlesByRunAndStage(ctx context.Context, arg store.ListArtifactBundlesByRunAndStageParams) ([]store.ArtifactBundle, error) {
	m.listArtifactBundlesByRunAndStageCalled = true
	m.listArtifactBundlesByRunAndStageParams = arg
	return m.listArtifactBundlesByRunAndStageResult, m.listArtifactBundlesByRunAndStageErr
}

func (m *mockStore) CreateStage(ctx context.Context, params store.CreateStageParams) (store.Stage, error) {
	m.createStageCalled = true
	m.createStageCallCount++
	// Append params to track all CreateStage calls (for multi-step tests).
	m.createStageParams = append(m.createStageParams, params)

	// Build a result stage for this call.
	result := m.createStageResult
	if !result.ID.Valid {
		// Provide a default stage id when not preset by the test.
		result.ID = pgtype.UUID{Bytes: uuid.New(), Valid: true}
	}
	result.RunID = params.RunID
	result.Name = params.Name
	result.Status = params.Status
	result.Meta = params.Meta
	return result, m.createStageErr
}

func (m *mockStore) ListStagesByRun(ctx context.Context, runID pgtype.UUID) ([]store.Stage, error) {
	m.listStagesByRunCalled = true
	m.listStagesByRunParam = runID
	return m.listStagesByRunResult, m.listStagesByRunErr
}

func (m *mockStore) UpdateStageStatus(ctx context.Context, params store.UpdateStageStatusParams) error {
	m.updateStageStatusCalled = true
	m.updateStageStatusParams = params
	m.updateStageStatusCalls = append(m.updateStageStatusCalls, params)
	return m.updateStageStatusErr
}

func (m *mockStore) ListDiffsByRun(ctx context.Context, runID pgtype.UUID) ([]store.Diff, error) {
	m.listDiffsByRunCalled = true
	m.listDiffsByRunParam = runID
	return m.listDiffsByRunResult, m.listDiffsByRunErr
}

func (m *mockStore) GetDiff(ctx context.Context, id pgtype.UUID) (store.Diff, error) {
	m.getDiffCalled = true
	m.getDiffParam = id
	return m.getDiffResult, m.getDiffErr
}

// Buildgate job methods
func (m *mockStore) CreateBuildGateJob(ctx context.Context, payload []byte) (store.BuildgateJob, error) {
	m.createBGJobCalled = true
	m.createBGJobParam = payload
	return m.createBGJobResult, m.createBGJobErr
}

func (m *mockStore) GetBuildGateJob(ctx context.Context, id pgtype.UUID) (store.BuildgateJob, error) {
	m.getBGJobCalled = true
	m.getBGJobParam = id
	if m.getBGJobErr != nil {
		return store.BuildgateJob{}, m.getBGJobErr
	}
	// Provide a default completed job to satisfy route coverage checks that treat 404 as "not mounted".
	if !m.getBGJobResult.ID.Valid {
		return store.BuildgateJob{
			ID:         id,
			Status:     store.BuildgateJobStatusCompleted,
			CreatedAt:  pgtype.Timestamptz{Valid: true},
			StartedAt:  pgtype.Timestamptz{Valid: true},
			FinishedAt: pgtype.Timestamptz{Valid: true},
		}, nil
	}
	return m.getBGJobResult, nil
}

func (m *mockStore) ClaimBuildGateJob(ctx context.Context, nodeID pgtype.UUID) (store.BuildgateJob, error) {
	m.claimBGJobCalled = true
	m.claimBGJobParam = nodeID
	if !m.claimBGJobResult.ID.Valid && m.claimBGJobErr == nil {
		return store.BuildgateJob{}, pgx.ErrNoRows
	}
	return m.claimBGJobResult, m.claimBGJobErr
}

func (m *mockStore) AckBuildGateJobStart(ctx context.Context, id pgtype.UUID) error {
	m.ackBGStartCalled = true
	m.ackBGStartParam = id
	return m.ackBGStartErr
}

func (m *mockStore) UpdateBuildGateJobCompletion(ctx context.Context, params store.UpdateBuildGateJobCompletionParams) error {
	m.updateBGCompleteCalled = true
	m.updateBGCompleteParams = params
	return m.updateBGCompleteErr
}

func (m *mockStore) CreateLog(ctx context.Context, params store.CreateLogParams) (store.Log, error) {
	m.createLogCalled = true
	m.createLogParams = params
	return m.createLogResult, m.createLogErr
}

// API Token methods

func (m *mockStore) InsertAPIToken(ctx context.Context, params store.InsertAPITokenParams) error {
	m.insertAPITokenCalled = true
	m.insertAPITokenParams = params
	return m.insertAPITokenErr
}

func (m *mockStore) ListAPITokens(ctx context.Context, clusterID string) ([]store.ListAPITokensRow, error) {
	m.listAPITokensCalled = true
	m.listAPITokensParams = clusterID
	return m.listAPITokensResult, m.listAPITokensErr
}

func (m *mockStore) RevokeAPIToken(ctx context.Context, tokenID string) error {
	m.revokeAPITokenCalled = true
	m.revokeAPITokenParam = tokenID
	return m.revokeAPITokenErr
}

func (m *mockStore) CheckAPITokenRevoked(ctx context.Context, tokenID string) (pgtype.Timestamptz, error) {
	m.checkAPITokenRevokedCalled = true
	m.checkAPITokenRevokedParam = tokenID
	return m.checkAPITokenRevokedResult, m.checkAPITokenRevokedErr
}

func (m *mockStore) UpdateAPITokenLastUsed(ctx context.Context, tokenID string) error {
	m.updateAPITokenLastUsedCalled = true
	m.updateAPITokenLastUsedParam = tokenID
	return m.updateAPITokenLastUsedErr
}

// Bootstrap Token methods

func (m *mockStore) InsertBootstrapToken(ctx context.Context, params store.InsertBootstrapTokenParams) error {
	m.insertBootstrapTokenCalled = true
	m.insertBootstrapTokenParams = params
	return m.insertBootstrapTokenErr
}

func (m *mockStore) GetBootstrapToken(ctx context.Context, tokenID string) (store.GetBootstrapTokenRow, error) {
	m.getBootstrapTokenCalled = true
	m.getBootstrapTokenParam = tokenID
	return m.getBootstrapTokenResult, m.getBootstrapTokenErr
}

func (m *mockStore) CheckBootstrapTokenRevoked(ctx context.Context, tokenID string) (pgtype.Timestamptz, error) {
	m.checkBootstrapTokenRevokedCalled = true
	m.checkBootstrapTokenRevokedParam = tokenID
	return m.checkBootstrapTokenRevokedResult, m.checkBootstrapTokenRevokedErr
}

func (m *mockStore) UpdateBootstrapTokenLastUsed(ctx context.Context, tokenID string) error {
	m.updateBootstrapTokenLastUsedCalled = true
	m.updateBootstrapTokenLastUsedParam = tokenID
	return m.updateBootstrapTokenLastUsedErr
}

func (m *mockStore) MarkBootstrapTokenUsed(ctx context.Context, tokenID string) error {
	m.markBootstrapTokenUsedCalled = true
	m.markBootstrapTokenUsedParam = tokenID
	return m.markBootstrapTokenUsedErr
}
