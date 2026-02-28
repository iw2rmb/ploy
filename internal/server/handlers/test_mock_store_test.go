package handlers

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// mockStore is a minimal Store implementation for testing handlers.
type mockStore struct {
	store.Store
	updateCertMetadataCalled bool
	updateCertMetadataParams store.UpdateNodeCertMetadataParams
	updateCertMetadataErr    error

	// v1 migs/specs/mig_repos tracking (used by /v1/migs and /v1/runs handlers)
	createSpecCalled bool
	createSpecParams store.CreateSpecParams
	createSpecResult store.Spec
	createSpecErr    error

	updateModSpecCalled bool
	updateModSpecParams store.UpdateMigSpecParams
	updateModSpecErr    error

	createMigCalled bool
	createMigParams store.CreateMigParams
	createMigResult store.Mig
	createMigErr    error

	listMigsCalled bool
	listMigsParams store.ListMigsParams
	listMigsResult []store.Mig
	listMigsErr    error

	getModCalled bool
	getModParam  string
	getModResult store.Mig
	getModErr    error

	getModByNameCalled bool
	getModByNameParam  string
	getModByNameResult store.Mig
	getModByNameErr    error

	deleteMigCalled bool
	deleteMigParam  string
	deleteMigErr    error

	archiveMigCalled bool
	archiveMigParam  string
	archiveMigErr    error

	unarchiveMigCalled bool
	unarchiveMigParam  string
	unarchiveMigErr    error

	createMigRepoCalled bool
	createMigRepoParams store.CreateMigRepoParams
	createMigRepoResult store.MigRepo
	createMigRepoErr    error

	getModRepoCalled bool
	getModRepoParam  string
	getModRepoResult store.MigRepo
	getModRepoErr    error

	createRunCalled bool
	createRunParams store.CreateRunParams
	createRunResult store.Run
	createRunErr    error
	// createRunErrs allows tests to configure per-call CreateRun errors.
	// When non-empty, successive CreateRun calls return errors from this slice;
	// the last entry is reused for extra calls.
	createRunErrs         []error
	createRunErrCallCount int
	// createRunResults allows tests to configure multiple CreateRun return values.
	// When non-empty, successive CreateRun calls return entries from this slice;
	// the last entry is reused for extra calls.
	createRunResults   []store.Run
	createRunCallCount int

	getRunCalled bool
	getRunParams string
	getRunResult store.Run
	getRunErr    error

	getSpecCalled bool
	getSpecParam  string
	getSpecResult store.Spec
	getSpecErr    error

	getRunTimingCalled bool
	getRunTimingParams string
	getRunTimingResult store.RunsTiming
	getRunTimingErr    error

	listRunsTimingsCalled bool
	listRunsTimingsParams store.ListRunsTimingsParams
	listRunsTimingsResult []store.RunsTiming
	listRunsTimingsErr    error

	deleteRunCalled bool
	deleteRunParams string
	deleteRunErr    error

	claimRunCalled bool
	claimRunParams *string
	claimRunResult store.Run
	claimRunErr    error

	claimJobCalled bool
	claimJobParams types.NodeID
	claimJobResult store.Job
	claimJobErr    error

	getNodeCalled bool
	getNodeParams string
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
	getJobParams string
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
	ackRunStartParam  string
	ackRunStartErr    error

	// UpdateRunCompletion tracking
	updateRunCompletionCalled bool
	updateRunCompletionParams store.UpdateRunCompletionParams
	updateRunCompletionErr    error

	// UpdateRunStatus tracking
	updateRunStatusCalled bool
	updateRunStatusParams store.UpdateRunStatusParams
	updateRunStatusErr    error

	// CancelRunV1 tracking
	cancelRunV1Called bool
	cancelRunV1Param  string
	cancelRunV1Err    error

	// UpdateRunResume tracking (resume_count, last_resumed_at)
	updateRunResumeCalled bool
	updateRunResumeParam  string
	updateRunResumeErr    error

	// UpdateRunStatsMRURL tracking (MR URL propagation)
	updateRunStatsMRURLCalled bool
	updateRunStatsMRURLParams store.UpdateRunStatsMRURLParams
	updateRunStatsMRURLErr    error

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

	// ListArtifactBundlesMetaByCID tracking
	listArtifactBundlesMetaByCIDCalled bool
	listArtifactBundlesMetaByCIDParams *string
	listArtifactBundlesMetaByCIDResult []store.ArtifactBundle
	listArtifactBundlesMetaByCIDErr    error

	// ListArtifactBundlesByRun tracking
	listArtifactBundlesByRunCalled bool
	listArtifactBundlesByRunParam  string
	listArtifactBundlesByRunResult []store.ArtifactBundle
	listArtifactBundlesByRunErr    error

	// ListArtifactBundlesMetaByRun tracking
	listArtifactBundlesMetaByRunCalled bool
	listArtifactBundlesMetaByRunParam  string
	listArtifactBundlesMetaByRunResult []store.ArtifactBundle
	listArtifactBundlesMetaByRunErr    error

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

	// ListArtifactBundlesMetaByRunAndJob tracking
	listArtifactBundlesMetaByRunAndJobCalled bool
	listArtifactBundlesMetaByRunAndJobParams store.ListArtifactBundlesMetaByRunAndJobParams
	listArtifactBundlesMetaByRunAndJobResult []store.ArtifactBundle
	listArtifactBundlesMetaByRunAndJobErr    error

	// CreateJob tracking
	createJobCalled    bool
	createJobCallCount int
	createJobParams    []store.CreateJobParams
	createJobResult    store.Job
	createJobErr       error

	// ListJobsByRun tracking
	listJobsByRunCalled bool
	listJobsByRunParam  string
	listJobsByRunResult []store.Job
	listJobsByRunErr    error

	// ListJobsByRunRepoAttempt tracking (v1 repo-scoped jobs)
	listJobsByRunRepoAttemptCalled bool
	listJobsByRunRepoAttemptParams store.ListJobsByRunRepoAttemptParams
	listJobsByRunRepoAttemptResult []store.Job
	listJobsByRunRepoAttemptErr    error

	// CountJobsByRun tracking
	countJobsByRunCalled bool
	countJobsByRunParam  string
	countJobsByRunResult int64
	countJobsByRunErr    error

	// CountJobsByRunAndStatus tracking
	countJobsByRunAndStatusCalled bool
	countJobsByRunAndStatusParams store.CountJobsByRunAndStatusParams
	countJobsByRunAndStatusResult int64
	countJobsByRunAndStatusErr    error

	// CountJobsByRunRepoAttemptGroupByStatus tracking (v1 repo-scoped progression)
	countJobsByRunRepoAttemptGroupByStatusCalled bool
	countJobsByRunRepoAttemptGroupByStatusParams store.CountJobsByRunRepoAttemptGroupByStatusParams
	countJobsByRunRepoAttemptGroupByStatusResult []store.CountJobsByRunRepoAttemptGroupByStatusRow
	countJobsByRunRepoAttemptGroupByStatusErr    error

	// UpdateJobStatus tracking
	updateJobStatusCalled bool
	updateJobStatusParams store.UpdateJobStatusParams
	updateJobStatusCalls  []store.UpdateJobStatusParams
	updateJobStatusErr    error

	// UpdateJobCompletion tracking
	updateJobCompletionCalled bool
	updateJobCompletionParams store.UpdateJobCompletionParams
	updateJobCompletionErr    error

	// UpdateJobCompletionWithMeta tracking
	updateJobCompletionWithMetaCalled bool
	updateJobCompletionWithMetaParams store.UpdateJobCompletionWithMetaParams
	updateJobCompletionWithMetaErr    error

	// UpsertJobMetric tracking
	upsertJobMetricCalled bool
	upsertJobMetricParams store.UpsertJobMetricParams
	upsertJobMetricErr    error

	// UpdateJobImageName tracking
	updateJobImageNameCalled bool
	updateJobImageNameParams store.UpdateJobImageNameParams
	updateJobImageNameErr    error

	// ScheduleNextJob tracking
	scheduleNextJobCalled bool
	scheduleNextJobParam  store.ScheduleNextJobParams
	scheduleNextJobResult store.Job
	scheduleNextJobErr    error

	// PromoteJobByIDIfUnblocked tracking
	promoteJobByIDIfUnblockedCalled bool
	promoteJobByIDIfUnblockedParam  types.JobID
	promoteJobByIDIfUnblockedResult store.Job
	promoteJobByIDIfUnblockedErr    error

	// PromoteReGateRecoveryCandidateGateProfile tracking
	promoteReGateRecoveryCandidateGateProfileCalled bool
	promoteReGateRecoveryCandidateGateProfileParams store.PromoteReGateRecoveryCandidateGateProfileParams
	promoteReGateRecoveryCandidateGateProfileResult types.MigRepoID
	promoteReGateRecoveryCandidateGateProfileErr    error

	// UpdateJobNextID tracking
	updateJobNextIDCalled bool
	updateJobNextIDParams []store.UpdateJobNextIDParams
	updateJobNextIDErr    error

	// ListDiffsByRunRepo tracking (v1 repo-scoped diffs listing)
	listDiffsByRunRepoCalled bool
	listDiffsByRunRepoParams store.ListDiffsByRunRepoParams
	listDiffsByRunRepoResult []store.Diff
	listDiffsByRunRepoErr    error

	// ListDiffsMetaByRunRepo tracking (v1 repo-scoped diffs listing; metadata-only)
	listDiffsMetaByRunRepoCalled bool
	listDiffsMetaByRunRepoParams store.ListDiffsMetaByRunRepoParams
	listDiffsMetaByRunRepoResult []store.Diff
	listDiffsMetaByRunRepoErr    error

	// GetDiff tracking
	getDiffCalled bool
	getDiffParam  pgtype.UUID
	getDiffResult store.Diff
	getDiffErr    error

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

	// Stale running jobs recovery tracking
	listStaleRunningJobsCalled bool
	listStaleRunningJobsParam  pgtype.Timestamptz
	listStaleRunningJobsResult []store.ListStaleRunningJobsRow
	listStaleRunningJobsErr    error

	countStaleNodesWithRunningJobsCalled bool
	countStaleNodesWithRunningJobsParam  pgtype.Timestamptz
	countStaleNodesWithRunningJobsResult int64
	countStaleNodesWithRunningJobsErr    error

	cancelActiveJobsByRunRepoAttemptCalled bool
	cancelActiveJobsByRunRepoAttemptParams []store.CancelActiveJobsByRunRepoAttemptParams
	cancelActiveJobsByRunRepoAttemptResult int64
	cancelActiveJobsByRunRepoAttemptErr    error

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

	// UpdateRunRepoError tracking (for Stack Gate failures)
	updateRunRepoErrorCalled bool
	updateRunRepoErrorParams store.UpdateRunRepoErrorParams
	updateRunRepoErrorErr    error

	// CreateRunRepo tracking
	createRunRepoCalled bool
	createRunRepoParams store.CreateRunRepoParams
	createRunRepoResult store.RunRepo
	createRunRepoErr    error

	listMigReposByModCalled  bool
	listMigReposByModParam   string
	listMigReposByModResult  []store.MigRepo
	listMigReposByModResults map[string][]store.MigRepo
	listMigReposByModErr     error

	// GetMigRepoByURL tracking (for bulk upsert duplicate detection)
	getModRepoByURLCalled bool
	getModRepoByURLParams store.GetMigRepoByURLParams
	getModRepoByURLResult store.MigRepo
	getModRepoByURLErr    error

	// UpsertMigRepo tracking (for bulk upsert)
	upsertModRepoCalled bool
	upsertModRepoParams store.UpsertMigRepoParams
	upsertModRepoResult store.MigRepo
	upsertModRepoErr    error

	// DeleteMigRepo tracking
	deleteMigRepoCalled bool
	deleteMigRepoParam  string
	deleteMigRepoErr    error

	// HasMigRepoHistory tracking (for delete validation)
	hasModRepoHistoryCalled bool
	hasModRepoHistoryParam  string
	hasModRepoHistoryResult bool
	hasModRepoHistoryErr    error

	// ListFailedRepoIDsByMig tracking (for "failed" repo selection)
	listFailedRepoIDsByModCalled bool
	listFailedRepoIDsByModParam  string
	listFailedRepoIDsByModResult []types.MigRepoID
	listFailedRepoIDsByModErr    error

	// ListRunReposWithURLByRun tracking (for pull resolution)
	listRunReposWithURLByRunCalled bool
	listRunReposWithURLByRunParam  string
	listRunReposWithURLByRunResult []store.ListRunReposWithURLByRunRow
	listRunReposWithURLByRunErr    error

	// GetLatestRunRepoByMigAndRepoStatus tracking (for mig pull resolution)
	getLatestRunRepoByModAndRepoStatusCalled bool
	getLatestRunRepoByModAndRepoStatusParams store.GetLatestRunRepoByMigAndRepoStatusParams
	getLatestRunRepoByModAndRepoStatusResult store.GetLatestRunRepoByMigAndRepoStatusRow
	getLatestRunRepoByModAndRepoStatusErr    error

	// GetRunRepo tracking — composite key (run_id, repo_id).
	getRunRepoCalled bool
	getRunRepoParam  store.GetRunRepoParams
	getRunRepoResult store.RunRepo
	getRunRepoErr    error

	// IncrementRunRepoAttempt tracking — composite key (run_id, repo_id).
	incrementRunRepoAttemptCalled bool
	incrementRunRepoAttemptParam  store.IncrementRunRepoAttemptParams
	incrementRunRepoAttemptErr    error

	// ListQueuedRunReposByRun tracking — run IDs are now strings (KSUID).
	listQueuedRunReposByRunCalled bool
	listQueuedRunReposByRunParam  string
	listQueuedRunReposByRunResult []store.RunRepo
	listQueuedRunReposByRunErr    error

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

	// Global Env tracking (config_env table; see docs/envs/README.md#Global Env Configuration)
	listGlobalEnvCalled bool
	listGlobalEnvResult []store.ConfigEnv
	listGlobalEnvErr    error

	getGlobalEnvCalled bool
	getGlobalEnvParam  string
	getGlobalEnvResult store.ConfigEnv
	getGlobalEnvErr    error

	upsertGlobalEnvCalled bool
	upsertGlobalEnvParams store.UpsertGlobalEnvParams
	upsertGlobalEnvErr    error

	deleteGlobalEnvCalled bool
	deleteGlobalEnvParam  string
	deleteGlobalEnvErr    error
}

