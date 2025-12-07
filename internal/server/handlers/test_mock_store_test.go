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

	claimJobCalled bool
	claimJobParams pgtype.UUID
	claimJobResult store.Job
	claimJobErr    error

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

	getJobCalled bool
	getJobParams pgtype.UUID
	getJobResult store.Job
	getJobErr    error

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

	// UpdateRunResume tracking (resume_count, last_resumed_at)
	updateRunResumeCalled bool
	updateRunResumeParam  pgtype.UUID
	updateRunResumeErr    error

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

	// ListArtifactBundlesByRunAndJob tracking
	listArtifactBundlesByRunAndJobCalled bool
	listArtifactBundlesByRunAndJobParams store.ListArtifactBundlesByRunAndJobParams
	listArtifactBundlesByRunAndJobResult []store.ArtifactBundle
	listArtifactBundlesByRunAndJobErr    error

	// CreateJob tracking
	createJobCalled    bool
	createJobCallCount int
	createJobParams    []store.CreateJobParams
	createJobResult    store.Job
	createJobErr       error

	// ListJobsByRun tracking
	listJobsByRunCalled bool
	listJobsByRunParam  pgtype.UUID
	listJobsByRunResult []store.Job
	listJobsByRunErr    error

	// CountJobsByRun tracking
	countJobsByRunCalled bool
	countJobsByRunParam  pgtype.UUID
	countJobsByRunResult int64
	countJobsByRunErr    error

	// CountJobsByRunAndStatus tracking
	countJobsByRunAndStatusCalled bool
	countJobsByRunAndStatusParams store.CountJobsByRunAndStatusParams
	countJobsByRunAndStatusResult int64
	countJobsByRunAndStatusErr    error

	// UpdateJobStatus tracking
	updateJobStatusCalled bool
	updateJobStatusParams store.UpdateJobStatusParams
	updateJobStatusCalls  []store.UpdateJobStatusParams
	updateJobStatusErr    error

	// UpdateJobCompletion tracking
	updateJobCompletionCalled bool
	updateJobCompletionParams store.UpdateJobCompletionParams
	updateJobCompletionErr    error

	// ScheduleNextJob tracking
	scheduleNextJobCalled bool
	scheduleNextJobParam  pgtype.UUID
	scheduleNextJobResult store.Job
	scheduleNextJobErr    error

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
	listAPITokensParams *string // cluster_id (nullable)
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

	// ListRuns tracking (for batch run handlers)
	listRunsCalled bool
	listRunsParams store.ListRunsParams
	listRunsResult []store.Run
	listRunsErr    error

	// ListRunReposByRun tracking — run IDs are now strings (KSUID).
	listRunReposByRunCalled bool
	listRunReposByRunParam  string
	listRunReposByRunResult []store.RunRepo
	listRunReposByRunErr    error

	// CountRunReposByStatus tracking — run IDs are now strings (KSUID).
	countRunReposByStatusCalled bool
	countRunReposByStatusParam  string
	countRunReposByStatusResult []store.CountRunReposByStatusRow
	countRunReposByStatusErr    error

	// UpdateRunRepoRefs tracking
	updateRunRepoRefsCalled bool
	updateRunRepoRefsParams store.UpdateRunRepoRefsParams
	updateRunRepoRefsErr    error

	// UpdateRunRepoStatus tracking
	updateRunRepoStatusCalled bool
	updateRunRepoStatusParams []store.UpdateRunRepoStatusParams
	updateRunRepoStatusErr    error

	// CreateRunRepo tracking
	createRunRepoCalled bool
	createRunRepoParams store.CreateRunRepoParams
	createRunRepoResult store.RunRepo
	createRunRepoErr    error

	// GetRunRepo tracking — repo IDs are now strings (NanoID).
	getRunRepoCalled bool
	getRunRepoParam  string
	getRunRepoResult store.RunRepo
	getRunRepoErr    error

	// IncrementRunRepoAttempt tracking — repo IDs are now strings (NanoID).
	incrementRunRepoAttemptCalled bool
	incrementRunRepoAttemptParam  string
	incrementRunRepoAttemptErr    error

	// ListPendingRunReposByRun tracking — run IDs are now strings (KSUID).
	listPendingRunReposByRunCalled bool
	listPendingRunReposByRunParam  string
	listPendingRunReposByRunResult []store.RunRepo
	listPendingRunReposByRunErr    error

	// SetRunRepoExecutionRun tracking — repo IDs are strings (NanoID), execution run IDs are strings (KSUID).
	setRunRepoExecutionRunCalled bool
	setRunRepoExecutionRunParams []store.SetRunRepoExecutionRunParams
	setRunRepoExecutionRunErr    error

	// GetRunRepoByExecutionRun tracking — execution run IDs are now strings (KSUID).
	getRunRepoByExecutionRunCalled bool
	getRunRepoByExecutionRunParam  *string
	getRunRepoByExecutionRunResult store.RunRepo
	getRunRepoByExecutionRunErr    error

	// ClearRunRepoExecutionRun tracking — repo IDs are now strings (NanoID).
	clearRunRepoExecutionRunCalled bool
	clearRunRepoExecutionRunParam  string
	clearRunRepoExecutionRunErr    error

	// ListDistinctRepos tracking (for repo-centric handlers)
	listDistinctReposCalled bool
	listDistinctReposParam  string
	listDistinctReposResult []store.ListDistinctReposRow
	listDistinctReposErr    error

	// ListRunsForRepo tracking
	listRunsForRepoCalled bool
	listRunsForRepoParams store.ListRunsForRepoParams
	listRunsForRepoResult []store.ListRunsForRepoRow
	listRunsForRepoErr    error
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

