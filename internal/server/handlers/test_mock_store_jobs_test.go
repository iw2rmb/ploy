package handlers

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func (m *mockStore) ListJobsForTUI(ctx context.Context, arg store.ListJobsForTUIParams) ([]store.ListJobsForTUIRow, error) {
	return m.listJobsForTUI.record(arg)
}

func (m *mockStore) CountJobsForTUI(ctx context.Context, runID *types.RunID) (int64, error) {
	return m.countJobsForTUI.record(runID)
}

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

func (m *mockStore) GetJob(ctx context.Context, id types.JobID) (store.Job, error) {
	m.getJobCalled = true
	m.getJobParams = id.String()
	if len(m.getJobResults) > 0 {
		if result, ok := m.getJobResults[id]; ok {
			return result, m.getJobErr
		}
	}
	return m.getJobResult, m.getJobErr
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
	result.RepoShaIn = params.RepoShaIn
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
		if m.updateJobCompletion.called && j.ID == m.updateJobCompletion.params.ID {
			result[i].Status = m.updateJobCompletion.params.Status
		}
	}
	return result, m.listJobsByRunErr
}

func (m *mockStore) ListJobsByRunRepoAttempt(ctx context.Context, arg store.ListJobsByRunRepoAttemptParams) ([]store.Job, error) {
	return m.listJobsByRunRepoAttempt.record(arg)
}

func (m *mockStore) ListStaleRunningJobs(ctx context.Context, lastHeartbeat pgtype.Timestamptz) ([]store.ListStaleRunningJobsRow, error) {
	return m.listStaleRunningJobs.record(lastHeartbeat)
}

func (m *mockStore) CountStaleNodesWithRunningJobs(ctx context.Context, lastHeartbeat pgtype.Timestamptz) (int64, error) {
	return m.countStaleNodesWithRunningJobs.ret()
}

func (m *mockStore) CancelActiveJobsByRunRepoAttempt(ctx context.Context, params store.CancelActiveJobsByRunRepoAttemptParams) (int64, error) {
	m.cancelActiveJobsByRunRepoAttemptCalled = true
	m.cancelActiveJobsByRunRepoAttemptParams = append(m.cancelActiveJobsByRunRepoAttemptParams, params)
	return m.cancelActiveJobsByRunRepoAttemptResult, m.cancelActiveJobsByRunRepoAttemptErr
}

