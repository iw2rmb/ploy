package handlers

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// jobStore is a focused mock for job completion, status, listing, claiming,
// healing, stale recovery, and related orchestration handler tests.
type jobStore struct {
	store.Store

	// Job queries
	getJobCalled  bool
	getJobParams  string
	getJobResult  store.Job
	getJobResults map[types.JobID]store.Job
	getJobErr     error

	createJob mockCallSlice[store.CreateJobParams, store.Job]

	listJobsByRunCalled bool
	listJobsByRunParam  string
	listJobsByRunResult []store.Job
	listJobsByRunErr    error

	listJobsByRunRepoAttempt mockCall[store.ListJobsByRunRepoAttemptParams, []store.Job]

	// Job status/completion
	updateJobStatus mockCallSlice[store.UpdateJobStatusParams, struct{}]

	updateJobCompletion         mockCall[store.UpdateJobCompletionParams, struct{}]
	updateJobCompletionWithMeta mockCall[store.UpdateJobCompletionWithMetaParams, struct{}]
	updateJobMeta               mockCall[store.UpdateJobMetaParams, struct{}]
	updateJobImageName          mockCall[store.UpdateJobImageNameParams, struct{}]
	upsertJobMetric             mockCall[store.UpsertJobMetricParams, struct{}]

	updateJobNextIDParams []store.UpdateJobNextIDParams
	updateJobNextIDErr    error

	// Job scheduling/promotion
	scheduleNextJob           mockCall[store.ScheduleNextJobParams, store.Job]
	promoteJobByIDIfUnblocked mockCall[types.JobID, store.Job]

	promoteReGateRecoveryCandidateGateProfileResult types.RepoID
	promoteReGateRecoveryCandidateGateProfileErr    error

	// Job counts
	countJobsByRunResult                   int64
	countJobsByRunErr                      error
	countJobsByRunAndStatusResult          int64
	countJobsByRunAndStatusErr             error
	countJobsByRunRepoAttemptGroupByStatus mockResult[[]store.CountJobsByRunRepoAttemptGroupByStatusRow]

	// Job listing (TUI)
	listJobsForTUI  mockCall[store.ListJobsForTUIParams, []store.ListJobsForTUIRow]
	countJobsForTUI mockCall[*string, int64]

	// Claiming
	claimJob   mockCall[types.NodeID, store.Job]
	unclaimJob mockCall[store.UnclaimJobParams, struct{}]

	claimRun mockResult[store.Run]

	// SBOM
	listSBOMRowsByJob       mockResult[[]store.Sbom]
	hasSBOMEvidenceForStack mockResult[bool]
	deleteSBOMRowsByJob     mockCallSlice[types.JobID, struct{}]

	upsertSBOMRow         mockCallSlice[store.UpsertSBOMRowParams, struct{}]
	hasHookOnceLedger     mockCall[store.HasHookOnceLedgerParams, bool]
	getHookOnceLedger     mockCall[store.GetHookOnceLedgerParams, store.HooksOnce]
	upsertHookOnceSuccess mockCall[store.UpsertHookOnceSuccessParams, struct{}]
	markHookOnceSkipped   mockCall[store.MarkHookOnceSkippedParams, struct{}]

	// Stack/Gate profile resolution
	resolveStackRowByImage           mockResult[store.ResolveStackRowByImageRow]
	resolveStackRowByLangTool        mockResult[store.ResolveStackRowByLangToolRow]
	resolveStackRowByLangToolRelease mockResult[store.ResolveStackRowByLangToolReleaseRow]

	upsertExactGateProfile mockCall[store.UpsertExactGateProfileParams, store.UpsertExactGateProfileRow]

	upsertGateJobProfileLink mockCall[store.UpsertGateJobProfileLinkParams, struct{}]

	// Artifact/Diff (for job completion)
	createDiff           mockCall[store.CreateDiffParams, store.Diff]
	createArtifactBundle mockResult[store.ArtifactBundle]

	listArtifactBundlesByRunAndJob mockResult[[]store.ArtifactBundle]

	// Run queries (for orchestration)
	getRun        mockCall[string, store.Run]
	getSpec       mockCall[string, store.Spec]
	getSpecBundle mockCall[string, store.SpecBundle]
	getRunTiming  mockCall[string, store.RunsTiming]

	listRunsTimings mockResult[[]store.RunsTiming]

	ackRunStart         mockResult[struct{}]
	updateRunCompletion mockResult[struct{}]
	updateRunStatus     mockCall[store.UpdateRunStatusParams, struct{}]
	cancelRunV1         mockCall[string, struct{}]
	updateRunResume     mockResult[struct{}]
	updateRunStatsMRURL mockCall[store.UpdateRunStatsMRURLParams, struct{}]

	// Run repo (for orchestration)
	getRunRepoCalled  bool
	getRunRepoParam   store.GetRunRepoParams
	getRunRepoResult  store.RunRepo
	getRunRepoResults []store.RunRepo
	getRunRepoCalls   int
	getRunRepoErr     error

	updateRunRepoStatus mockCallSlice[store.UpdateRunRepoStatusParams, struct{}]

	updateRunRepoError      mockCall[store.UpdateRunRepoErrorParams, struct{}]
	updateRunRepoRefs       mockCall[store.UpdateRunRepoRefsParams, struct{}]
	incrementRunRepoAttempt mockCall[store.IncrementRunRepoAttemptParams, struct{}]

	createRunRepoCalled bool
	createRunRepoParams store.CreateRunRepoParams
	createRunRepoResult store.RunRepo
	createRunRepoErr    error

	listRunReposByRun        mockCall[string, []store.RunRepo]
	listQueuedRunReposByRun  mockCall[string, []store.RunRepo]
	listRunReposWithURLByRun mockCall[string, []store.ListRunReposWithURLByRunRow]

	countRunReposByStatus mockResult[[]store.CountRunReposByStatusRow]

	cancelActiveJobsByRunRepoAttempt mockCallSlice[store.CancelActiveJobsByRunRepoAttemptParams, int64]

	getLatestRunRepoByMigAndRepoStatus mockCall[store.GetLatestRunRepoByMigAndRepoStatusParams, store.GetLatestRunRepoByMigAndRepoStatusRow]

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
	createEvent mockResult[store.Event]

	// Ingest (logs)
	createLog     mockResult[store.Log]
	listLogsByRun mockCall[string, []store.Log]

	// Spec creation (for migs_ticket flow)
	createSpecCalled bool
	createSpecParams store.CreateSpecParams
	createSpecErr    error

	// Mig creation (for migs_ticket flow)
	createMigCalled     bool
	createMigParams     store.CreateMigParams
	createMigRepoCalled bool

	// Run creation (for migs_ticket flow)
	createRunCalled bool
	createRunParams store.CreateRunParams
	createRunResult store.Run

	listRunsResult []store.Run
}

