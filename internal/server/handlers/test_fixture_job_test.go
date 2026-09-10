package handlers

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// handlerStore is the canonical configurable store for handler tests.
type handlerStore struct {
	store.Store

	getJob                             mockCall[types.JobID, store.Job]
	getJobByID                         map[types.JobID]store.Job
	createJob                          mockCallSlice[store.CreateJobParams, store.Job]
	listJobsByRun                      mockCall[types.RunID, []store.Job]
	listJobsByRunAttempt               mockCall[store.ListJobsByRunAttemptParams, []store.Job]
	updateJobStatus                    mockCallSlice[store.UpdateJobStatusParams, struct{}]
	updateJobCompletion                mockCall[store.UpdateJobCompletionParams, struct{}]
	updateJobCompletionWithMeta        mockCall[store.UpdateJobCompletionWithMetaParams, struct{}]
	updateJobMeta                      mockCall[store.UpdateJobMetaParams, struct{}]
	updateJobRepoSHAIn                 mockCall[store.UpdateJobRepoSHAInParams, struct{}]
	clearRepoSHAChainFromJob           mockCall[store.ClearRepoSHAChainFromJobParams, int64]
	updateJobImageName                 mockCall[store.UpdateJobImageNameParams, struct{}]
	upsertJobMetric                    mockCall[store.UpsertJobMetricParams, struct{}]
	updateJobNextID                    mockCallSlice[store.UpdateJobNextIDParams, struct{}]
	promoteJobByIDIfUnblocked          mockCall[types.JobID, store.Job]
	countJobsByRun                     mockResult[int64]
	countJobsByRunAndStatus            mockResult[int64]
	countJobsByRunAttemptGroupByStatus mockResult[[]store.CountJobsByRunAttemptGroupByStatusRow]
	listJobsPage                       mockCall[store.ListJobsPageParams, []store.ListJobsPageRow]
	countJobsPage                      mockCall[store.CountJobsPageParams, int64]
	claimJob                           mockCall[types.NodeID, store.Job]
	unclaimJob                         mockCall[store.UnclaimJobParams, struct{}]
	claimRun                           mockResult[store.Run]

	deleteSBOMRowsByJob      mockCallSlice[types.JobID, struct{}]
	upsertSBOMRow            mockCallSlice[store.UpsertSBOMRowParams, struct{}]
	listRunSBOMRowsByJobType mockCallSlice[store.ListRunSBOMRowsByJobTypeParams, []store.ListRunSBOMRowsByJobTypeRow]
	sbomRowsByJobType        map[types.JobType][]store.ListRunSBOMRowsByJobTypeRow

	createDiff                     mockCall[store.CreateDiffParams, store.Diff]
	deleteDiff                     mockCall[pgtype.UUID, struct{}]
	getLatestDiffByJob             mockCall[*types.JobID, store.Diff]
	getLatestDiffByJobByID         map[types.JobID]store.Diff
	getLatestDiffByJobError        error
	createArtifactBundle           mockResult[store.ArtifactBundle]
	deleteArtifactBundle           mockCall[pgtype.UUID, struct{}]
	listArtifactBundlesByCID       mockResult[[]store.ArtifactBundle]
	listArtifactBundlesByRun       mockResult[[]store.ArtifactBundle]
	getArtifactBundle              mockResult[store.ArtifactBundle]
	listArtifactBundlesByRunAndJob mockCall[store.ListArtifactBundlesByRunAndJobParams, []store.ArtifactBundle]

	getRun                         mockCall[string, store.Run]
	getRunByID                     map[types.RunID]store.Run
	getRunSeq                      mockCallSeq[types.RunID, store.Run]
	getWave                        mockCall[string, store.Wave]
	getRunTiming                   mockCall[string, store.RunsTiming]
	listRunsTimings                mockResult[[]store.RunsTiming]
	listRuns                       mockResult[[]store.Run]
	listRunsWithMetadata           mockCall[store.ListRunsWithMetadataParams, []store.ListRunsWithMetadataRow]
	deleteRun                      mockCall[string, struct{}]
	cancelRun                      mockCall[string, struct{}]
	restartRun                     mockCall[store.RestartRunParams, store.Run]
	ackRunStart                    mockResult[struct{}]
	updateRunCompletion            mockResult[struct{}]
	updateRunStatus                mockCallSlice[store.UpdateRunStatusParams, struct{}]
	updateWaveStatus               mockCallSlice[store.UpdateWaveStatusParams, struct{}]
	cancelRunV1                    mockCall[string, struct{}]
	updateRunResume                mockResult[struct{}]
	updateRunError                 mockCall[store.UpdateRunErrorParams, struct{}]
	updateRunBaseRef               mockCall[store.UpdateRunBaseRefParams, struct{}]
	listRunsByWave                 mockCall[string, []store.Run]
	listRunsWithURLByWave          mockCall[string, []store.ListRunsWithURLByWaveRow]
	countRunsByStatus              mockResult[[]store.CountRunsByWaveStatusRow]
	cancelActiveJobsByRunAttempt   mockCallSlice[store.CancelActiveJobsByRunAttemptParams, int64]
	getLatestRunByMigAndRepoStatus mockCall[store.GetLatestRunByMigAndRepoStatusParams, store.GetLatestRunByMigAndRepoStatusRow]
	listStaleRunningJobs           mockCall[pgtype.Timestamptz, []store.ListStaleRunningJobsRow]
	countStaleNodesWithRunningJobs mockResult[int64]
	createRun                      mockCall[store.RunPlan, store.Run]
	createRunSeq                   mockCallSeq[store.RunPlan, store.Run]
	createRunParams                []store.RunPlan
	createRunCalled                bool
	createWaveWithRuns             mockCall[store.CreateWaveWithRunsParams, store.Wave]
	createWaveWithRunsHook         func(store.CreateWaveWithRunsParams)

	getNode             mockCall[string, store.Node]
	updateNodeDrained   mockCall[store.UpdateNodeDrainedParams, struct{}]
	listNodes           mockCall[struct{}, []store.Node]
	updateNodeHeartbeat mockCall[store.UpdateNodeHeartbeatParams, struct{}]
	updateCertMetadata  mockResult[struct{}]
	upsertDiagnostic    mockCall[store.UpsertNodeDiagnosticParams, store.NodeDiagnostic]
	listDiagnostics     mockCall[types.NodeID, []store.NodeDiagnostic]
	createDaemonLog     mockCallSlice[store.CreateNodeDaemonLogParams, store.NodeDaemonLog]
	listDaemonLogs      mockCall[store.ListNodeDaemonLogsParams, []store.NodeDaemonLog]
	trimDaemonLogs      mockCall[store.TrimNodeDaemonLogsParams, struct{}]

	getSpec               mockCall[string, store.Spec]
	createSpec            mockCall[store.CreateSpecParams, store.Spec]
	createGitSpecSnapshot mockCall[store.CreateGitSpecSnapshotParams, store.Spec]
	getGitSpec            mockCall[store.GetGitSpecSnapshotParams, store.Spec]
	getAPITokenByID       mockCall[string, store.GetAPITokenByIDRow]
	updateMigSpec         mockCall[store.UpdateMigSpecParams, struct{}]
	createSpecBundle      mockCall[store.CreateSpecBundleParams, store.SpecBundle]
	getSpecBundle         mockResult[store.SpecBundle]
	getSpecBundleByCID    mockResult[store.SpecBundle]
	deleteSpecBundle      mockResult[struct{}]

	getMig       mockCall[types.MigID, store.Mig]
	getMigByName mockCall[string, store.Mig]
	createMig    mockCall[store.CreateMigParams, store.Mig]
	listMigs     mockCall[store.ListMigsParams, []store.Mig]
	deleteMig    mockCall[string, struct{}]
	archiveMig   mockCall[string, struct{}]
	unarchiveMig mockCall[string, struct{}]

	createMigRepo            mockCall[store.CreateMigRepoParams, store.MigRepo]
	getMigRepo               mockResult[store.MigRepo]
	listMigReposByMig        mockCall[types.MigID, []store.MigRepo]
	listMigReposByMigResults map[string][]store.MigRepo
	getMigRepoByURL          mockCall[store.GetMigRepoByURLParams, store.MigRepo]
	upsertMigRepo            mockCall[store.UpsertMigRepoParams, store.MigRepo]
	deleteMigRepo            mockResult[struct{}]
	hasMigRepoHistory        mockResult[bool]
	listFailedRepoIDsByMig   mockCall[string, []types.RepoID]
	updateMigRepoBaseRef     mockCall[store.UpdateMigRepoBaseRefParams, struct{}]
	repoByID                 map[types.RepoID]store.Repo
	listDistinctRepos        mockCall[string, []store.ListDistinctReposRow]
	listRunsForRepo          mockCall[store.ListRunsForRepoParams, []store.ListRunsForRepoRow]

	createEvent         mockCall[store.CreateEventParams, store.Event]
	createLog           mockResult[store.Log]
	listLogsByRun       mockCall[string, []store.Log]
	listLogsByRunAndJob mockCall[store.ListLogsByRunAndJobParams, []store.Log]

	listGlobalEnv         mockResult[[]store.ConfigEnv]
	getGlobalEnv          mockResult[store.ConfigEnv]
	upsertGlobalEnv       mockCall[store.UpsertGlobalEnvParams, struct{}]
	deleteGlobalEnv       mockCall[store.DeleteGlobalEnvParams, struct{}]
	upsertConfigIn        mockCall[store.UpsertConfigInParams, struct{}]
	deleteConfigIn        mockCall[store.DeleteConfigInParams, struct{}]
	upsertConfigBundleMap mockCall[store.UpsertConfigBundleMapParams, struct{}]

	updateSpecBundleLastRefAtCalled  bool
	updateSpecBundleLastRefAtParam   string
	updateSpecBundleLastRefAtErr     error
	updateSpecBundleLastRefAtStarted chan struct{}
	updateSpecBundleLastRefAtProceed chan struct{}
	updateSpecBundleLastRefAtDone    chan struct{}
	updateSpecBundleLastRefAtCtxErr  error
}

