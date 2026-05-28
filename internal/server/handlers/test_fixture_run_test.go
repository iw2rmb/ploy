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
	listRuns        mockResult[[]store.Run]

	deleteRun   mockCall[string, struct{}]
	cancelRunV1 mockCall[string, struct{}]

	updateRunStatus mockCall[store.UpdateRunStatusParams, struct{}]

	// Run repo queries
	countRunReposByStatus    mockResult[[]store.CountRunReposByStatusRow]
	listRunReposByRun        mockCall[string, []store.RunRepo]
	listRunReposWithURLByRun mockCall[string, []store.ListRunReposWithURLByRunRow]

	getLatestRunRepoByMigAndRepoStatus mockCall[store.GetLatestRunRepoByMigAndRepoStatusParams, store.GetLatestRunRepoByMigAndRepoStatusRow]

	listQueuedRunReposByRun mockCall[string, []store.RunRepo]

	getRunRepo mockCallSeq[store.GetRunRepoParams, store.RunRepo]

	updateRunRepoBaseRef    mockCall[store.UpdateRunRepoBaseRefParams, struct{}]
	updateMigRepoBaseRef    mockCall[store.UpdateMigRepoBaseRefParams, struct{}]
	incrementRunRepoAttempt mockCall[store.IncrementRunRepoAttemptParams, struct{}]
	updateRunRepoError      mockCall[store.UpdateRunRepoErrorParams, struct{}]

	updateRunRepoStatus mockCallSlice[store.UpdateRunRepoStatusParams, struct{}]

	// Create run repo (for batch add)
	createRunRepo mockCall[store.CreateRunRepoParams, store.RunRepo]

	// Mig repo (for batch operations and pull)
	createMigRepo mockCall[store.CreateMigRepoParams, store.MigRepo]

	getMig mockResult[store.Mig]

	getMigRepo mockResult[store.MigRepo]

	listMigReposByMig mockResult[[]store.MigRepo]

	repoByID map[types.RepoID]store.Repo

	// Spec (for batch scheduler)
	getSpec mockCall[string, store.Spec]

	// Job (for batch, ingest, and run-repo-jobs)
	getJob mockCall[types.JobID, store.Job]

	createJob mockCallSlice[store.CreateJobParams, store.Job]

	scheduleNextJob mockCall[store.ScheduleNextJobParams, store.Job]

	listJobsByRunRepoAttempt       mockCall[store.ListJobsByRunRepoAttemptParams, []store.Job]
	listArtifactBundlesByRunAndJob mockCall[store.ListArtifactBundlesByRunAndJobParams, []store.ArtifactBundle]

	// Ingest (logs, diffs, artifacts)
	createLog            mockResult[store.Log]
	createDiff           mockCall[store.CreateDiffParams, store.Diff]
	createArtifactBundle mockResult[store.ArtifactBundle]

	// Events
	createEvent mockResult[store.Event]

	// Run creation tripwire (for batch scheduler assertions; runStore has no
	// CreateRun method, so this flag stays at its zero value — the check exists
	// only to fail loudly if the code path is ever wired into the mock).
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
	return listPaged(m.listRuns.val, params.Offset, params.Limit), m.listRuns.err
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
	return m.getRunRepo.record(arg)
}

func (m *runStore) UpdateRunRepoBaseRef(ctx context.Context, params store.UpdateRunRepoBaseRefParams) error {
	_, err := m.updateRunRepoBaseRef.record(params)
	return err
}

func (m *runStore) UpdateRunRepoStatus(ctx context.Context, params store.UpdateRunRepoStatusParams) error {
	_, err := m.updateRunRepoStatus.record(params)
	return err
}

func (m *runStore) UpdateRunRepoError(ctx context.Context, params store.UpdateRunRepoErrorParams) error {
	_, err := m.updateRunRepoError.record(params)
	return err
}

func (m *runStore) CreateRunRepo(ctx context.Context, params store.CreateRunRepoParams) (store.RunRepo, error) {
	result := defaultRunRepo(m.createRunRepo.val, params)
	m.createRunRepo.val = result
	_, err := m.createRunRepo.record(params)
	return result, err
}

func (m *runStore) IncrementRunRepoAttempt(ctx context.Context, arg store.IncrementRunRepoAttemptParams) error {
	_, err := m.incrementRunRepoAttempt.record(arg)
	return err
}

// Mig/Repo methods (for batch and pull)

func (m *runStore) CreateMigRepo(ctx context.Context, params store.CreateMigRepoParams) (store.MigRepo, error) {
	result := defaultMigRepo(m.createMigRepo.val, params.ID, params.MigID, params.BaseRef)
	if m.repoByID == nil {
		m.repoByID = map[types.RepoID]store.Repo{}
	}
	m.repoByID[result.RepoID] = store.Repo{ID: result.RepoID, Url: params.Url}
	m.createMigRepo.val = result
	_, err := m.createMigRepo.record(params)
	return result, err
}

func (m *runStore) GetMig(ctx context.Context, id types.MigID) (store.Mig, error) {
	if m.getMig.err != nil {
		return store.Mig{}, m.getMig.err
	}
	result := m.getMig.val
	if result.ID.IsZero() {
		result.ID = id
	}
	return result, nil
}

func (m *runStore) GetMigRepo(ctx context.Context, id types.MigRepoID) (store.MigRepo, error) {
	return m.getMigRepo.ret()
}

func (m *runStore) ListMigReposByMig(ctx context.Context, migID types.MigID) ([]store.MigRepo, error) {
	return m.listMigReposByMig.ret()
}

func (m *runStore) UpdateMigRepoBaseRef(ctx context.Context, params store.UpdateMigRepoBaseRefParams) error {
	_, err := m.updateMigRepoBaseRef.record(params)
	return err
}

func (m *runStore) GetRepo(ctx context.Context, id types.RepoID) (store.Repo, error) {
	if m.repoByID != nil {
		if repo, ok := m.repoByID[id]; ok {
			return repo, nil
		}
	}
	return defaultRepo(id)
}

// Spec methods

func (m *runStore) GetSpec(ctx context.Context, id types.SpecID) (store.Spec, error) {
	return m.getSpec.record(id.String())
}

// Job methods (for batch, ingest)

func (m *runStore) GetJob(ctx context.Context, id types.JobID) (store.Job, error) {
	return m.getJob.record(id)
}

func (m *runStore) CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error) {
	m.createJob.called = true
	m.createJob.calls = append(m.createJob.calls, params)
	result := buildCreateJobResult(m.createJob.val, params)
	return result, m.createJob.err
}

func (m *runStore) ScheduleNextJob(ctx context.Context, arg store.ScheduleNextJobParams) (store.Job, error) {
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

func (m *runStore) ListJobsByRunRepoAttempt(ctx context.Context, arg store.ListJobsByRunRepoAttemptParams) ([]store.Job, error) {
	return m.listJobsByRunRepoAttempt.record(arg)
}

func (m *runStore) ListArtifactBundlesByRunAndJob(ctx context.Context, arg store.ListArtifactBundlesByRunAndJobParams) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesByRunAndJob.record(arg)
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
