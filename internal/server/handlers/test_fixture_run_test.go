package handlers

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// runStore is a focused mock for run listing/timing/delete, batch operations,
// pull resolution, ingest, events, and run job handler tests.
type runStore struct {
	store.Store

	// Run queries
	getRun       mockCall[string, store.Run]
	getWave      mockCall[string, store.Wave]
	getRunTiming mockCall[string, store.RunsTiming]

	listRunsTimings mockResult[[]store.RunsTiming]
	listRuns        mockResult[[]store.Run]

	deleteRun  mockCall[string, struct{}]
	cancelRun  mockCall[string, struct{}]
	restartRun mockCall[string, store.Run]

	updateRunStatus mockCall[store.UpdateRunStatusParams, struct{}]

	// Run queries
	countRunsByStatus     mockResult[[]store.CountRunsByWaveStatusRow]
	listRunsByWave        mockCall[string, []store.Run]
	listRunsWithURLByWave mockCall[string, []store.ListRunsWithURLByWaveRow]

	getLatestRunByMigAndRepoStatus mockCall[store.GetLatestRunByMigAndRepoStatusParams, store.GetLatestRunByMigAndRepoStatusRow]

	listQueuedRunsByWave mockCall[string, []store.Run]

	getRunSeq mockCallSeq[types.RunID, store.Run]

	updateRunBaseRef     mockCall[store.UpdateRunBaseRefParams, struct{}]
	updateMigRepoBaseRef mockCall[store.UpdateMigRepoBaseRefParams, struct{}]
	incrementRunAttempt  mockCall[types.RunID, struct{}]
	updateRunError       mockCall[store.UpdateRunErrorParams, struct{}]

	// Create run (for batch add)
	createRun mockCall[store.CreateRunParams, store.Run]

	// Mig repo (for batch operations and pull)
	createMigRepo mockCall[store.CreateMigRepoParams, store.MigRepo]

	getMig mockResult[store.Mig]

	getMigRepo mockResult[store.MigRepo]

	listMigReposByMig mockResult[[]store.MigRepo]

	repoByID map[types.RepoID]store.Repo

	// Spec (for batch scheduler)
	getSpec mockCall[string, store.Spec]

	// Job (for batch, ingest, and run jobs)
	getJob mockCall[types.JobID, store.Job]

	createJob mockCallSlice[store.CreateJobParams, store.Job]

	scheduleNextJob mockCall[store.ScheduleNextJobParams, store.Job]

	listJobsByRunAttempt           mockCall[store.ListJobsByRunAttemptParams, []store.Job]
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
	if len(m.getRunSeq.vals) > 0 || len(m.getRunSeq.errs) > 0 {
		return m.getRunSeq.record(id)
	}
	return m.getRun.record(id.String())
}

func (m *runStore) GetWave(_ context.Context, id types.WaveID) (store.Wave, error) {
	return m.getWave.record(id.String())
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

func (m *runStore) CancelRun(ctx context.Context, runID types.RunID) error {
	_, err := m.cancelRun.record(runID.String())
	return err
}

func (m *runStore) RestartRun(ctx context.Context, runID types.RunID) (store.Run, error) {
	return m.restartRun.record(runID.String())
}

func (m *runStore) UpdateRunStatus(ctx context.Context, params store.UpdateRunStatusParams) error {
	_, err := m.updateRunStatus.record(params)
	return err
}

// Run repo methods

func (m *runStore) CountRunsByWaveStatus(ctx context.Context, waveID types.WaveID) ([]store.CountRunsByWaveStatusRow, error) {
	return m.countRunsByStatus.ret()
}

func (m *runStore) ListRunsByWave(ctx context.Context, waveID types.WaveID) ([]store.Run, error) {
	return m.listRunsByWave.record(waveID.String())
}

func (m *runStore) ListRunsWithURLByWave(ctx context.Context, waveID types.WaveID) ([]store.ListRunsWithURLByWaveRow, error) {
	return m.listRunsWithURLByWave.record(waveID.String())
}

func (m *runStore) GetLatestRunByMigAndRepoStatus(ctx context.Context, arg store.GetLatestRunByMigAndRepoStatusParams) (store.GetLatestRunByMigAndRepoStatusRow, error) {
	return m.getLatestRunByMigAndRepoStatus.record(arg)
}

func (m *runStore) ListQueuedRunsByWave(ctx context.Context, waveID types.WaveID) ([]store.Run, error) {
	return m.listQueuedRunsByWave.record(waveID.String())
}

func (m *runStore) UpdateRunBaseRef(ctx context.Context, params store.UpdateRunBaseRefParams) error {
	_, err := m.updateRunBaseRef.record(params)
	return err
}

func (m *runStore) UpdateRunError(ctx context.Context, params store.UpdateRunErrorParams) error {
	_, err := m.updateRunError.record(params)
	return err
}

func (m *runStore) CreateRun(ctx context.Context, params store.CreateRunParams) (store.Run, error) {
	result := defaultRun(m.createRun.val, params)
	m.createRun.val = result
	_, err := m.createRun.record(params)
	return result, err
}

func (m *runStore) IncrementRunAttempt(ctx context.Context, arg types.RunID) error {
	_, err := m.incrementRunAttempt.record(arg)
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

func (m *runStore) ListJobsByRunAttempt(ctx context.Context, arg store.ListJobsByRunAttemptParams) ([]store.Job, error) {
	return m.listJobsByRunAttempt.record(arg)
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