func (m *mockStore) UpdateNodeCertMetadata(ctx context.Context, params store.UpdateNodeCertMetadataParams) error {
	m.updateCertMetadataCalled = true
	m.updateCertMetadataParams = params
	return m.updateCertMetadataErr
}

func (m *mockStore) CreateSpec(ctx context.Context, params store.CreateSpecParams) (store.Spec, error) {
	m.createSpecCalled = true
	m.createSpecParams = params

	result := m.createSpecResult
	if result.ID.IsZero() {
		result.ID = params.ID
	}
	if result.Spec == nil {
		result.Spec = params.Spec
	}
	result.CreatedBy = params.CreatedBy
	return result, m.createSpecErr
}

func (m *mockStore) CreateMig(ctx context.Context, params store.CreateMigParams) (store.Mig, error) {
	m.createMigCalled = true
	m.createMigParams = params

	result := m.createMigResult
	if result.ID.IsZero() {
		result.ID = params.ID
	}
	if result.Name == "" {
		result.Name = params.Name
	}
	result.SpecID = params.SpecID
	result.CreatedBy = params.CreatedBy
	return result, m.createMigErr
}

func (m *mockStore) UpdateMigSpec(ctx context.Context, params store.UpdateMigSpecParams) error {
	m.updateModSpecCalled = true
	m.updateModSpecParams = params
	return m.updateModSpecErr
}

