package handlers

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// runStore is a focused mock for run listing/timing/delete, batch operations,
// pull resolution, ingest, events, and run-repo-jobs handler tests.
type runStore struct {
	store.Store

	// Run queries
	getRun       mockCall[string, store.Run]
	getRunTiming mockCall[string, store.RunsTiming]

	listRunsTimings mockResult[[]store.RunsTiming]
	listRunsResult  []store.Run
	listRunsErr     error

	deleteRun   mockCall[string, struct{}]
	cancelRunV1 mockCall[string, struct{}]

	updateRunStatus mockCall[store.UpdateRunStatusParams, struct{}]

	// Run repo queries
	countRunReposByStatus    mockResult[[]store.CountRunReposByStatusRow]
	listRunReposByRun        mockCall[string, []store.RunRepo]
	listRunReposWithURLByRun mockCall[string, []store.ListRunReposWithURLByRunRow]

	getLatestRunRepoByMigAndRepoStatus mockCall[store.GetLatestRunRepoByMigAndRepoStatusParams, store.GetLatestRunRepoByMigAndRepoStatusRow]

	listQueuedRunReposByRun mockCall[string, []store.RunRepo]

	getRunRepoCalled  bool
	getRunRepoParam   store.GetRunRepoParams
	getRunRepoResult  store.RunRepo
	getRunRepoResults []store.RunRepo
	getRunRepoCalls   int
	getRunRepoErr     error

	updateRunRepoRefs       mockCall[store.UpdateRunRepoRefsParams, struct{}]
	updateMigRepoRefs       mockCall[store.UpdateMigRepoRefsParams, struct{}]
	incrementRunRepoAttempt mockCall[store.IncrementRunRepoAttemptParams, struct{}]

	updateRunRepoStatusCalled bool
	updateRunRepoStatusParams []store.UpdateRunRepoStatusParams
	updateRunRepoStatusErr    error

	// Create run repo (for batch add)
	createRunRepoCalled bool
	createRunRepoParams store.CreateRunRepoParams
	createRunRepoResult store.RunRepo
	createRunRepoErr    error

	// Mig repo (for batch operations and pull)
	createMigRepoResult store.MigRepo
	createMigRepoCalled bool
	createMigRepoParams store.CreateMigRepoParams
	createMigRepoErr    error

	getMigResult store.Mig
	getMigErr    error

	getMigRepo mockResult[store.MigRepo]

	listMigReposByMigResult []store.MigRepo
	listMigReposByMigErr    error

	repoByID map[types.RepoID]store.Repo

	// Spec (for batch scheduler)
	getSpec mockCall[string, store.Spec]

	// Job (for batch, ingest, and run-repo-jobs)
	getJobCalled bool
	getJobParams string
	getJobResult store.Job
	getJobErr    error

	createJobCalled    bool
	createJobCallCount int
	createJobParams    []store.CreateJobParams
	createJobResult    store.Job
	createJobErr       error

	scheduleNextJobCalled bool
	scheduleNextJobParam  store.ScheduleNextJobParams
	scheduleNextJobResult store.Job
	scheduleNextJobErr    error

	listJobsByRunRepoAttempt       mockCall[store.ListJobsByRunRepoAttemptParams, []store.Job]
	listArtifactBundlesByRunAndJob mockCall[store.ListArtifactBundlesByRunAndJobParams, []store.ArtifactBundle]
	listSBOMRowsByJob              mockCall[types.JobID, []store.Sbom]

	// Ingest (logs, diffs, artifacts)
	createLog            mockResult[store.Log]
	createDiff           mockCall[store.CreateDiffParams, store.Diff]
	createArtifactBundle mockResult[store.ArtifactBundle]

	// Events
	createEvent mockResult[store.Event]

	// Run creation (for batch scheduler assertions)
	createRunCalled bool
}

// Run query methods

func (m *runStore) GetRun(ctx context.Context, id types.RunID) (store.Run, error) {
	return m.getRun.record(id.String())
}