// Job query methods

func (m *handlerStore) UpdateJobStatus(ctx context.Context, params store.UpdateJobStatusParams) error {
	_, err := m.updateJobStatus.record(params)
	return err
}

func (m *handlerStore) UpdateJobCompletion(ctx context.Context, params store.UpdateJobCompletionParams) error {
	_, err := m.updateJobCompletion.record(params)
	return err
}

func (m *handlerStore) UpdateJobCompletionWithMeta(ctx context.Context, params store.UpdateJobCompletionWithMetaParams) error {
	_, err := m.updateJobCompletionWithMeta.record(params)
	return err
}

func (m *handlerStore) UpdateJobMeta(ctx context.Context, params store.UpdateJobMetaParams) error {
	_, err := m.updateJobMeta.record(params)
	return err
}

func (m *handlerStore) UpdateJobRepoSHAIn(ctx context.Context, params store.UpdateJobRepoSHAInParams) error {
	_, err := m.updateJobRepoSHAIn.record(params)
	return err
}

func (m *handlerStore) ClearRepoSHAChainFromJob(ctx context.Context, params store.ClearRepoSHAChainFromJobParams) (int64, error) {
	return m.clearRepoSHAChainFromJob.record(params)
}

func (m *handlerStore) UpdateJobImageName(ctx context.Context, params store.UpdateJobImageNameParams) error {
	_, err := m.updateJobImageName.record(params)
	return err
}