// ClaimJob implements job claiming for the new unified job model.
func (m *mockStore) ClaimJob(ctx context.Context, nodeID pgtype.UUID) (store.Job, error) {
	m.claimJobCalled = true
	m.claimJobParams = nodeID
	if m.claimJobErr != nil {
		return store.Job{}, m.claimJobErr
	}
	if !m.claimJobResult.ID.Valid {
		return store.Job{}, pgx.ErrNoRows
	}
	return m.claimJobResult, nil
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

func (m *mockStore) GetJob(ctx context.Context, id pgtype.UUID) (store.Job, error) {
	m.getJobCalled = true
	m.getJobParams = id
	return m.getJobResult, m.getJobErr
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

func (m *mockStore) UpdateRunResume(ctx context.Context, id pgtype.UUID) error {
	m.updateRunResumeCalled = true
	m.updateRunResumeParam = id
	return m.updateRunResumeErr
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

func (m *mockStore) ListArtifactBundlesByRunAndJob(ctx context.Context, arg store.ListArtifactBundlesByRunAndJobParams) ([]store.ArtifactBundle, error) {
	m.listArtifactBundlesByRunAndJobCalled = true
	m.listArtifactBundlesByRunAndJobParams = arg
	return m.listArtifactBundlesByRunAndJobResult, m.listArtifactBundlesByRunAndJobErr
}

func (m *mockStore) CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error) {
	m.createJobCalled = true
	m.createJobCallCount++
	// Append params to track all CreateJob calls (for multi-step tests).
	m.createJobParams = append(m.createJobParams, params)

	// Build a result job for this call.
	result := m.createJobResult
	if !result.ID.Valid {
		// Provide a default job id when not preset by the test.
		result.ID = pgtype.UUID{Bytes: uuid.New(), Valid: true}
	}
	result.RunID = params.RunID
	result.Name = params.Name
	result.Status = params.Status
	result.ModType = params.ModType
	result.ModImage = params.ModImage
	result.Meta = params.Meta
	return result, m.createJobErr
}

func (m *mockStore) ListJobsByRun(ctx context.Context, runID pgtype.UUID) ([]store.Job, error) {
	m.listJobsByRunCalled = true
	m.listJobsByRunParam = runID

	// Return a copy with updated status from UpdateJobCompletion applied.
	// This ensures maybeCompleteMultiStepRun sees the correct job statuses.
	result := make([]store.Job, len(m.listJobsByRunResult))
	for i, j := range m.listJobsByRunResult {
		result[i] = j
		// If this job was updated via UpdateJobCompletion, reflect the new status.
		if m.updateJobCompletionCalled && j.ID == m.updateJobCompletionParams.ID {
			result[i].Status = m.updateJobCompletionParams.Status
		}
	}
	return result, m.listJobsByRunErr
}

func (m *mockStore) CountJobsByRun(ctx context.Context, runID pgtype.UUID) (int64, error) {
	m.countJobsByRunCalled = true
	m.countJobsByRunParam = runID
	if m.countJobsByRunErr != nil {
		return 0, m.countJobsByRunErr
	}
	// Default: count from listJobsByRunResult if not explicitly set.
	if m.countJobsByRunResult == 0 && len(m.listJobsByRunResult) > 0 {
		return int64(len(m.listJobsByRunResult)), nil
	}
	return m.countJobsByRunResult, nil
}

func (m *mockStore) CountJobsByRunAndStatus(ctx context.Context, arg store.CountJobsByRunAndStatusParams) (int64, error) {
	m.countJobsByRunAndStatusCalled = true
	m.countJobsByRunAndStatusParams = arg
	if m.countJobsByRunAndStatusErr != nil {
		return 0, m.countJobsByRunAndStatusErr
	}
	// Default: count matching jobs from listJobsByRunResult, accounting for job completions.
	if m.countJobsByRunAndStatusResult == 0 && len(m.listJobsByRunResult) > 0 {
		var count int64
		for _, j := range m.listJobsByRunResult {
			// If this job was marked as completed via UpdateJobCompletion, use the completed status.
			effectiveStatus := j.Status
			if m.updateJobCompletionCalled && j.ID == m.updateJobCompletionParams.ID {
				effectiveStatus = m.updateJobCompletionParams.Status
			}
			if effectiveStatus == arg.Status {
				count++
			}
		}
		return count, nil
	}
	return m.countJobsByRunAndStatusResult, nil
}

func (m *mockStore) UpdateJobStatus(ctx context.Context, params store.UpdateJobStatusParams) error {
	m.updateJobStatusCalled = true
	m.updateJobStatusParams = params
	m.updateJobStatusCalls = append(m.updateJobStatusCalls, params)
	return m.updateJobStatusErr
}

func (m *mockStore) UpdateJobCompletion(ctx context.Context, params store.UpdateJobCompletionParams) error {
	m.updateJobCompletionCalled = true
	m.updateJobCompletionParams = params
	return m.updateJobCompletionErr
}

func (m *mockStore) ScheduleNextJob(ctx context.Context, runID pgtype.UUID) (store.Job, error) {
	m.scheduleNextJobCalled = true
	m.scheduleNextJobParam = runID
	if m.scheduleNextJobErr != nil {
		return store.Job{}, m.scheduleNextJobErr
	}
	// Return no rows by default if no result configured.
	if !m.scheduleNextJobResult.ID.Valid {
		return store.Job{}, pgx.ErrNoRows
	}
	return m.scheduleNextJobResult, nil
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

func (m *mockStore) ListAPITokens(ctx context.Context, clusterID *string) ([]store.ListAPITokensRow, error) {
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

// RunRepo methods for batch run handlers

func (m *mockStore) ListRuns(ctx context.Context, params store.ListRunsParams) ([]store.Run, error) {
	m.listRunsCalled = true
	m.listRunsParams = params
	return m.listRunsResult, m.listRunsErr
}

// ListRunReposByRun — run IDs are now strings (KSUID).
func (m *mockStore) ListRunReposByRun(ctx context.Context, runID string) ([]store.RunRepo, error) {
	m.listRunReposByRunCalled = true
	m.listRunReposByRunParam = runID
	return m.listRunReposByRunResult, m.listRunReposByRunErr
}

// CountRunReposByStatus — run IDs are now strings (KSUID).
func (m *mockStore) CountRunReposByStatus(ctx context.Context, runID string) ([]store.CountRunReposByStatusRow, error) {
	m.countRunReposByStatusCalled = true
	m.countRunReposByStatusParam = runID
	return m.countRunReposByStatusResult, m.countRunReposByStatusErr
}

func (m *mockStore) UpdateRunRepoRefs(ctx context.Context, params store.UpdateRunRepoRefsParams) error {
	m.updateRunRepoRefsCalled = true
	m.updateRunRepoRefsParams = params
	return m.updateRunRepoRefsErr
}

func (m *mockStore) UpdateRunRepoStatus(ctx context.Context, params store.UpdateRunRepoStatusParams) error {
	m.updateRunRepoStatusCalled = true
	m.updateRunRepoStatusParams = append(m.updateRunRepoStatusParams, params)
	return m.updateRunRepoStatusErr
}

func (m *mockStore) CreateRunRepo(ctx context.Context, params store.CreateRunRepoParams) (store.RunRepo, error) {
	m.createRunRepoCalled = true
	m.createRunRepoParams = params
	return m.createRunRepoResult, m.createRunRepoErr
}

// GetRunRepo — repo IDs are now strings (NanoID).
func (m *mockStore) GetRunRepo(ctx context.Context, id string) (store.RunRepo, error) {
	m.getRunRepoCalled = true
	m.getRunRepoParam = id
	return m.getRunRepoResult, m.getRunRepoErr
}

// IncrementRunRepoAttempt — repo IDs are now strings (NanoID).
func (m *mockStore) IncrementRunRepoAttempt(ctx context.Context, id string) error {
	m.incrementRunRepoAttemptCalled = true
	m.incrementRunRepoAttemptParam = id
	return m.incrementRunRepoAttemptErr
}

// ListPendingRunReposByRun — run IDs are now strings (KSUID).
func (m *mockStore) ListPendingRunReposByRun(ctx context.Context, runID string) ([]store.RunRepo, error) {
	m.listPendingRunReposByRunCalled = true
	m.listPendingRunReposByRunParam = runID
	return m.listPendingRunReposByRunResult, m.listPendingRunReposByRunErr
}

// SetRunRepoExecutionRun links a run_repo to its child execution run.
func (m *mockStore) SetRunRepoExecutionRun(ctx context.Context, arg store.SetRunRepoExecutionRunParams) error {
	m.setRunRepoExecutionRunCalled = true
	m.setRunRepoExecutionRunParams = append(m.setRunRepoExecutionRunParams, arg)
	return m.setRunRepoExecutionRunErr
}

// GetRunRepoByExecutionRun — execution run IDs are now strings (KSUID).
func (m *mockStore) GetRunRepoByExecutionRun(ctx context.Context, executionRunID *string) (store.RunRepo, error) {
	m.getRunRepoByExecutionRunCalled = true
	m.getRunRepoByExecutionRunParam = executionRunID
	return m.getRunRepoByExecutionRunResult, m.getRunRepoByExecutionRunErr
}

// ClearRunRepoExecutionRun — repo IDs are now strings (NanoID).
func (m *mockStore) ClearRunRepoExecutionRun(ctx context.Context, id string) error {
	m.clearRunRepoExecutionRunCalled = true
	m.clearRunRepoExecutionRunParam = id
	return m.clearRunRepoExecutionRunErr
}

// ListDistinctRepos returns distinct repos with optional substring filter.
func (m *mockStore) ListDistinctRepos(ctx context.Context, filter string) ([]store.ListDistinctReposRow, error) {
	m.listDistinctReposCalled = true
	m.listDistinctReposParam = filter
	return m.listDistinctReposResult, m.listDistinctReposErr
}

// ListRunsForRepo returns runs for a specific repository URL.
func (m *mockStore) ListRunsForRepo(ctx context.Context, arg store.ListRunsForRepoParams) ([]store.ListRunsForRepoRow, error) {
	m.listRunsForRepoCalled = true
	m.listRunsForRepoParams = arg
	return m.listRunsForRepoResult, m.listRunsForRepoErr
}
