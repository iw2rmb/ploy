package handlers

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// jobStore is a focused mock for job completion, status, listing, claiming,
// stale recovery and related orchestration handler tests.
//
// Method receivers are split across companion files to keep each shard small:
//   - test_fixture_job_run_test.go      - Run, Artifact, and Diff methods.
//   - test_fixture_job_misc_test.go     - Stale recovery, Node, MigRepo, Event,
//     Ingest, and Spec/Mig/Run creation.
type jobStore struct {
	store.Store

	// Job queries
	getJob     mockCall[types.JobID, store.Job]
	getJobByID map[types.JobID]store.Job

	createJob mockCallSlice[store.CreateJobParams, store.Job]

	listJobsByRun mockCall[types.RunID, []store.Job]

	listJobsByRunAttempt mockCall[store.ListJobsByRunAttemptParams, []store.Job]

	// Job status/completion
	updateJobStatus mockCallSlice[store.UpdateJobStatusParams, struct{}]

	updateJobCompletion         mockCall[store.UpdateJobCompletionParams, struct{}]
	updateJobCompletionWithMeta mockCall[store.UpdateJobCompletionWithMetaParams, struct{}]
	updateJobMeta               mockCall[store.UpdateJobMetaParams, struct{}]
	updateJobRepoSHAIn          mockCall[store.UpdateJobRepoSHAInParams, struct{}]
	clearRepoSHAChainFromJob    mockCall[store.ClearRepoSHAChainFromJobParams, int64]
	updateJobImageName          mockCall[store.UpdateJobImageNameParams, struct{}]
	upsertJobMetric             mockCall[store.UpsertJobMetricParams, struct{}]

	updateJobNextID mockCallSlice[store.UpdateJobNextIDParams, struct{}]

	// Job scheduling/promotion
	scheduleNextJob           mockCall[store.ScheduleNextJobParams, store.Job]
	promoteJobByIDIfUnblocked mockCall[types.JobID, store.Job]

	// Job counts
	countJobsByRun                     mockResult[int64]
	countJobsByRunAndStatus            mockResult[int64]
	countJobsByRunAttemptGroupByStatus mockResult[[]store.CountJobsByRunAttemptGroupByStatusRow]

	// Job listing (TUI)
	listJobsForTUI  mockCall[store.ListJobsForTUIParams, []store.ListJobsForTUIRow]
	countJobsForTUI mockCall[*string, int64]

	// Claiming
	claimJob   mockCall[types.NodeID, store.Job]
	unclaimJob mockCall[store.UnclaimJobParams, struct{}]

	claimRun mockResult[store.Run]

	// SBOM
	deleteSBOMRowsByJob mockCallSlice[types.JobID, struct{}]

	upsertSBOMRow mockCallSlice[store.UpsertSBOMRowParams, struct{}]

	// Artifact/Diff (for job completion)
	createDiff              mockCall[store.CreateDiffParams, store.Diff]
	deleteDiff              mockCall[pgtype.UUID, struct{}]
	getLatestDiffByJob      mockCall[*types.JobID, store.Diff]
	getLatestDiffByJobByID  map[types.JobID]store.Diff
	getLatestDiffByJobError error
	createArtifactBundle    mockResult[store.ArtifactBundle]
	deleteArtifactBundle    mockCall[pgtype.UUID, struct{}]

	listArtifactBundlesByRunAndJob mockCall[store.ListArtifactBundlesByRunAndJobParams, []store.ArtifactBundle]

	// Run queries (for orchestration)
	getRun        mockCall[string, store.Run]
	getWave       mockCall[string, store.Wave]
	getSpec       mockCall[string, store.Spec]
	getSpecBundle mockCall[string, store.SpecBundle]
	getRunTiming  mockCall[string, store.RunsTiming]

	listRunsTimings mockResult[[]store.RunsTiming]

	ackRunStart         mockResult[struct{}]
	updateRunCompletion mockResult[struct{}]
	updateRunStatus     mockCallSlice[store.UpdateRunStatusParams, struct{}]
	updateWaveStatus    mockCallSlice[store.UpdateWaveStatusParams, struct{}]
	cancelRunV1         mockCall[string, struct{}]
	updateRunResume     mockResult[struct{}]

	// Run mutation and wave listing helpers.
	updateRunError      mockCall[store.UpdateRunErrorParams, struct{}]
	updateRunBaseRef    mockCall[store.UpdateRunBaseRefParams, struct{}]
	incrementRunAttempt mockCall[types.RunID, struct{}]

	listRunsByWave        mockCall[string, []store.Run]
	listQueuedRunsByWave  mockCall[string, []store.Run]
	listRunsWithURLByWave mockCall[string, []store.ListRunsWithURLByWaveRow]

	countRunsByStatus mockResult[[]store.CountRunsByWaveStatusRow]

	cancelActiveJobsByRunAttempt mockCallSlice[store.CancelActiveJobsByRunAttemptParams, int64]

	getLatestRunByMigAndRepoStatus mockCall[store.GetLatestRunByMigAndRepoStatusParams, store.GetLatestRunByMigAndRepoStatusRow]

	// Stale recovery
	listStaleRunningJobs           mockCall[pgtype.Timestamptz, []store.ListStaleRunningJobsRow]
	countStaleNodesWithRunningJobs mockResult[int64]

	// Node (for claim)
	getNode             mockCall[string, store.Node]
	updateNodeHeartbeat mockCall[store.UpdateNodeHeartbeatParams, struct{}]

	// Mig repo (for claim spec merge)
	getMigRepo              mockResult[store.MigRepo]
	listMigReposByMigResult []store.MigRepo

	// Event
	createEvent mockCall[store.CreateEventParams, store.Event]

	// Ingest (logs)
	createLog           mockResult[store.Log]
	listLogsByRun       mockCall[string, []store.Log]
	listLogsByRunAndJob mockCall[store.ListLogsByRunAndJobParams, []store.Log]

	// Spec creation (for migs_ticket flow)
	createSpec mockCall[store.CreateSpecParams, store.Spec]

	// Mig creation (for migs_ticket flow)
	createMig     mockCall[store.CreateMigParams, store.Mig]
	createMigRepo mockCall[store.CreateMigRepoParams, store.MigRepo]

	// Run creation (for migs_ticket flow)
	createWaveWithRuns mockCall[store.CreateWaveWithRunsParams, store.Wave]
	createRun          mockCall[store.CreateRunParams, store.Run]

	listRuns mockResult[[]store.Run]
}