func (m *handlerStore) UpsertJobMetric(ctx context.Context, params store.UpsertJobMetricParams) error {
	_, err := m.upsertJobMetric.record(params)
	return err
}

func (m *handlerStore) UpdateJobNextID(ctx context.Context, params store.UpdateJobNextIDParams) error {
	if _, err := m.updateJobNextID.record(params); err != nil {
		return err
	}
	for i := range m.listJobsByRunAttempt.val {
		if m.listJobsByRunAttempt.val[i].ID == params.ID {
			m.listJobsByRunAttempt.val[i].NextID = params.NextID
		}
	}
	for i := range m.listJobsByRun.val {
		if m.listJobsByRun.val[i].ID == params.ID {
			m.listJobsByRun.val[i].NextID = params.NextID
		}
	}
	return nil
}

// Job scheduling/promotion methods

func (m *handlerStore) PromoteJobByIDIfUnblocked(ctx context.Context, id types.JobID) (store.Job, error) {
	m.promoteJobByIDIfUnblocked.called = true
	m.promoteJobByIDIfUnblocked.params = id
	if m.promoteJobByIDIfUnblocked.err != nil {
		return store.Job{}, m.promoteJobByIDIfUnblocked.err
	}
	if !m.promoteJobByIDIfUnblocked.val.ID.IsZero() {
		return m.promoteJobByIDIfUnblocked.val, nil
	}
	for i := range m.listJobsByRunAttempt.val {
		if m.listJobsByRunAttempt.val[i].ID != id {
			continue
		}
		if m.listJobsByRunAttempt.val[i].Status != types.JobStatusCreated {
			return store.Job{}, pgx.ErrNoRows
		}
		m.listJobsByRunAttempt.val[i].Status = types.JobStatusQueued
		return m.listJobsByRunAttempt.val[i], nil
	}
	for i := range m.listJobsByRun.val {
		if m.listJobsByRun.val[i].ID != id {
			continue
		}
		if m.listJobsByRun.val[i].Status != types.JobStatusCreated {
			return store.Job{}, pgx.ErrNoRows
		}
		m.listJobsByRun.val[i].Status = types.JobStatusQueued
		return m.listJobsByRun.val[i], nil
	}
	return store.Job{}, pgx.ErrNoRows
}