func (m *mockStore) CountJobsByRun(ctx context.Context, runID types.RunID) (int64, error) {
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
	if m.countJobsByRunAndStatusErr != nil {
		return 0, m.countJobsByRunAndStatusErr
	}
	// Default: count matching jobs from listJobsByRunResult, accounting for job completions.
	if m.countJobsByRunAndStatusResult == 0 && len(m.listJobsByRunResult) > 0 {
		var count int64
		for _, j := range m.listJobsByRunResult {
			// If this job was marked as completed via UpdateJobCompletion, use the completed status.
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

func (m *mockStore) CountJobsByRunRepoAttemptGroupByStatus(ctx context.Context, arg store.CountJobsByRunRepoAttemptGroupByStatusParams) ([]store.CountJobsByRunRepoAttemptGroupByStatusRow, error) {
	return m.countJobsByRunRepoAttemptGroupByStatus.ret()
}

func (m *mockStore) UpdateJobStatus(ctx context.Context, params store.UpdateJobStatusParams) error {
	m.updateJobStatusCalled = true
	m.updateJobStatusParams = params
	m.updateJobStatusCalls = append(m.updateJobStatusCalls, params)
	return m.updateJobStatusErr
}

func (m *mockStore) UpdateJobCompletion(ctx context.Context, params store.UpdateJobCompletionParams) error {
	_, err := m.updateJobCompletion.record(params)
	return err
}

func (m *mockStore) UpdateJobCompletionWithMeta(ctx context.Context, params store.UpdateJobCompletionWithMetaParams) error {
	_, err := m.updateJobCompletionWithMeta.record(params)
	return err
}

func (m *mockStore) UpdateJobMeta(ctx context.Context, params store.UpdateJobMetaParams) error {
	_, err := m.updateJobMeta.record(params)
	return err
}

func (m *mockStore) DeleteSBOMRowsByJob(ctx context.Context, jobID types.JobID) error {
	_, err := m.deleteSBOMRowsByJob.record(jobID)
	return err
}

func (m *mockStore) UpsertSBOMRow(ctx context.Context, arg store.UpsertSBOMRowParams) error {
	m.upsertSBOMRowCalled = true
	m.upsertSBOMRowParams = append(m.upsertSBOMRowParams, arg)
	return m.upsertSBOMRowErr
}

func (m *mockStore) UpsertJobMetric(ctx context.Context, params store.UpsertJobMetricParams) error {
	_, err := m.upsertJobMetric.record(params)
	return err
}

func (m *mockStore) UpdateJobImageName(ctx context.Context, params store.UpdateJobImageNameParams) error {
	_, err := m.updateJobImageName.record(params)
	return err
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

func (m *mockStore) PromoteReGateRecoveryCandidateGateProfile(ctx context.Context, arg store.PromoteReGateRecoveryCandidateGateProfileParams) (types.RepoID, error) {
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

func (m *mockStore) ResolveStackRowByImage(ctx context.Context, image string) (store.ResolveStackRowByImageRow, error) {
	if m.resolveStackRowByImageErr != nil {
		return store.ResolveStackRowByImageRow{}, m.resolveStackRowByImageErr
	}
	if m.resolveStackRowByImageResult.ID == 0 {
		return store.ResolveStackRowByImageRow{}, pgx.ErrNoRows
	}
	return m.resolveStackRowByImageResult, nil
}

func (m *mockStore) ResolveStackRowByLangTool(ctx context.Context, arg store.ResolveStackRowByLangToolParams) (store.ResolveStackRowByLangToolRow, error) {
	if m.resolveStackRowByLangToolErr != nil {
		return store.ResolveStackRowByLangToolRow{}, m.resolveStackRowByLangToolErr
	}
	if m.resolveStackRowByLangToolResult.ID == 0 {
		return store.ResolveStackRowByLangToolRow{}, pgx.ErrNoRows
	}
	return m.resolveStackRowByLangToolResult, nil
}

func (m *mockStore) ResolveStackRowByLangToolRelease(ctx context.Context, arg store.ResolveStackRowByLangToolReleaseParams) (store.ResolveStackRowByLangToolReleaseRow, error) {
	if m.resolveStackRowByLangToolReleaseErr != nil {
		return store.ResolveStackRowByLangToolReleaseRow{}, m.resolveStackRowByLangToolReleaseErr
	}
	if m.resolveStackRowByLangToolReleaseResult.ID == 0 {
		return store.ResolveStackRowByLangToolReleaseRow{}, pgx.ErrNoRows
	}
	return m.resolveStackRowByLangToolReleaseResult, nil
}

func (m *mockStore) UpsertExactGateProfile(ctx context.Context, arg store.UpsertExactGateProfileParams) (store.UpsertExactGateProfileRow, error) {
	m.upsertExactGateProfileCalled = true
	m.upsertExactGateProfileParam = arg
	if m.upsertExactGateProfileErr != nil {
		return store.UpsertExactGateProfileRow{}, m.upsertExactGateProfileErr
	}
	if m.upsertExactGateProfileResult.ID != 0 {
		return m.upsertExactGateProfileResult, nil
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

func (m *mockStore) UpsertGateJobProfileLink(ctx context.Context, arg store.UpsertGateJobProfileLinkParams) error {
	_, err := m.upsertGateJobProfileLink.record(arg)
	return err
}

func (m *mockStore) UpdateJobNextID(ctx context.Context, params store.UpdateJobNextIDParams) error {
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