// Job query methods

func (m *jobStore) GetJob(ctx context.Context, id types.JobID) (store.Job, error) {
	if len(m.getJobByID) > 0 {
		if result, ok := m.getJobByID[id]; ok {
			m.getJob.called = true
			m.getJob.params = id
			return result, m.getJob.err
		}
	}
	return m.getJob.record(id)
}

func (m *jobStore) CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error) {
	m.createJob.called = true
	m.createJob.calls = append(m.createJob.calls, params)
	result := buildCreateJobResult(m.createJob.val, params)
	return result, m.createJob.err
}

func (m *jobStore) ListJobsByRun(ctx context.Context, runID types.RunID) ([]store.Job, error) {
	m.listJobsByRun.called = true
	m.listJobsByRun.params = runID
	result := make([]store.Job, len(m.listJobsByRun.val))
	for i, j := range m.listJobsByRun.val {
		result[i] = j
		if m.updateJobCompletion.called && j.ID == m.updateJobCompletion.params.ID {
			result[i].Status = m.updateJobCompletion.params.Status
		}
	}
	return result, m.listJobsByRun.err
}

func (m *jobStore) ListJobsByRunAttempt(ctx context.Context, arg store.ListJobsByRunAttemptParams) ([]store.Job, error) {
	return m.listJobsByRunAttempt.record(arg)
}

// Job status/completion methods

func (m *jobStore) UpdateJobStatus(ctx context.Context, params store.UpdateJobStatusParams) error {
	_, err := m.updateJobStatus.record(params)
	return err
}

func (m *jobStore) UpdateJobCompletion(ctx context.Context, params store.UpdateJobCompletionParams) error {
	_, err := m.updateJobCompletion.record(params)
	return err
}

