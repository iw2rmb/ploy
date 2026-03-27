package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// mockStore is a minimal Store implementation for testing handlers.
type mockStore struct {
	store.Store
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

	listRunsTimingsResult []store.RunsTiming
	listRunsTimingsErr    error

	deleteRunCalled bool
	deleteRunParams string
	deleteRunErr    error

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

	createEventResult store.Event
	createEventErr    error

	getJobCalled  bool
	getJobParams  string
	getJobResult  store.Job
	getJobResults map[types.JobID]store.Job
	getJobErr     error

	createDiffCalled bool
	createDiffParams store.CreateDiffParams
	createDiffResult store.Diff
	createDiffErr    error

	createArtifactBundleResult store.ArtifactBundle
	createArtifactBundleErr    error

	// AckRunStart tracking
	ackRunStartErr    error

	// UpdateRunCompletion tracking
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
	listArtifactBundlesByCIDResult []store.ArtifactBundle
	listArtifactBundlesByCIDErr    error

	// ListArtifactBundlesMetaByCID tracking
	listArtifactBundlesMetaByCIDResult []store.ArtifactBundle
	listArtifactBundlesMetaByCIDErr    error

	// ListArtifactBundlesByRun tracking
	listArtifactBundlesByRunResult []store.ArtifactBundle
	listArtifactBundlesByRunErr    error

	// ListArtifactBundlesMetaByRun tracking
	listArtifactBundlesMetaByRunCalled bool
	listArtifactBundlesMetaByRunParam  string
	listArtifactBundlesMetaByRunResult []store.ArtifactBundle
	listArtifactBundlesMetaByRunErr    error

	// GetArtifactBundle tracking
	getArtifactBundleResult store.ArtifactBundle
	getArtifactBundleErr    error

	// ListArtifactBundlesByRunAndJob tracking
	listArtifactBundlesByRunAndJobResult []store.ArtifactBundle
	listArtifactBundlesByRunAndJobErr    error

	// ListArtifactBundlesMetaByRunAndJob tracking
	listArtifactBundlesMetaByRunAndJobResult []store.ArtifactBundle
	listArtifactBundlesMetaByRunAndJobErr    error

	// SBOM row query tracking
	listSBOMRowsByJobResult []store.Sbom
	listSBOMRowsByJobErr    error

	hasSBOMEvidenceForStackResult bool
	hasSBOMEvidenceForStackErr    error

	listSBOMCompatRowsCalled bool
	listSBOMCompatRowsParams store.ListSBOMCompatRowsParams
	listSBOMCompatRowsResult []store.ListSBOMCompatRowsRow
	listSBOMCompatRowsErr    error

	// CreateJob tracking
	createJobCalled    bool
	createJobCallCount int
	createJobParams    []store.CreateJobParams
	createJobResult    store.Job
	createJobErr       error

	// ListJobsForTUI tracking (TUI global jobs listing)
	listJobsForTUICalled bool
	listJobsForTUIParams store.ListJobsForTUIParams
	listJobsForTUIResult []store.ListJobsForTUIRow
	listJobsForTUIErr    error

	// CountJobsForTUI tracking (TUI global jobs count)
	countJobsForTUICalled bool
	countJobsForTUIParam  *types.RunID
	countJobsForTUIResult int64
	countJobsForTUIErr    error

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
	countJobsByRunResult int64
	countJobsByRunErr    error

	// CountJobsByRunAndStatus tracking
	countJobsByRunAndStatusResult int64
	countJobsByRunAndStatusErr    error

	// CountJobsByRunRepoAttemptGroupByStatus tracking (v1 repo-scoped progression)
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

	// UpdateJobMeta tracking
	updateJobMetaCalled bool
	updateJobMetaParams store.UpdateJobMetaParams
	updateJobMetaErr    error

	// SBOM mutation tracking
	deleteSBOMRowsByJobCalled bool
	deleteSBOMRowsByJobParam  types.JobID
	deleteSBOMRowsByJobErr    error

	upsertSBOMRowCalled bool
	upsertSBOMRowParams []store.UpsertSBOMRowParams
	upsertSBOMRowErr    error

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
	promoteReGateRecoveryCandidateGateProfileResult types.RepoID
	promoteReGateRecoveryCandidateGateProfileErr    error

	resolveStackRowByImageResult store.ResolveStackRowByImageRow
	resolveStackRowByImageErr    error

	resolveStackRowByLangToolResult store.ResolveStackRowByLangToolRow
	resolveStackRowByLangToolErr    error

	resolveStackRowByLangToolReleaseResult store.ResolveStackRowByLangToolReleaseRow
	resolveStackRowByLangToolReleaseErr    error

	upsertExactGateProfileCalled bool
	upsertExactGateProfileParam  store.UpsertExactGateProfileParams
	upsertExactGateProfileResult store.UpsertExactGateProfileRow
	upsertExactGateProfileErr    error

	upsertGateJobProfileLinkCalled bool
	upsertGateJobProfileLinkParam  store.UpsertGateJobProfileLinkParams
	upsertGateJobProfileLinkErr    error

	// UpdateJobNextID tracking
	updateJobNextIDParams []store.UpdateJobNextIDParams
	updateJobNextIDErr    error

	// ListDiffsByRunRepo tracking (v1 repo-scoped diffs listing)
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
	createLogResult store.Log
	createLogErr    error

	// API Token tracking
	insertAPITokenErr    error

	listAPITokensResult []store.ListAPITokensRow
	listAPITokensErr    error

	revokeAPITokenErr    error

	checkAPITokenRevokedResult pgtype.Timestamptz
	checkAPITokenRevokedErr    error

	updateAPITokenLastUsedErr    error

	// Bootstrap Token tracking
	insertBootstrapTokenErr    error

	getBootstrapTokenResult store.GetBootstrapTokenRow
	getBootstrapTokenErr    error

	checkBootstrapTokenRevokedResult pgtype.Timestamptz
	checkBootstrapTokenRevokedErr    error

	updateBootstrapTokenLastUsedErr    error

	markBootstrapTokenUsedErr    error

	// ListRuns tracking (for batch run handlers)
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

	countStaleNodesWithRunningJobsResult int64
	countStaleNodesWithRunningJobsErr    error

	cancelActiveJobsByRunRepoAttemptCalled bool
	cancelActiveJobsByRunRepoAttemptParams []store.CancelActiveJobsByRunRepoAttemptParams
	cancelActiveJobsByRunRepoAttemptResult int64
	cancelActiveJobsByRunRepoAttemptErr    error

	// CountRunReposByStatus tracking — run IDs are now strings (KSUID).
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
	repoByID                 map[types.RepoID]store.Repo

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
	deleteMigRepoErr    error

	// HasMigRepoHistory tracking (for delete validation)
	hasModRepoHistoryResult bool
	hasModRepoHistoryErr    error

	// ListFailedRepoIDsByMig tracking (for "failed" repo selection)
	listFailedRepoIDsByModCalled bool
	listFailedRepoIDsByModParam  string
	listFailedRepoIDsByModResult []types.RepoID
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
	getRunRepoCalled  bool
	getRunRepoParam   store.GetRunRepoParams
	getRunRepoResult  store.RunRepo
	getRunRepoResults []store.RunRepo // successive calls return entries in order
	getRunRepoCalls   int
	getRunRepoErr     error

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

	// SpecBundle tracking
	createSpecBundleParams store.CreateSpecBundleParams
	createSpecBundleResult store.SpecBundle
	createSpecBundleErr    error

	getSpecBundleResult store.SpecBundle
	getSpecBundleErr    error

	getSpecBundleByCIDResult store.SpecBundle
	getSpecBundleByCIDErr    error

	updateSpecBundleLastRefAtCalled bool
	updateSpecBundleLastRefAtParam  string
	updateSpecBundleLastRefAtErr    error
	updateSpecBundleLastRefAtStarted chan struct{}
	updateSpecBundleLastRefAtProceed chan struct{}
	updateSpecBundleLastRefAtDone    chan struct{}
	updateSpecBundleLastRefAtCtxErr  error

	deleteSpecBundleErr    error

	// UpdateMigRepoRefs tracking
	updateMigRepoRefsCalled bool
	updateMigRepoRefsParams store.UpdateMigRepoRefsParams
	updateMigRepoRefsErr    error

	// Global Env tracking (config_env table; see docs/envs/README.md#Global Env Configuration)
	listGlobalEnvResult []store.ConfigEnv
	listGlobalEnvErr    error

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
	return m.updateCertMetadataErr
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
	return m.createEventResult, m.createEventErr
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