// Job query methods

func (m *jobStore) GetJob(ctx context.Context, id types.JobID) (store.Job, error) {
	m.getJobCalled = true
	m.getJobParams = id.String()
	if len(m.getJobResults) > 0 {
		if result, ok := m.getJobResults[id]; ok {
			return result, m.getJobErr
		}
	}
	return m.getJobResult, m.getJobErr
}

func (m *jobStore) CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error) {
	m.createJob.called = true
	m.createJob.calls = append(m.createJob.calls, params)
	result := m.createJob.val
	if result.ID.IsZero() {
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
	result.RepoShaIn = params.RepoShaIn
	result.Meta = params.Meta
	return result, m.createJob.err
}

func (m *jobStore) ListJobsByRun(ctx context.Context, runID types.RunID) ([]store.Job, error) {
	m.listJobsByRunCalled = true
	m.listJobsByRunParam = runID.String()
	result := make([]store.Job, len(m.listJobsByRunResult))
	for i, j := range m.listJobsByRunResult {
		result[i] = j
		if m.updateJobCompletion.called && j.ID == m.updateJobCompletion.params.ID {
			result[i].Status = m.updateJobCompletion.params.Status
		}
	}
	return result, m.listJobsByRunErr
}

func (m *jobStore) ListJobsByRunRepoAttempt(ctx context.Context, arg store.ListJobsByRunRepoAttemptParams) ([]store.Job, error) {
	return m.listJobsByRunRepoAttempt.record(arg)
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

func (m *jobStore) UpdateJobImageName(ctx context.Context, params store.UpdateJobImageNameParams) error {
	_, err := m.updateJobImageName.record(params)
	return err
}

func (m *jobStore) UpsertJobMetric(ctx context.Context, params store.UpsertJobMetricParams) error {
	_, err := m.upsertJobMetric.record(params)
	return err
}

func (m *jobStore) UpdateJobNextID(ctx context.Context, params store.UpdateJobNextIDParams) error {
	m.updateJobNextIDParams = append(m.updateJobNextIDParams, params)
	if m.updateJobNextIDErr != nil {
		return m.updateJobNextIDErr
	}
	for i := range m.listJobsByRunRepoAttempt.val {
		if m.listJobsByRunRepoAttempt.val[i].ID == params.ID {
			m.listJobsByRunRepoAttempt.val[i].NextID = params.NextID
		}
	}
	for i := range m.listJobsByRunResult {
		if m.listJobsByRunResult[i].ID == params.ID {
			m.listJobsByRunResult[i].NextID = params.NextID
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
	for i := range m.listJobsByRunRepoAttempt.val {
		if m.listJobsByRunRepoAttempt.val[i].ID != id {
			continue
		}
		if m.listJobsByRunRepoAttempt.val[i].Status != types.JobStatusCreated {
			return store.Job{}, pgx.ErrNoRows
		}
		m.listJobsByRunRepoAttempt.val[i].Status = types.JobStatusQueued
		return m.listJobsByRunRepoAttempt.val[i], nil
	}
	for i := range m.listJobsByRunResult {
		if m.listJobsByRunResult[i].ID != id {
			continue
		}
		if m.listJobsByRunResult[i].Status != types.JobStatusCreated {
			return store.Job{}, pgx.ErrNoRows
		}
		m.listJobsByRunResult[i].Status = types.JobStatusQueued
		return m.listJobsByRunResult[i], nil
	}
	return store.Job{}, pgx.ErrNoRows
}

func (m *jobStore) PromoteReGateRecoveryCandidateGateProfile(ctx context.Context, arg store.PromoteReGateRecoveryCandidateGateProfileParams) (types.RepoID, error) {
	if m.promoteReGateRecoveryCandidateGateProfileErr != nil {
		return "", m.promoteReGateRecoveryCandidateGateProfileErr
	}
	if !m.promoteReGateRecoveryCandidateGateProfileResult.IsZero() {
		return m.promoteReGateRecoveryCandidateGateProfileResult, nil
	}
	if !m.getJobResult.RepoID.IsZero() {
		return m.getJobResult.RepoID, nil
	}
	return "", pgx.ErrNoRows
}

// Job count methods

func (m *jobStore) CountJobsByRun(ctx context.Context, runID types.RunID) (int64, error) {
	if m.countJobsByRunErr != nil {
		return 0, m.countJobsByRunErr
	}
	if m.countJobsByRunResult == 0 && len(m.listJobsByRunResult) > 0 {
		return int64(len(m.listJobsByRunResult)), nil
	}
	return m.countJobsByRunResult, nil
}

func (m *jobStore) CountJobsByRunAndStatus(ctx context.Context, arg store.CountJobsByRunAndStatusParams) (int64, error) {
	if m.countJobsByRunAndStatusErr != nil {
		return 0, m.countJobsByRunAndStatusErr
	}
	if m.countJobsByRunAndStatusResult == 0 && len(m.listJobsByRunResult) > 0 {
		var count int64
		for _, j := range m.listJobsByRunResult {
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
	return m.countJobsByRunAndStatusResult, nil
}

func (m *jobStore) CountJobsByRunRepoAttemptGroupByStatus(ctx context.Context, arg store.CountJobsByRunRepoAttemptGroupByStatusParams) ([]store.CountJobsByRunRepoAttemptGroupByStatusRow, error) {
	return m.countJobsByRunRepoAttemptGroupByStatus.ret()
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

func (m *jobStore) ListSBOMRowsByJob(ctx context.Context, jobID types.JobID) ([]store.Sbom, error) {
	return m.listSBOMRowsByJob.ret()
}

func (m *jobStore) HasSBOMEvidenceForStack(ctx context.Context, arg store.HasSBOMEvidenceForStackParams) (bool, error) {
	return m.hasSBOMEvidenceForStack.ret()
}

func (m *jobStore) DeleteSBOMRowsByJob(ctx context.Context, jobID types.JobID) error {
	_, err := m.deleteSBOMRowsByJob.record(jobID)
	return err
}

func (m *jobStore) UpsertSBOMRow(ctx context.Context, arg store.UpsertSBOMRowParams) error {
	_, err := m.upsertSBOMRow.record(arg)
	return err
}

func (m *jobStore) HasHookOnceLedger(ctx context.Context, arg store.HasHookOnceLedgerParams) (bool, error) {
	return m.hasHookOnceLedger.record(arg)
}

func (m *jobStore) GetHookOnceLedger(ctx context.Context, arg store.GetHookOnceLedgerParams) (store.HooksOnce, error) {
	return m.getHookOnceLedger.record(arg)
}

func (m *jobStore) UpsertHookOnceSuccess(ctx context.Context, arg store.UpsertHookOnceSuccessParams) error {
	_, err := m.upsertHookOnceSuccess.record(arg)
	return err
}

func (m *jobStore) MarkHookOnceSkipped(ctx context.Context, arg store.MarkHookOnceSkippedParams) error {
	_, err := m.markHookOnceSkipped.record(arg)
	return err
}

// Stack/Gate profile methods

func (m *jobStore) ResolveStackRowByImage(ctx context.Context, image string) (store.ResolveStackRowByImageRow, error) {
	return resolveOrNoRows(&m.resolveStackRowByImage, func(r store.ResolveStackRowByImageRow) int64 { return r.ID })
}

func (m *jobStore) ResolveStackRowByLangTool(ctx context.Context, arg store.ResolveStackRowByLangToolParams) (store.ResolveStackRowByLangToolRow, error) {
	return resolveOrNoRows(&m.resolveStackRowByLangTool, func(r store.ResolveStackRowByLangToolRow) int64 { return r.ID })
}

func (m *jobStore) ResolveStackRowByLangToolRelease(ctx context.Context, arg store.ResolveStackRowByLangToolReleaseParams) (store.ResolveStackRowByLangToolReleaseRow, error) {
	return resolveOrNoRows(&m.resolveStackRowByLangToolRelease, func(r store.ResolveStackRowByLangToolReleaseRow) int64 { return r.ID })
}

func (m *jobStore) UpsertExactGateProfile(ctx context.Context, arg store.UpsertExactGateProfileParams) (store.UpsertExactGateProfileRow, error) {
	m.upsertExactGateProfile.called = true
	m.upsertExactGateProfile.params = arg
	if m.upsertExactGateProfile.err != nil {
		return store.UpsertExactGateProfileRow{}, m.upsertExactGateProfile.err
	}
	if m.upsertExactGateProfile.val.ID != 0 {
		return m.upsertExactGateProfile.val, nil
	}
	return store.UpsertExactGateProfileRow{
		ID:       1,
		RepoID:   arg.RepoID,
		RepoSha:  arg.RepoSha,
		RepoSha8: "",
		StackID:  arg.StackID,
		Url:      arg.Url,
	}, nil
}

func (m *jobStore) UpsertGateJobProfileLink(ctx context.Context, arg store.UpsertGateJobProfileLinkParams) error {
	_, err := m.upsertGateJobProfileLink.record(arg)
	return err
}

// Artifact/Diff methods

func (m *jobStore) CreateDiff(ctx context.Context, params store.CreateDiffParams) (store.Diff, error) {
	return m.createDiff.record(params)
}

func (m *jobStore) CreateArtifactBundle(ctx context.Context, params store.CreateArtifactBundleParams) (store.ArtifactBundle, error) {
	return m.createArtifactBundle.ret()
}

func (m *jobStore) ListArtifactBundlesByRunAndJob(ctx context.Context, arg store.ListArtifactBundlesByRunAndJobParams) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesByRunAndJob.ret()
}

// Run query methods

func (m *jobStore) GetRun(ctx context.Context, id types.RunID) (store.Run, error) {
	return m.getRun.record(id.String())
}

func (m *jobStore) GetSpec(ctx context.Context, id types.SpecID) (store.Spec, error) {
	return m.getSpec.record(id.String())
}

func (m *jobStore) GetSpecBundle(ctx context.Context, id string) (store.SpecBundle, error) {
	bundle, err := m.getSpecBundle.record(id)
	if err != nil {
		return store.SpecBundle{}, err
	}
	if bundle.ID == "" && bundle.ObjectKey == nil {
		return store.SpecBundle{}, errors.New("mock GetSpecBundle not configured")
	}
	return bundle, nil
}

func (m *jobStore) GetRunTiming(ctx context.Context, id types.RunID) (store.RunsTiming, error) {
	return m.getRunTiming.record(id.String())
}

func (m *jobStore) ListRunsTimings(ctx context.Context, arg store.ListRunsTimingsParams) ([]store.RunsTiming, error) {
	return m.listRunsTimings.ret()
}

func (m *jobStore) AckRunStart(ctx context.Context, id string) error {
	return m.ackRunStart.err
}

func (m *jobStore) UpdateRunCompletion(ctx context.Context, params store.UpdateRunCompletionParams) error {
	return m.updateRunCompletion.err
}

func (m *jobStore) UpdateRunStatus(ctx context.Context, params store.UpdateRunStatusParams) error {
	_, err := m.updateRunStatus.record(params)
	return err
}

func (m *jobStore) CancelRunV1(ctx context.Context, runID types.RunID) error {
	_, err := m.cancelRunV1.record(runID.String())
	return err
}

func (m *jobStore) UpdateRunResume(ctx context.Context, id types.RunID) error {
	return m.updateRunResume.err
}

func (m *jobStore) UpdateRunStatsMRURL(ctx context.Context, params store.UpdateRunStatsMRURLParams) error {
	_, err := m.updateRunStatsMRURL.record(params)
	return err
}

// Run repo methods

func (m *jobStore) GetRunRepo(ctx context.Context, arg store.GetRunRepoParams) (store.RunRepo, error) {
	m.getRunRepoCalled = true
	m.getRunRepoParam = arg
	if m.getRunRepoErr != nil {
		return store.RunRepo{}, m.getRunRepoErr
	}
	if len(m.getRunRepoResults) > 0 {
		idx := m.getRunRepoCalls
		if idx >= len(m.getRunRepoResults) {
			idx = len(m.getRunRepoResults) - 1
		}
		m.getRunRepoCalls++
		return m.getRunRepoResults[idx], nil
	}
	return m.getRunRepoResult, nil
}

func (m *jobStore) UpdateRunRepoStatus(ctx context.Context, params store.UpdateRunRepoStatusParams) error {
	_, err := m.updateRunRepoStatus.record(params)
	return err
}

func (m *jobStore) UpdateRunRepoError(ctx context.Context, params store.UpdateRunRepoErrorParams) error {
	_, err := m.updateRunRepoError.record(params)
	return err
}

func (m *jobStore) UpdateRunRepoRefs(ctx context.Context, params store.UpdateRunRepoRefsParams) error {
	_, err := m.updateRunRepoRefs.record(params)
	return err
}

func (m *jobStore) IncrementRunRepoAttempt(ctx context.Context, arg store.IncrementRunRepoAttemptParams) error {
	_, err := m.incrementRunRepoAttempt.record(arg)
	return err
}

func (m *jobStore) CreateRunRepo(ctx context.Context, params store.CreateRunRepoParams) (store.RunRepo, error) {
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
		result.Status = types.RunRepoStatusQueued
	}
	if result.Attempt == 0 {
		result.Attempt = 1
	}
	return result, m.createRunRepoErr
}

func (m *jobStore) ListRunReposByRun(ctx context.Context, runID types.RunID) ([]store.RunRepo, error) {
	return m.listRunReposByRun.record(runID.String())
}

func (m *jobStore) ListQueuedRunReposByRun(ctx context.Context, runID types.RunID) ([]store.RunRepo, error) {
	return m.listQueuedRunReposByRun.record(runID.String())
}

func (m *jobStore) ListRunReposWithURLByRun(ctx context.Context, runID types.RunID) ([]store.ListRunReposWithURLByRunRow, error) {
	return m.listRunReposWithURLByRun.record(runID.String())
}

func (m *jobStore) CountRunReposByStatus(ctx context.Context, runID types.RunID) ([]store.CountRunReposByStatusRow, error) {
	return m.countRunReposByStatus.ret()
}

func (m *jobStore) CancelActiveJobsByRunRepoAttempt(ctx context.Context, params store.CancelActiveJobsByRunRepoAttemptParams) (int64, error) {
	return m.cancelActiveJobsByRunRepoAttempt.record(params)
}

func (m *jobStore) GetLatestRunRepoByMigAndRepoStatus(ctx context.Context, arg store.GetLatestRunRepoByMigAndRepoStatusParams) (store.GetLatestRunRepoByMigAndRepoStatusRow, error) {
	return m.getLatestRunRepoByMigAndRepoStatus.record(arg)
}

// Stale recovery methods

func (m *jobStore) ListStaleRunningJobs(ctx context.Context, lastHeartbeat pgtype.Timestamptz) ([]store.ListStaleRunningJobsRow, error) {
	return m.listStaleRunningJobs.record(lastHeartbeat)
}

func (m *jobStore) CountStaleNodesWithRunningJobs(ctx context.Context, lastHeartbeat pgtype.Timestamptz) (int64, error) {
	return m.countStaleNodesWithRunningJobs.ret()
}

// Node methods (for claim)

func (m *jobStore) GetNode(ctx context.Context, id types.NodeID) (store.Node, error) {
	return m.getNode.record(id.String())
}

func (m *jobStore) UpdateNodeHeartbeat(ctx context.Context, params store.UpdateNodeHeartbeatParams) error {
	_, err := m.updateNodeHeartbeat.record(params)
	return err
}

// Mig repo methods (for claim spec merge)

func (m *jobStore) GetMigRepo(ctx context.Context, id types.MigRepoID) (store.MigRepo, error) {
	return m.getMigRepo.ret()
}

func (m *jobStore) ListMigReposByMig(ctx context.Context, migID types.MigID) ([]store.MigRepo, error) {
	return m.listMigReposByMigResult, nil
}

// Event methods

func (m *jobStore) CreateEvent(ctx context.Context, params store.CreateEventParams) (store.Event, error) {
	return m.createEvent.ret()
}

// Ingest methods

func (m *jobStore) CreateLog(ctx context.Context, params store.CreateLogParams) (store.Log, error) {
	return m.createLog.ret()
}

func (m *jobStore) ListLogsByRun(ctx context.Context, runID types.RunID) ([]store.Log, error) {
	return m.listLogsByRun.record(runID.String())
}

// Spec creation (for migs_ticket flow)

func (m *jobStore) CreateSpec(ctx context.Context, params store.CreateSpecParams) (store.Spec, error) {
	m.createSpecCalled = true
	m.createSpecParams = params
	result := store.Spec{ID: params.ID, Spec: params.Spec, CreatedBy: params.CreatedBy}
	return result, m.createSpecErr
}

func (m *jobStore) CreateMig(ctx context.Context, params store.CreateMigParams) (store.Mig, error) {
	m.createMigCalled = true
	m.createMigParams = params
	return store.Mig{ID: params.ID, Name: params.Name, SpecID: params.SpecID, CreatedBy: params.CreatedBy}, nil
}

func (m *jobStore) CreateMigRepo(ctx context.Context, params store.CreateMigRepoParams) (store.MigRepo, error) {
	m.createMigRepoCalled = true
	return store.MigRepo{ID: params.ID, MigID: params.MigID, RepoID: types.NewRepoID(), BaseRef: params.BaseRef, TargetRef: params.TargetRef}, nil
}

func (m *jobStore) GetRepo(ctx context.Context, id types.RepoID) (store.Repo, error) {
	if !id.IsZero() {
		return store.Repo{ID: id, Url: "https://github.com/user/repo.git"}, nil
	}
	return store.Repo{}, pgx.ErrNoRows
}

func (m *jobStore) CreateRun(ctx context.Context, params store.CreateRunParams) (store.Run, error) {
	m.createRunCalled = true
	m.createRunParams = params
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
	return result, nil
}

func (m *jobStore) ListRuns(ctx context.Context, params store.ListRunsParams) ([]store.Run, error) {
	if int(params.Offset) >= len(m.listRunsResult) {
		return []store.Run{}, nil
	}
	end := int(params.Offset) + int(params.Limit)
	if end > len(m.listRunsResult) {
		end = len(m.listRunsResult)
	}
	return m.listRunsResult[params.Offset:end], nil
}