func (m *mockStore) ListMigs(ctx context.Context, params store.ListMigsParams) ([]store.Mig, error) {
	m.listMigsCalled = true
	m.listMigsParams = params
	// Simulate pagination: return empty list when offset exceeds available results.
	if int(params.Offset) >= len(m.listMigsResult) {
		return []store.Mig{}, m.listMigsErr
	}
	// Apply simple pagination simulation: return slice starting at offset, up to limit.
	end := int(params.Offset) + int(params.Limit)
	if end > len(m.listMigsResult) {
		end = len(m.listMigsResult)
	}
	return m.listMigsResult[params.Offset:end], m.listMigsErr
}

func (m *mockStore) GetMig(ctx context.Context, id types.MigID) (store.Mig, error) {
	m.getModCalled = true
	m.getModParam = id.String()
	if m.getModErr != nil {
		return store.Mig{}, m.getModErr
	}
	result := m.getModResult
	if result.ID.IsZero() {
		result.ID = id
	}
	if result.Name == "" {
		result.Name = "mig-" + id.String()
	}
	return result, nil
}

func (m *mockStore) GetMigByName(ctx context.Context, name string) (store.Mig, error) {
	m.getModByNameCalled = true
	m.getModByNameParam = name
	if m.getModByNameErr != nil {
		return store.Mig{}, m.getModByNameErr
	}

	// Default behavior: not found unless explicitly configured.
	result := m.getModByNameResult
	if result.ID.IsZero() && result.Name == "" {
		return store.Mig{}, pgx.ErrNoRows
	}
	if result.Name == "" {
		result.Name = name
	}
	return result, nil
}