func (m *runStore) GetRunTiming(ctx context.Context, id types.RunID) (store.RunsTiming, error) {
	return m.getRunTiming.record(id.String())
}

func (m *runStore) ListRunsTimings(ctx context.Context, arg store.ListRunsTimingsParams) ([]store.RunsTiming, error) {
	return m.listRunsTimings.ret()
}

func (m *runStore) ListRuns(ctx context.Context, params store.ListRunsParams) ([]store.Run, error) {
	if int(params.Offset) >= len(m.listRunsResult) {
		return []store.Run{}, m.listRunsErr
	}
	end := int(params.Offset) + int(params.Limit)
	if end > len(m.listRunsResult) {
		end = len(m.listRunsResult)
	}
	return m.listRunsResult[params.Offset:end], m.listRunsErr
}

func (m *runStore) DeleteRun(ctx context.Context, id types.RunID) error {
	_, err := m.deleteRun.record(id.String())
	return err
}

func (m *runStore) CancelRunV1(ctx context.Context, runID types.RunID) error {
	_, err := m.cancelRunV1.record(runID.String())
	return err
}

func (m *runStore) UpdateRunStatus(ctx context.Context, params store.UpdateRunStatusParams) error {
	_, err := m.updateRunStatus.record(params)
	return err
}

// Run repo methods

func (m *runStore) CountRunReposByStatus(ctx context.Context, runID types.RunID) ([]store.CountRunReposByStatusRow, error) {
	return m.countRunReposByStatus.ret()
}

func (m *runStore) ListRunReposByRun(ctx context.Context, runID types.RunID) ([]store.RunRepo, error) {
	return m.listRunReposByRun.record(runID.String())
}

func (m *runStore) ListRunReposWithURLByRun(ctx context.Context, runID types.RunID) ([]store.ListRunReposWithURLByRunRow, error) {
	return m.listRunReposWithURLByRun.record(runID.String())
}

func (m *runStore) GetLatestRunRepoByMigAndRepoStatus(ctx context.Context, arg store.GetLatestRunRepoByMigAndRepoStatusParams) (store.GetLatestRunRepoByMigAndRepoStatusRow, error) {
	return m.getLatestRunRepoByMigAndRepoStatus.record(arg)
}

func (m *runStore) ListQueuedRunReposByRun(ctx context.Context, runID types.RunID) ([]store.RunRepo, error) {
	return m.listQueuedRunReposByRun.record(runID.String())
}

