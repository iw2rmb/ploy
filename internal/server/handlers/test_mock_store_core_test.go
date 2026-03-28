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
	updateCertMetadata mockResult[struct{}]

	// v1 migs/specs/mig_repos tracking (used by /v1/migs and /v1/runs handlers)
	createSpecCalled bool
	createSpecParams store.CreateSpecParams
	createSpecResult store.Spec
	createSpecErr    error

	updateModSpec mockCall[store.UpdateMigSpecParams, struct{}]

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

	deleteMig   mockCall[string, struct{}]
	archiveMig  mockCall[string, struct{}]
	unarchiveMig mockCall[string, struct{}]

	createMigRepoCalled bool
	createMigRepoParams store.CreateMigRepoParams
	createMigRepoResult store.MigRepo
	createMigRepoErr    error

	getModRepo mockResult[store.MigRepo]

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

	getRun       mockCall[string, store.Run]
	getSpec      mockCall[string, store.Spec]
	getRunTiming mockCall[string, store.RunsTiming]

	listRunsTimings mockResult[[]store.RunsTiming]

	deleteRun mockCall[string, struct{}]

	claimRun mockResult[store.Run]

	claimJobCalled bool
	claimJobParams types.NodeID
	claimJobResult store.Job
	claimJobErr    error

	getNode          mockCall[string, store.Node]
	updateNodeHeartbeat mockCall[store.UpdateNodeHeartbeatParams, struct{}]

	createEvent mockResult[store.Event]

	getJobCalled  bool
	getJobParams  string
	getJobResult  store.Job
	getJobResults map[types.JobID]store.Job
	getJobErr     error

	createDiff mockCall[store.CreateDiffParams, store.Diff]

	createArtifactBundle mockResult[store.ArtifactBundle]

	// AckRunStart tracking
	ackRunStart mockResult[struct{}]

	// UpdateRunCompletion tracking
	updateRunCompletion mockResult[struct{}]

	// UpdateRunStatus tracking
	updateRunStatus mockCall[store.UpdateRunStatusParams, struct{}]

	// CancelRunV1 tracking
	cancelRunV1 mockCall[string, struct{}]

	// UpdateRunResume tracking (resume_count, last_resumed_at)
	updateRunResume mockResult[struct{}]

	// UpdateRunStatsMRURL tracking (MR URL propagation)
	updateRunStatsMRURL mockCall[store.UpdateRunStatsMRURLParams, struct{}]

	// Node drain/undrain tracking
	updateNodeDrained mockCall[store.UpdateNodeDrainedParams, struct{}]

	// ListNodes tracking
	listNodes mockCall[struct{}, []store.Node]

	// Artifact bundle queries
	listArtifactBundlesByCID          mockResult[[]store.ArtifactBundle]
	listArtifactBundlesMetaByCID      mockResult[[]store.ArtifactBundle]
	listArtifactBundlesByRun          mockResult[[]store.ArtifactBundle]
	listArtifactBundlesMetaByRun      mockResult[[]store.ArtifactBundle]
	getArtifactBundle                 mockResult[store.ArtifactBundle]
	listArtifactBundlesByRunAndJob    mockResult[[]store.ArtifactBundle]
	listArtifactBundlesMetaByRunAndJob mockResult[[]store.ArtifactBundle]

	// SBOM row query tracking
	listSBOMRowsByJob          mockResult[[]store.Sbom]
	hasSBOMEvidenceForStack    mockResult[bool]
	listSBOMCompatRows         mockCall[store.ListSBOMCompatRowsParams, []store.ListSBOMCompatRowsRow]

	// CreateJob tracking
	createJobCalled    bool
	createJobCallCount int
	createJobParams    []store.CreateJobParams
	createJobResult    store.Job
	createJobErr       error

	// ListJobsForTUI tracking (TUI global jobs listing)
	listJobsForTUI mockCall[store.ListJobsForTUIParams, []store.ListJobsForTUIRow]

	// CountJobsForTUI tracking (TUI global jobs count)
	countJobsForTUI mockCall[*types.RunID, int64]

	// ListJobsByRun tracking (status overlay with UpdateJobCompletion)
	listJobsByRunCalled bool
	listJobsByRunParam  string
	listJobsByRunResult []store.Job
	listJobsByRunErr    error

	// ListJobsByRunRepoAttempt tracking (v1 repo-scoped jobs)
	listJobsByRunRepoAttempt mockCall[store.ListJobsByRunRepoAttemptParams, []store.Job]

	// CountJobsByRun tracking
	countJobsByRunResult int64
	countJobsByRunErr    error

	// CountJobsByRunAndStatus tracking
	countJobsByRunAndStatusResult int64
	countJobsByRunAndStatusErr    error

	// CountJobsByRunRepoAttemptGroupByStatus tracking (v1 repo-scoped progression)
	countJobsByRunRepoAttemptGroupByStatus mockResult[[]store.CountJobsByRunRepoAttemptGroupByStatusRow]

	// UpdateJobStatus tracking
	updateJobStatusCalled bool
	updateJobStatusParams store.UpdateJobStatusParams
	updateJobStatusCalls  []store.UpdateJobStatusParams
	updateJobStatusErr    error

	// UpdateJobCompletion tracking
	updateJobCompletion mockCall[store.UpdateJobCompletionParams, struct{}]

	// UpdateJobCompletionWithMeta tracking
	updateJobCompletionWithMeta mockCall[store.UpdateJobCompletionWithMetaParams, struct{}]

	// UpdateJobMeta tracking
	updateJobMeta mockCall[store.UpdateJobMetaParams, struct{}]

	// SBOM mutation tracking
	deleteSBOMRowsByJob mockCall[types.JobID, struct{}]

	upsertSBOMRowCalled bool
	upsertSBOMRowParams []store.UpsertSBOMRowParams
	upsertSBOMRowErr    error

	// UpsertJobMetric tracking
	upsertJobMetric mockCall[store.UpsertJobMetricParams, struct{}]

	// UpdateJobImageName tracking
	updateJobImageName mockCall[store.UpdateJobImageNameParams, struct{}]

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

	upsertGateJobProfileLink mockCall[store.UpsertGateJobProfileLinkParams, struct{}]

	// UpdateJobNextID tracking
	updateJobNextIDParams []store.UpdateJobNextIDParams
	updateJobNextIDErr    error

	// Diff tracking
	listDiffsByRunRepo mockCall[store.ListDiffsByRunRepoParams, []store.Diff]
	getDiff            mockCall[pgtype.UUID, store.Diff]

	// CreateLog tracking
	createLog mockResult[store.Log]

	// API Token tracking
	insertAPIToken         mockResult[struct{}]
	listAPITokens          mockResult[[]store.ListAPITokensRow]
	revokeAPIToken         mockResult[struct{}]
	checkAPITokenRevoked   mockResult[pgtype.Timestamptz]
	updateAPITokenLastUsed mockResult[struct{}]

	// Bootstrap Token tracking
	insertBootstrapToken         mockResult[struct{}]
	getBootstrapToken            mockResult[store.GetBootstrapTokenRow]
	checkBootstrapTokenRevoked   mockResult[pgtype.Timestamptz]
	updateBootstrapTokenLastUsed mockResult[struct{}]
	markBootstrapTokenUsed       mockResult[struct{}]

	// ListRuns tracking (for batch run handlers)
	listRunsResult []store.Run
	listRunsErr    error

	// ListRunReposByRun tracking — run IDs are now strings (KSUID).
	listRunReposByRun mockCall[string, []store.RunRepo]

	// Stale running jobs recovery tracking
	listStaleRunningJobs mockCall[pgtype.Timestamptz, []store.ListStaleRunningJobsRow]

	countStaleNodesWithRunningJobs mockResult[int64]

	cancelActiveJobsByRunRepoAttemptCalled bool
	cancelActiveJobsByRunRepoAttemptParams []store.CancelActiveJobsByRunRepoAttemptParams
	cancelActiveJobsByRunRepoAttemptResult int64
	cancelActiveJobsByRunRepoAttemptErr    error

	// CountRunReposByStatus — run IDs are now strings (KSUID).
	countRunReposByStatus mockResult[[]store.CountRunReposByStatusRow]

	// UpdateRunRepoRefs tracking
	updateRunRepoRefs mockCall[store.UpdateRunRepoRefsParams, struct{}]

	// UpdateRunRepoStatus tracking
	updateRunRepoStatusCalled bool
	updateRunRepoStatusParams []store.UpdateRunRepoStatusParams
	updateRunRepoStatusErr    error

	// UpdateRunRepoError tracking (for Stack Gate failures)
	updateRunRepoError mockCall[store.UpdateRunRepoErrorParams, struct{}]

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
	getModRepoByURL mockCall[store.GetMigRepoByURLParams, store.MigRepo]

	// UpsertMigRepo tracking (for bulk upsert)
	upsertModRepoCalled bool
	upsertModRepoParams store.UpsertMigRepoParams
	upsertModRepoResult store.MigRepo
	upsertModRepoErr    error

	// DeleteMigRepo tracking
	deleteMigRepo mockResult[struct{}]

	// HasMigRepoHistory tracking (for delete validation)
	hasModRepoHistory mockResult[bool]

	// ListFailedRepoIDsByMig tracking (for "failed" repo selection)
	listFailedRepoIDsByMod mockCall[string, []types.RepoID]

	// ListRunReposWithURLByRun tracking (for pull resolution)
	listRunReposWithURLByRun mockCall[string, []store.ListRunReposWithURLByRunRow]

	// GetLatestRunRepoByMigAndRepoStatus tracking (for mig pull resolution)
	getLatestRunRepoByModAndRepoStatus mockCall[store.GetLatestRunRepoByMigAndRepoStatusParams, store.GetLatestRunRepoByMigAndRepoStatusRow]

	// GetRunRepo tracking — composite key (run_id, repo_id).
	getRunRepoCalled  bool
	getRunRepoParam   store.GetRunRepoParams
	getRunRepoResult  store.RunRepo
	getRunRepoResults []store.RunRepo // successive calls return entries in order
	getRunRepoCalls   int
	getRunRepoErr     error

	// IncrementRunRepoAttempt tracking — composite key (run_id, repo_id).
	incrementRunRepoAttempt mockCall[store.IncrementRunRepoAttemptParams, struct{}]

	// ListQueuedRunReposByRun tracking — run IDs are now strings (KSUID).
	listQueuedRunReposByRun mockCall[string, []store.RunRepo]

	// ListDistinctRepos tracking (for repo-centric handlers)
	listDistinctRepos mockCall[string, []store.ListDistinctReposRow]

	// ListRunsForRepo tracking
	listRunsForRepo mockCall[store.ListRunsForRepoParams, []store.ListRunsForRepoRow]

	// SpecBundle tracking
	createSpecBundle mockCall[store.CreateSpecBundleParams, store.SpecBundle]

	getSpecBundle      mockResult[store.SpecBundle]
	getSpecBundleByCID mockResult[store.SpecBundle]

	updateSpecBundleLastRefAtCalled bool
	updateSpecBundleLastRefAtParam  string
	updateSpecBundleLastRefAtErr    error
	updateSpecBundleLastRefAtStarted chan struct{}
	updateSpecBundleLastRefAtProceed chan struct{}
	updateSpecBundleLastRefAtDone    chan struct{}
	updateSpecBundleLastRefAtCtxErr  error

	deleteSpecBundle mockResult[struct{}]

	// UpdateMigRepoRefs tracking
	updateMigRepoRefs mockCall[store.UpdateMigRepoRefsParams, struct{}]

	// Global Env tracking (config_env table; see docs/envs/README.md#Global Env Configuration)
	listGlobalEnv   mockResult[[]store.ConfigEnv]
	getGlobalEnv    mockResult[store.ConfigEnv]
	upsertGlobalEnv mockCall[store.UpsertGlobalEnvParams, struct{}]
	deleteGlobalEnv mockCall[string, struct{}]
}

func (m *mockStore) UpdateNodeCertMetadata(ctx context.Context, params store.UpdateNodeCertMetadataParams) error {
	return m.updateCertMetadata.err
}

func (m *mockStore) GetNode(ctx context.Context, id types.NodeID) (store.Node, error) {
	return m.getNode.record(id.String())
}

func (m *mockStore) UpdateNodeHeartbeat(ctx context.Context, params store.UpdateNodeHeartbeatParams) error {
	_, err := m.updateNodeHeartbeat.record(params)
	return err
}

func (m *mockStore) CreateEvent(ctx context.Context, params store.CreateEventParams) (store.Event, error) {
	return m.createEvent.ret()
}

func (m *mockStore) UpdateNodeDrained(ctx context.Context, params store.UpdateNodeDrainedParams) error {
	_, err := m.updateNodeDrained.record(params)
	return err
}

func (m *mockStore) ListNodes(ctx context.Context) ([]store.Node, error) {
	m.listNodes.called = true
	return m.listNodes.val, m.listNodes.err
}