func (m *mockStore) DeleteMig(ctx context.Context, id types.MigID) error {
	m.deleteMigCalled = true
	m.deleteMigParam = id.String()
	return m.deleteMigErr
}

func (m *mockStore) ArchiveMig(ctx context.Context, id types.MigID) error {
	m.archiveMigCalled = true
	m.archiveMigParam = id.String()
	return m.archiveMigErr
}

func (m *mockStore) UnarchiveMig(ctx context.Context, id types.MigID) error {
	m.unarchiveMigCalled = true
	m.unarchiveMigParam = id.String()
	return m.unarchiveMigErr
}

func (m *mockStore) CreateMigRepo(ctx context.Context, params store.CreateMigRepoParams) (store.MigRepo, error) {
	m.createMigRepoCalled = true
	m.createMigRepoParams = params

	result := m.createMigRepoResult
	if result.ID.IsZero() {
		result.ID = params.ID
	}
	if result.MigID.IsZero() {
		result.MigID = params.MigID
	}
	if result.RepoUrl == "" {
		result.RepoUrl = params.RepoUrl
	}
	if result.BaseRef == "" {
		result.BaseRef = params.BaseRef
	}
	if result.TargetRef == "" {
		result.TargetRef = params.TargetRef
	}
	return result, m.createMigRepoErr
}

func (m *mockStore) GetMigRepo(ctx context.Context, id types.MigRepoID) (store.MigRepo, error) {
	m.getModRepoCalled = true
	m.getModRepoParam = id.String()
	return m.getModRepoResult, m.getModRepoErr
}

func (m *mockStore) ListMigReposByMig(ctx context.Context, modID types.MigID) ([]store.MigRepo, error) {
	m.listMigReposByModCalled = true
	modIDStr := modID.String()
	m.listMigReposByModParam = modIDStr
	if m.listMigReposByModResults != nil {
		if repos, ok := m.listMigReposByModResults[modIDStr]; ok {
			return repos, m.listMigReposByModErr
		}
	}
	return m.listMigReposByModResult, m.listMigReposByModErr
}