func (m *runStore) GetRunRepo(ctx context.Context, arg store.GetRunRepoParams) (store.RunRepo, error) {
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

func (m *runStore) UpdateRunRepoRefs(ctx context.Context, params store.UpdateRunRepoRefsParams) error {
	_, err := m.updateRunRepoRefs.record(params)
	return err
}

func (m *runStore) UpdateRunRepoStatus(ctx context.Context, params store.UpdateRunRepoStatusParams) error {
	m.updateRunRepoStatusCalled = true
	m.updateRunRepoStatusParams = append(m.updateRunRepoStatusParams, params)
	return m.updateRunRepoStatusErr
}

func (m *runStore) CreateRunRepo(ctx context.Context, params store.CreateRunRepoParams) (store.RunRepo, error) {
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

func (m *runStore) IncrementRunRepoAttempt(ctx context.Context, arg store.IncrementRunRepoAttemptParams) error {
	_, err := m.incrementRunRepoAttempt.record(arg)
	return err
}

// Mig/Repo methods (for batch and pull)

func (m *runStore) CreateMigRepo(ctx context.Context, params store.CreateMigRepoParams) (store.MigRepo, error) {
	m.createMigRepoCalled = true
	m.createMigRepoParams = params
	result := m.createMigRepoResult
	if result.ID.IsZero() {
		result.ID = params.ID
	}
	if result.MigID.IsZero() {
		result.MigID = params.MigID
	}
	if result.RepoID.IsZero() {
		result.RepoID = types.NewRepoID()
	}
	if result.BaseRef == "" {
		result.BaseRef = params.BaseRef
	}
	if result.TargetRef == "" {
		result.TargetRef = params.TargetRef
	}
	if m.repoByID == nil {
		m.repoByID = map[types.RepoID]store.Repo{}
	}
	m.repoByID[result.RepoID] = store.Repo{ID: result.RepoID, Url: params.Url}
	return result, m.createMigRepoErr
}

func (m *runStore) GetMig(ctx context.Context, id types.MigID) (store.Mig, error) {
	if m.getMigErr != nil {
		return store.Mig{}, m.getMigErr
	}
	result := m.getMigResult
	if result.ID.IsZero() {
		result.ID = id
	}
	return result, nil
}

func (m *runStore) GetMigRepo(ctx context.Context, id types.MigRepoID) (store.MigRepo, error) {
	return m.getMigRepo.ret()
}

func (m *runStore) ListMigReposByMig(ctx context.Context, migID types.MigID) ([]store.MigRepo, error) {
	return m.listMigReposByMigResult, m.listMigReposByMigErr
}

func (m *runStore) UpdateMigRepoRefs(ctx context.Context, params store.UpdateMigRepoRefsParams) error {
	_, err := m.updateMigRepoRefs.record(params)
	return err
}

func (m *runStore) GetRepo(ctx context.Context, id types.RepoID) (store.Repo, error) {
	if m.repoByID != nil {
		if repo, ok := m.repoByID[id]; ok {
			return repo, nil
		}
	}
	if !id.IsZero() {
		return store.Repo{ID: id, Url: "https://github.com/user/repo.git"}, nil
	}
	return store.Repo{}, pgx.ErrNoRows
}

// Spec methods

func (m *runStore) GetSpec(ctx context.Context, id types.SpecID) (store.Spec, error) {
	return m.getSpec.record(id.String())
}

// Job methods (for batch, ingest)

func (m *runStore) GetJob(ctx context.Context, id types.JobID) (store.Job, error) {
	m.getJobCalled = true
	m.getJobParams = id.String()
	return m.getJobResult, m.getJobErr
}

func (m *runStore) CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error) {
	m.createJobCalled = true
	m.createJobCallCount++
	m.createJobParams = append(m.createJobParams, params)
	result := m.createJobResult
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
	return result, m.createJobErr
}

func (m *runStore) ScheduleNextJob(ctx context.Context, arg store.ScheduleNextJobParams) (store.Job, error) {
	m.scheduleNextJobCalled = true
	m.scheduleNextJobParam = arg
	if m.scheduleNextJobErr != nil {
		return store.Job{}, m.scheduleNextJobErr
	}
	if m.scheduleNextJobResult.ID.IsZero() {
		return store.Job{}, pgx.ErrNoRows
	}
	return m.scheduleNextJobResult, nil
}

func (m *runStore) ListJobsByRunRepoAttempt(ctx context.Context, arg store.ListJobsByRunRepoAttemptParams) ([]store.Job, error) {
	return m.listJobsByRunRepoAttempt.record(arg)
}

func (m *runStore) ListArtifactBundlesByRunAndJob(ctx context.Context, arg store.ListArtifactBundlesByRunAndJobParams) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesByRunAndJob.record(arg)
}

func (m *runStore) ListSBOMRowsByJob(ctx context.Context, jobID types.JobID) ([]store.Sbom, error) {
	return m.listSBOMRowsByJob.record(jobID)
}

// Ingest methods

func (m *runStore) CreateLog(ctx context.Context, params store.CreateLogParams) (store.Log, error) {
	return m.createLog.ret()
}

func (m *runStore) CreateDiff(ctx context.Context, params store.CreateDiffParams) (store.Diff, error) {
	return m.createDiff.record(params)
}

func (m *runStore) CreateArtifactBundle(ctx context.Context, params store.CreateArtifactBundleParams) (store.ArtifactBundle, error) {
	return m.createArtifactBundle.ret()
}

// Event methods

func (m *runStore) CreateEvent(ctx context.Context, params store.CreateEventParams) (store.Event, error) {
	return m.createEvent.ret()
}