func (m *jobStore) UpdateJobCompletionWithMeta(ctx context.Context, params store.UpdateJobCompletionWithMetaParams) error {
	_, err := m.updateJobCompletionWithMeta.record(params)
	return err
}

func (m *jobStore) UpdateJobMeta(ctx context.Context, params store.UpdateJobMetaParams) error {
	_, err := m.updateJobMeta.record(params)
	return err
}

func (m *jobStore) UpdateJobRepoSHAIn(ctx context.Context, params store.UpdateJobRepoSHAInParams) error {
	_, err := m.updateJobRepoSHAIn.record(params)
	return err
}

func (m *jobStore) ClearRepoSHAChainFromJob(ctx context.Context, params store.ClearRepoSHAChainFromJobParams) (int64, error) {
	return m.clearRepoSHAChainFromJob.record(params)
}

func (m *jobStore) UpdateJobImageName(ctx context.Context, params store.UpdateJobImageNameParams) error {
	_, err := m.updateJobImageName.record(params)
	return err
}

func (m *jobStore) UpsertJobMetric(ctx context.Context, params store.UpsertJobMetricParams) error {
	_, err := m.upsertJobMetric.record(params)
	return err
}

func (m *jobStore) UpdateJobNextID(ctx context.Context, params store.UpdateJobNextIDParams) error {
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

func (m *jobStore) ScheduleNextJob(ctx context.Context, arg store.ScheduleNextJobParams) (store.Job, error) {
	m.scheduleNextJob.called = true
	m.scheduleNextJob.params = arg
	if m.scheduleNextJob.err != nil {
		return store.Job{}, m.scheduleNextJob.err
	}
	if m.scheduleNextJob.val.ID.IsZero() {
		return store.Job{}, pgx.ErrNoRows
	}
	return m.scheduleNextJob.val, nil
}

func (m *jobStore) PromoteJobByIDIfUnblocked(ctx context.Context, id types.JobID) (store.Job, error) {
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

func (m *jobStore) CountJobsByRun(ctx context.Context, runID types.RunID) (int64, error) {
	if m.countJobsByRun.err != nil {
		return 0, m.countJobsByRun.err
	}
	if m.countJobsByRun.val == 0 && len(m.listJobsByRun.val) > 0 {
		return int64(len(m.listJobsByRun.val)), nil
	}
	return m.countJobsByRun.val, nil
}

func (m *jobStore) CountJobsByRunAndStatus(ctx context.Context, arg store.CountJobsByRunAndStatusParams) (int64, error) {
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

func (m *jobStore) CountJobsByRunAttemptGroupByStatus(ctx context.Context, arg store.CountJobsByRunAttemptGroupByStatusParams) ([]store.CountJobsByRunAttemptGroupByStatusRow, error) {
	return m.countJobsByRunAttemptGroupByStatus.ret()
}

// Job listing methods

func (m *jobStore) ListJobsForTUI(ctx context.Context, arg store.ListJobsForTUIParams) ([]store.ListJobsForTUIRow, error) {
	return m.listJobsForTUI.record(arg)
}

func (m *jobStore) CountJobsForTUI(ctx context.Context, runID *string) (int64, error) {
	return m.countJobsForTUI.record(runID)
}

// Claim methods

func (m *jobStore) ClaimJob(ctx context.Context, nodeID types.NodeID) (store.Job, error) {
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

func (m *jobStore) UnclaimJob(ctx context.Context, arg store.UnclaimJobParams) error {
	_, err := m.unclaimJob.record(arg)
	return err
}

func (m *jobStore) ClaimRun(ctx context.Context, nodeID *string) (store.Run, error) {
	return m.claimRun.ret()
}

// SBOM methods

func (m *jobStore) DeleteSBOMRowsByJob(ctx context.Context, jobID types.JobID) error {
	_, err := m.deleteSBOMRowsByJob.record(jobID)
	return err
}

func (m *jobStore) UpsertSBOMRow(ctx context.Context, arg store.UpsertSBOMRowParams) error {
	_, err := m.upsertSBOMRow.record(arg)
	return err
}