func (m *mockStore) CreateRun(ctx context.Context, params store.CreateRunParams) (store.Run, error) {
	m.createRunCalled = true
	m.createRunParams = params

	err := m.createRunErr
	if len(m.createRunErrs) > 0 {
		idx := m.createRunErrCallCount
		if idx >= len(m.createRunErrs) {
			idx = len(m.createRunErrs) - 1
		}
		m.createRunErrCallCount++
		err = m.createRunErrs[idx]
	}

	// When multiple results are configured, return them in order.
	if len(m.createRunResults) > 0 {
		idx := m.createRunCallCount
		if idx >= len(m.createRunResults) {
			idx = len(m.createRunResults) - 1
		}
		m.createRunCallCount++
		return m.createRunResults[idx], err
	}
	result := m.createRunResult
	if result.ID.IsZero() {
		result.ID = params.ID
	}
	if result.MigID.IsZero() {
		result.MigID = params.MigID
	}
	if result.SpecID.IsZero() {
		result.SpecID = params.SpecID
	}
	result.CreatedBy = params.CreatedBy
	return result, err
}

func (m *mockStore) GetRun(ctx context.Context, id types.RunID) (store.Run, error) {
	m.getRunCalled = true
	m.getRunParams = id.String()
	return m.getRunResult, m.getRunErr
}

func (m *mockStore) GetSpec(ctx context.Context, id types.SpecID) (store.Spec, error) {
	m.getSpecCalled = true
	m.getSpecParam = id.String()
	return m.getSpecResult, m.getSpecErr
}

func (m *mockStore) GetRunTiming(ctx context.Context, id types.RunID) (store.RunsTiming, error) {
	m.getRunTimingCalled = true
	m.getRunTimingParams = id.String()
	return m.getRunTimingResult, m.getRunTimingErr
}

func (m *mockStore) ListRunsTimings(ctx context.Context, arg store.ListRunsTimingsParams) ([]store.RunsTiming, error) {
	m.listRunsTimingsCalled = true
	m.listRunsTimingsParams = arg
	return m.listRunsTimingsResult, m.listRunsTimingsErr
}

func (m *mockStore) DeleteRun(ctx context.Context, id types.RunID) error {
	m.deleteRunCalled = true
	m.deleteRunParams = id.String()
	return m.deleteRunErr
}

func (m *mockStore) ClaimRun(ctx context.Context, nodeID *string) (store.Run, error) {
	m.claimRunCalled = true
	m.claimRunParams = nodeID
	return m.claimRunResult, m.claimRunErr
}

// ClaimJob implements job claiming for the new unified job model.
// Requires a non-empty nodeID (ErrEmptyNodeID is returned otherwise).
func (m *mockStore) ClaimJob(ctx context.Context, nodeID types.NodeID) (store.Job, error) {
	m.claimJobCalled = true
	m.claimJobParams = nodeID
	if nodeID.IsZero() {
		return store.Job{}, store.ErrEmptyNodeID
	}
	if m.claimJobErr != nil {
		return store.Job{}, m.claimJobErr
	}
	if m.claimJobResult.ID.IsZero() {
		return store.Job{}, pgx.ErrNoRows
	}
	return m.claimJobResult, nil
}