// Job count methods

func (m *handlerStore) CountJobsByRun(ctx context.Context, runID types.RunID) (int64, error) {
	if m.countJobsByRun.err != nil {
		return 0, m.countJobsByRun.err
	}
	if m.countJobsByRun.val == 0 && len(m.listJobsByRun.val) > 0 {
		return int64(len(m.listJobsByRun.val)), nil
	}
	return m.countJobsByRun.val, nil
}

func (m *handlerStore) CountJobsByRunAndStatus(ctx context.Context, arg store.CountJobsByRunAndStatusParams) (int64, error) {
	if m.countJobsByRunAndStatus.err != nil {
		return 0, m.countJobsByRunAndStatus.err
	}
	if m.countJobsByRunAndStatus.val == 0 && len(m.listJobsByRun.val) > 0 {
		var count int64
		for _, j := range m.listJobsByRun.val {
			effectiveStatus := j.Status
			if m.updateJobCompletion.called && j.ID == m.updateJobCompletion.params.ID {
				effectiveStatus = m.updateJobCompletion.params.Status
			}
			if effectiveStatus == arg.Status {
				count++
			}
		}
		return count, nil
	}
	return m.countJobsByRunAndStatus.val, nil
}

func (m *handlerStore) CountJobsByRunAttemptGroupByStatus(ctx context.Context, arg store.CountJobsByRunAttemptGroupByStatusParams) ([]store.CountJobsByRunAttemptGroupByStatusRow, error) {
	return m.countJobsByRunAttemptGroupByStatus.ret()
}

// Job listing methods

func (m *handlerStore) ListJobsPage(ctx context.Context, arg store.ListJobsPageParams) ([]store.ListJobsPageRow, error) {
	return m.listJobsPage.record(arg)
}

func (m *handlerStore) CountJobsPage(ctx context.Context, arg store.CountJobsPageParams) (int64, error) {
	return m.countJobsPage.record(arg)
}

// Claim methods

func (m *handlerStore) ClaimJob(ctx context.Context, nodeID types.NodeID) (store.Job, error) {
	m.claimJob.called = true
	m.claimJob.params = nodeID
	if nodeID.IsZero() {
		return store.Job{}, store.ErrEmptyNodeID
	}
	if m.claimJob.err != nil {
		return store.Job{}, m.claimJob.err
	}
	if m.claimJob.val.ID.IsZero() {
		return store.Job{}, pgx.ErrNoRows
	}
	return m.claimJob.val, nil
}

func (m *handlerStore) UnclaimJob(ctx context.Context, arg store.UnclaimJobParams) error {
	_, err := m.unclaimJob.record(arg)
	return err
}

func (m *handlerStore) ClaimRun(ctx context.Context, nodeID *string) (store.Run, error) {
	return m.claimRun.ret()
}

// SBOM methods