func (m *mockStore) GetNode(ctx context.Context, id types.NodeID) (store.Node, error) {
	m.getNodeCalled = true
	m.getNodeParams = id.String()
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

func (m *mockStore) GetJob(ctx context.Context, id types.JobID) (store.Job, error) {
	m.getJobCalled = true
	m.getJobParams = id.String()
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

func (m *mockStore) AckRunStart(ctx context.Context, id string) error {
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

func (m *mockStore) CancelRunV1(ctx context.Context, runID types.RunID) error {
	m.cancelRunV1Called = true
	m.cancelRunV1Param = runID.String()
	return m.cancelRunV1Err
}

func (m *mockStore) UpdateRunResume(ctx context.Context, id types.RunID) error {
	m.updateRunResumeCalled = true
	m.updateRunResumeParam = id.String()
	return m.updateRunResumeErr
}

func (m *mockStore) UpdateRunStatsMRURL(ctx context.Context, params store.UpdateRunStatsMRURLParams) error {
	m.updateRunStatsMRURLCalled = true
	m.updateRunStatsMRURLParams = params
	return m.updateRunStatsMRURLErr
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

func (m *mockStore) CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error) {
	m.createJobCalled = true
	m.createJobCallCount++
	// Append params to track all CreateJob calls (for multi-step tests).
	m.createJobParams = append(m.createJobParams, params)

	// Build a result job for this call.
	result := m.createJobResult
	if result.ID.IsZero() {
		// Provide a default job id when not preset by the test.
		result.ID = types.NewJobID()
	}
	result.RunID = params.RunID
	result.RepoID = params.RepoID
	result.RepoBaseRef = params.RepoBaseRef
	result.Attempt = params.Attempt
	result.Name = params.Name
	result.Status = params.Status
	result.JobType = params.JobType
	result.JobImage = params.JobImage
	result.NextID = params.NextID
	result.Meta = params.Meta
	return result, m.createJobErr
}

func (m *mockStore) ListJobsByRun(ctx context.Context, runID types.RunID) ([]store.Job, error) {
	m.listJobsByRunCalled = true
	m.listJobsByRunParam = runID.String()

	// Return a copy with updated status from UpdateJobCompletion applied.
	// This ensures maybeCompleteRunIfAllReposTerminal sees the correct job statuses.
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

func (m *mockStore) ListJobsByRunRepoAttempt(ctx context.Context, arg store.ListJobsByRunRepoAttemptParams) ([]store.Job, error) {
	m.listJobsByRunRepoAttemptCalled = true
	m.listJobsByRunRepoAttemptParams = arg
	return m.listJobsByRunRepoAttemptResult, m.listJobsByRunRepoAttemptErr
}

func (m *mockStore) ListStaleRunningJobs(ctx context.Context, lastHeartbeat pgtype.Timestamptz) ([]store.ListStaleRunningJobsRow, error) {
	m.listStaleRunningJobsCalled = true
	m.listStaleRunningJobsParam = lastHeartbeat
	return m.listStaleRunningJobsResult, m.listStaleRunningJobsErr
}

func (m *mockStore) CountStaleNodesWithRunningJobs(ctx context.Context, lastHeartbeat pgtype.Timestamptz) (int64, error) {
	m.countStaleNodesWithRunningJobsCalled = true
	m.countStaleNodesWithRunningJobsParam = lastHeartbeat
	return m.countStaleNodesWithRunningJobsResult, m.countStaleNodesWithRunningJobsErr
}

func (m *mockStore) CancelActiveJobsByRunRepoAttempt(ctx context.Context, params store.CancelActiveJobsByRunRepoAttemptParams) (int64, error) {
	m.cancelActiveJobsByRunRepoAttemptCalled = true
	m.cancelActiveJobsByRunRepoAttemptParams = append(m.cancelActiveJobsByRunRepoAttemptParams, params)
	return m.cancelActiveJobsByRunRepoAttemptResult, m.cancelActiveJobsByRunRepoAttemptErr
}

func (m *mockStore) CountJobsByRun(ctx context.Context, runID types.RunID) (int64, error) {
	m.countJobsByRunCalled = true
	m.countJobsByRunParam = runID.String()
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

// CountJobsByRunRepoAttemptGroupByStatus returns job counts by status for a repo attempt.
// Used by maybeUpdateRunRepoStatus for v1 repo-scoped terminal detection.
func (m *mockStore) CountJobsByRunRepoAttemptGroupByStatus(ctx context.Context, arg store.CountJobsByRunRepoAttemptGroupByStatusParams) ([]store.CountJobsByRunRepoAttemptGroupByStatusRow, error) {
	m.countJobsByRunRepoAttemptGroupByStatusCalled = true
	m.countJobsByRunRepoAttemptGroupByStatusParams = arg
	return m.countJobsByRunRepoAttemptGroupByStatusResult, m.countJobsByRunRepoAttemptGroupByStatusErr
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

func (m *mockStore) UpdateJobCompletionWithMeta(ctx context.Context, params store.UpdateJobCompletionWithMetaParams) error {
	m.updateJobCompletionWithMetaCalled = true
	m.updateJobCompletionWithMetaParams = params
	return m.updateJobCompletionWithMetaErr
}

func (m *mockStore) UpsertJobMetric(ctx context.Context, params store.UpsertJobMetricParams) error {
	m.upsertJobMetricCalled = true
	m.upsertJobMetricParams = params
	return m.upsertJobMetricErr
}

func (m *mockStore) UpdateJobImageName(ctx context.Context, params store.UpdateJobImageNameParams) error {
	m.updateJobImageNameCalled = true
	m.updateJobImageNameParams = params
	return m.updateJobImageNameErr
}

func (m *mockStore) ScheduleNextJob(ctx context.Context, arg store.ScheduleNextJobParams) (store.Job, error) {
	m.scheduleNextJobCalled = true
	m.scheduleNextJobParam = arg
	if m.scheduleNextJobErr != nil {
		return store.Job{}, m.scheduleNextJobErr
	}
	// Return no rows by default if no result configured.
	if m.scheduleNextJobResult.ID.IsZero() {
		return store.Job{}, pgx.ErrNoRows
	}
	return m.scheduleNextJobResult, nil
}

func (m *mockStore) PromoteJobByIDIfUnblocked(ctx context.Context, id types.JobID) (store.Job, error) {
	m.promoteJobByIDIfUnblockedCalled = true
	m.promoteJobByIDIfUnblockedParam = id
	if m.promoteJobByIDIfUnblockedErr != nil {
		return store.Job{}, m.promoteJobByIDIfUnblockedErr
	}
	if !m.promoteJobByIDIfUnblockedResult.ID.IsZero() {
		return m.promoteJobByIDIfUnblockedResult, nil
	}
	for i := range m.listJobsByRunRepoAttemptResult {
		if m.listJobsByRunRepoAttemptResult[i].ID != id {
			continue
		}
		if m.listJobsByRunRepoAttemptResult[i].Status != store.JobStatusCreated {
			return store.Job{}, pgx.ErrNoRows
		}
		m.listJobsByRunRepoAttemptResult[i].Status = store.JobStatusQueued
		return m.listJobsByRunRepoAttemptResult[i], nil
	}
	for i := range m.listJobsByRunResult {
		if m.listJobsByRunResult[i].ID != id {
			continue
		}
		if m.listJobsByRunResult[i].Status != store.JobStatusCreated {
			return store.Job{}, pgx.ErrNoRows
		}
		m.listJobsByRunResult[i].Status = store.JobStatusQueued
		return m.listJobsByRunResult[i], nil
	}
	return store.Job{}, pgx.ErrNoRows
}

func (m *mockStore) PromoteReGateRecoveryCandidateGateProfile(ctx context.Context, arg store.PromoteReGateRecoveryCandidateGateProfileParams) (types.MigRepoID, error) {
	m.promoteReGateRecoveryCandidateGateProfileCalled = true
	m.promoteReGateRecoveryCandidateGateProfileParams = arg
	if m.promoteReGateRecoveryCandidateGateProfileErr != nil {
		return "", m.promoteReGateRecoveryCandidateGateProfileErr
	}
	if !m.promoteReGateRecoveryCandidateGateProfileResult.IsZero() {
		return m.promoteReGateRecoveryCandidateGateProfileResult, nil
	}
	// Default to the current job's repo when available to keep tests lightweight.
	if !m.getJobResult.RepoID.IsZero() {
		return m.getJobResult.RepoID, nil
	}
	return "", pgx.ErrNoRows
}

func (m *mockStore) UpdateJobNextID(ctx context.Context, params store.UpdateJobNextIDParams) error {
	m.updateJobNextIDCalled = true
	m.updateJobNextIDParams = append(m.updateJobNextIDParams, params)
	if m.updateJobNextIDErr != nil {
		return m.updateJobNextIDErr
	}
	for i := range m.listJobsByRunRepoAttemptResult {
		if m.listJobsByRunRepoAttemptResult[i].ID == params.ID {
			m.listJobsByRunRepoAttemptResult[i].NextID = params.NextID
		}
	}
	for i := range m.listJobsByRunResult {
		if m.listJobsByRunResult[i].ID == params.ID {
			m.listJobsByRunResult[i].NextID = params.NextID
		}
	}
	return nil
}

// ListDiffsByRunRepo implements the v1 repo-scoped diffs listing query.
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
	// Simulate pagination: return empty list when offset exceeds available results.
	if int(params.Offset) >= len(m.listRunsResult) {
		return []store.Run{}, m.listRunsErr
	}
	// Apply simple pagination simulation: return slice starting at offset, up to limit.
	end := int(params.Offset) + int(params.Limit)
	if end > len(m.listRunsResult) {
		end = len(m.listRunsResult)
	}
	return m.listRunsResult[params.Offset:end], m.listRunsErr
}

// ListRunReposByRun — run IDs are now strings (KSUID).
func (m *mockStore) ListRunReposByRun(ctx context.Context, runID types.RunID) ([]store.RunRepo, error) {
	m.listRunReposByRunCalled = true
	m.listRunReposByRunParam = runID.String()
	return m.listRunReposByRunResult, m.listRunReposByRunErr
}

// CountRunReposByStatus — run IDs are now strings (KSUID).
func (m *mockStore) CountRunReposByStatus(ctx context.Context, runID types.RunID) ([]store.CountRunReposByStatusRow, error) {
	m.countRunReposByStatusCalled = true
	m.countRunReposByStatusParam = runID.String()
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

func (m *mockStore) UpdateRunRepoError(ctx context.Context, params store.UpdateRunRepoErrorParams) error {
	m.updateRunRepoErrorCalled = true
	m.updateRunRepoErrorParams = params
	return m.updateRunRepoErrorErr
}

func (m *mockStore) CreateRunRepo(ctx context.Context, params store.CreateRunRepoParams) (store.RunRepo, error) {
	m.createRunRepoCalled = true
	m.createRunRepoParams = params
	result := m.createRunRepoResult
	if result.MigID.IsZero() {
		result.MigID = params.MigID
	}
	if result.RunID.IsZero() {
		result.RunID = params.RunID
	}
	if result.RepoID.IsZero() {
		result.RepoID = params.RepoID
	}
	if result.RepoBaseRef == "" {
		result.RepoBaseRef = params.RepoBaseRef
	}
	if result.RepoTargetRef == "" {
		result.RepoTargetRef = params.RepoTargetRef
	}
	if result.Status == "" {
		result.Status = store.RunRepoStatusQueued
	}
	if result.Attempt == 0 {
		result.Attempt = 1
	}
	return result, m.createRunRepoErr
}

// GetRunRepo — repo IDs are now strings (NanoID).
func (m *mockStore) GetRunRepo(ctx context.Context, arg store.GetRunRepoParams) (store.RunRepo, error) {
	m.getRunRepoCalled = true
	m.getRunRepoParam = arg
	return m.getRunRepoResult, m.getRunRepoErr
}

// IncrementRunRepoAttempt — repo IDs are now strings (NanoID).
func (m *mockStore) IncrementRunRepoAttempt(ctx context.Context, arg store.IncrementRunRepoAttemptParams) error {
	m.incrementRunRepoAttemptCalled = true
	m.incrementRunRepoAttemptParam = arg
	return m.incrementRunRepoAttemptErr
}

// ListQueuedRunReposByRun — run IDs are now strings (KSUID).
func (m *mockStore) ListQueuedRunReposByRun(ctx context.Context, runID types.RunID) ([]store.RunRepo, error) {
	m.listQueuedRunReposByRunCalled = true
	m.listQueuedRunReposByRunParam = runID.String()
	return m.listQueuedRunReposByRunResult, m.listQueuedRunReposByRunErr
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

// Global Env methods (config_env table)

func (m *mockStore) ListGlobalEnv(ctx context.Context) ([]store.ConfigEnv, error) {
	m.listGlobalEnvCalled = true
	return m.listGlobalEnvResult, m.listGlobalEnvErr
}

func (m *mockStore) GetGlobalEnv(ctx context.Context, key string) (store.ConfigEnv, error) {
	m.getGlobalEnvCalled = true
	m.getGlobalEnvParam = key
	return m.getGlobalEnvResult, m.getGlobalEnvErr
}

func (m *mockStore) UpsertGlobalEnv(ctx context.Context, params store.UpsertGlobalEnvParams) error {
	m.upsertGlobalEnvCalled = true
	m.upsertGlobalEnvParams = params
	return m.upsertGlobalEnvErr
}

func (m *mockStore) DeleteGlobalEnv(ctx context.Context, key string) error {
	m.deleteGlobalEnvCalled = true
	m.deleteGlobalEnvParam = key
	return m.deleteGlobalEnvErr
}

// GetMigRepoByURL returns a mod_repo by mig_id and repo_url.
func (m *mockStore) GetMigRepoByURL(ctx context.Context, arg store.GetMigRepoByURLParams) (store.MigRepo, error) {
	m.getModRepoByURLCalled = true
	m.getModRepoByURLParams = arg
	return m.getModRepoByURLResult, m.getModRepoByURLErr
}

// UpsertMigRepo upserts a mod_repo by mig_id and repo_url.
func (m *mockStore) UpsertMigRepo(ctx context.Context, arg store.UpsertMigRepoParams) (store.MigRepo, error) {
	m.upsertModRepoCalled = true
	m.upsertModRepoParams = arg
	result := m.upsertModRepoResult
	if result.ID.IsZero() {
		result.ID = arg.ID
	}
	if result.MigID.IsZero() {
		result.MigID = arg.MigID
	}
	if result.RepoUrl == "" {
		result.RepoUrl = arg.RepoUrl
	}
	if result.BaseRef == "" {
		result.BaseRef = arg.BaseRef
	}
	if result.TargetRef == "" {
		result.TargetRef = arg.TargetRef
	}
	return result, m.upsertModRepoErr
}

// DeleteMigRepo deletes a mod_repo by id.
func (m *mockStore) DeleteMigRepo(ctx context.Context, id types.MigRepoID) error {
	m.deleteMigRepoCalled = true
	m.deleteMigRepoParam = id.String()
	return m.deleteMigRepoErr
}

// HasMigRepoHistory checks if a mod_repo has any historical executions.
func (m *mockStore) HasMigRepoHistory(ctx context.Context, repoID types.MigRepoID) (bool, error) {
	m.hasModRepoHistoryCalled = true
	m.hasModRepoHistoryParam = repoID.String()
	return m.hasModRepoHistoryResult, m.hasModRepoHistoryErr
}

// ListFailedRepoIDsByMig returns repo IDs whose last terminal status is 'Fail'.
func (m *mockStore) ListFailedRepoIDsByMig(ctx context.Context, modID types.MigID) ([]types.MigRepoID, error) {
	m.listFailedRepoIDsByModCalled = true
	m.listFailedRepoIDsByModParam = modID.String()
	return m.listFailedRepoIDsByModResult, m.listFailedRepoIDsByModErr
}

// ListRunReposWithURLByRun returns run repos with their repo_url for pull resolution.
func (m *mockStore) ListRunReposWithURLByRun(ctx context.Context, runID types.RunID) ([]store.ListRunReposWithURLByRunRow, error) {
	m.listRunReposWithURLByRunCalled = true
	m.listRunReposWithURLByRunParam = runID.String()
	return m.listRunReposWithURLByRunResult, m.listRunReposWithURLByRunErr
}

// GetLatestRunRepoByMigAndRepoStatus returns the latest run_repos row for a repo in a mig filtered by status.
func (m *mockStore) GetLatestRunRepoByMigAndRepoStatus(ctx context.Context, arg store.GetLatestRunRepoByMigAndRepoStatusParams) (store.GetLatestRunRepoByMigAndRepoStatusRow, error) {
	m.getLatestRunRepoByModAndRepoStatusCalled = true
	m.getLatestRunRepoByModAndRepoStatusParams = arg
	return m.getLatestRunRepoByModAndRepoStatusResult, m.getLatestRunRepoByModAndRepoStatusErr
}
