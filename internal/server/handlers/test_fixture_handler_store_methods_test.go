package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func (m *handlerStore) GetJob(_ context.Context, id types.JobID) (store.Job, error) {
	if result, ok := m.getJobByID[id]; ok {
		m.getJob.called = true
		m.getJob.params = id
		return result, nil
	}
	return m.getJob.record(id)
}

func (m *handlerStore) CreateJob(_ context.Context, params store.CreateJobParams) (store.Job, error) {
	m.createJob.called = true
	m.createJob.params = params
	m.createJob.calls = append(m.createJob.calls, params)
	return buildCreateJobResult(m.createJob.val, params), m.createJob.err
}

func (m *handlerStore) ListJobsByRun(_ context.Context, runID types.RunID) ([]store.Job, error) {
	m.listJobsByRun.called = true
	m.listJobsByRun.params = runID
	result := make([]store.Job, len(m.listJobsByRun.val))
	for i, job := range m.listJobsByRun.val {
		result[i] = job
		if m.updateJobCompletion.called && job.ID == m.updateJobCompletion.params.ID {
			result[i].Status = m.updateJobCompletion.params.Status
		}
	}
	return result, m.listJobsByRun.err
}

func (m *handlerStore) ListJobsByRunAttempt(_ context.Context, params store.ListJobsByRunAttemptParams) ([]store.Job, error) {
	return m.listJobsByRunAttempt.record(params)
}

func (m *handlerStore) GetRun(_ context.Context, id types.RunID) (store.Run, error) {
	if result, ok := m.getRunByID[id]; ok {
		m.getRun.called = true
		m.getRun.params = id.String()
		return result, nil
	}
	if len(m.getRunSeq.vals) > 0 || len(m.getRunSeq.errs) > 0 {
		return m.getRunSeq.record(id)
	}
	return m.getRun.record(id.String())
}

func (m *handlerStore) GetWave(_ context.Context, id types.WaveID) (store.Wave, error) {
	return m.getWave.record(id.String())
}

func (m *handlerStore) UpdateRunStatus(_ context.Context, params store.UpdateRunStatusParams) error {
	_, err := m.updateRunStatus.record(params)
	return err
}

func (m *handlerStore) UpdateRunError(_ context.Context, params store.UpdateRunErrorParams) error {
	_, err := m.updateRunError.record(params)
	return err
}

func (m *handlerStore) ListRuns(_ context.Context, params store.ListRunsParams) ([]store.Run, error) {
	return listPaged(m.listRuns.val, params.Offset, params.Limit), m.listRuns.err
}

func (m *handlerStore) ListRunsByWave(_ context.Context, waveID types.WaveID) ([]store.Run, error) {
	return m.listRunsByWave.record(waveID.String())
}

func (m *handlerStore) ListRunsWithURLByWave(_ context.Context, waveID types.WaveID) ([]store.ListRunsWithURLByWaveRow, error) {
	return m.listRunsWithURLByWave.record(waveID.String())
}

func (m *handlerStore) CountRunsByWaveStatus(_ context.Context, _ types.WaveID) ([]store.CountRunsByWaveStatusRow, error) {
	return m.countRunsByStatus.ret()
}

func (m *handlerStore) GetLatestRunByMigAndRepoStatus(_ context.Context, params store.GetLatestRunByMigAndRepoStatusParams) (store.GetLatestRunByMigAndRepoStatusRow, error) {
	return m.getLatestRunByMigAndRepoStatus.record(params)
}

func (m *handlerStore) materializeRun(wave store.CreateWaveParams, params store.RunPlan) (store.Run, error) {
	m.createRunCalled = true
	if len(m.createRunSeq.vals) > 0 || len(m.createRunSeq.errs) > 0 {
		return m.createRunSeq.record(params)
	}
	result := defaultRun(m.createRun.val, wave, params)
	m.createRun.val = result
	_, err := m.createRun.record(params)
	return result, err
}

func (m *handlerStore) CreateWaveWithRuns(ctx context.Context, params store.CreateWaveWithRunsParams) (store.Wave, []store.RunMaterialization, error) {
	m.createWaveWithRuns.called = true
	m.createWaveWithRuns.params = params
	if m.createWaveWithRunsHook != nil {
		m.createWaveWithRunsHook(params)
	}
	if m.createWaveWithRuns.err != nil {
		return store.Wave{}, nil, m.createWaveWithRuns.err
	}
	wave := defaultWave(m.createWaveWithRuns.val, params.Wave)
	runs := make([]store.RunMaterialization, 0, len(params.Runs))
	for _, runPlan := range params.Runs {
		m.createRunParams = append(m.createRunParams, runPlan)
		run, err := m.materializeRun(params.Wave, runPlan)
		if err != nil {
			return store.Wave{}, nil, err
		}
		jobIDs := make([]types.JobID, len(runPlan.Jobs))
		for i := range jobIDs {
			jobIDs[i] = types.NewJobID()
		}
		jobs := make([]store.Job, len(runPlan.Jobs))
		for i := len(runPlan.Jobs) - 1; i >= 0; i-- {
			job := runPlan.Jobs[i]
			status := types.JobStatusCreated
			if i == 0 {
				status = types.JobStatusQueued
			}
			var nextID *types.JobID
			if i+1 < len(runPlan.Jobs) {
				nextID = &jobIDs[i+1]
			}
			repoSHAIn := ""
			if i == 0 {
				repoSHAIn = run.RepoSha0
			}
			created, err := m.CreateJob(ctx, store.CreateJobParams{
				ID:          jobIDs[i],
				RunID:       run.ID,
				RepoID:      run.RepoID,
				RepoBaseRef: run.RepoBaseRef,
				Attempt:     run.Attempt,
				Status:      status,
				JobType:     job.JobType,
				JobImage:    job.JobImage,
				NextID:      nextID,
				Name:        job.Name,
				Meta:        job.Meta,
				RepoShaIn:   repoSHAIn,
			})
			if err != nil {
				return store.Wave{}, nil, err
			}
			jobs[i] = created
		}
		runs = append(runs, store.RunMaterialization{Run: run, Jobs: jobs})
	}
	return wave, runs, nil
}

func (m *handlerStore) GetSpec(_ context.Context, id types.SpecID) (store.Spec, error) {
	return m.getSpec.record(id.String())
}

func (m *handlerStore) CreateSpec(_ context.Context, params store.CreateSpecParams) (store.Spec, error) {
	result := store.Spec{ID: params.ID, Spec: params.Spec, CreatedBy: params.CreatedBy}
	m.createSpec.val = result
	_, err := m.createSpec.record(params)
	return result, err
}

func (m *handlerStore) GetAPITokenByID(_ context.Context, tokenID string) (store.GetAPITokenByIDRow, error) {
	return m.getAPITokenByID.record(tokenID)
}

func (m *handlerStore) GetMig(_ context.Context, id types.MigID) (store.Mig, error) {
	result, err := m.getMig.record(id)
	if err != nil {
		return store.Mig{}, err
	}
	if result.ID.IsZero() {
		result.ID = id
	}
	if result.Name == "" {
		result.Name = "mig-" + id.String()
	}
	return result, nil
}

func (m *handlerStore) CreateMig(_ context.Context, params store.CreateMigParams) (store.Mig, error) {
	result := store.Mig{ID: params.ID, Name: params.Name, SpecID: params.SpecID, CreatedBy: params.CreatedBy}
	m.createMig.val = result
	_, err := m.createMig.record(params)
	return result, err
}

func (m *handlerStore) CreateMigRepo(_ context.Context, params store.CreateMigRepoParams) (store.MigRepo, error) {
	result := defaultMigRepo(m.createMigRepo.val, params.ID, params.MigID, params.BaseRef)
	if m.repoByID == nil {
		m.repoByID = map[types.RepoID]store.Repo{}
	}
	m.repoByID[result.RepoID] = store.Repo{ID: result.RepoID, Url: params.Url}
	m.createMigRepo.val = result
	_, err := m.createMigRepo.record(params)
	return result, err
}

func (m *handlerStore) GetMigRepo(_ context.Context, _ types.MigRepoID) (store.MigRepo, error) {
	return m.getMigRepo.ret()
}

func (m *handlerStore) ListMigReposByMig(_ context.Context, migID types.MigID) ([]store.MigRepo, error) {
	if result, ok := m.listMigReposByMigResults[migID.String()]; ok {
		m.listMigReposByMig.called = true
		m.listMigReposByMig.params = migID
		return result, nil
	}
	return m.listMigReposByMig.record(migID)
}

func (m *handlerStore) GetRepo(_ context.Context, id types.RepoID) (store.Repo, error) {
	if repo, ok := m.repoByID[id]; ok {
		return repo, nil
	}
	return defaultRepo(id)
}

func (m *handlerStore) ListDistinctRepos(_ context.Context, filter string) ([]store.ListDistinctReposRow, error) {
	return m.listDistinctRepos.record(filter)
}

func (m *handlerStore) ListRunsForRepo(_ context.Context, params store.ListRunsForRepoParams) ([]store.ListRunsForRepoRow, error) {
	return m.listRunsForRepo.record(params)
}

func (m *handlerStore) GetNode(_ context.Context, id types.NodeID) (store.Node, error) {
	return m.getNode.record(id.String())
}

func (m *handlerStore) UpdateNodeHeartbeat(_ context.Context, params store.UpdateNodeHeartbeatParams) error {
	_, err := m.updateNodeHeartbeat.record(params)
	return err
}

func (m *handlerStore) GetLatestDiffByJob(_ context.Context, jobID *types.JobID) (store.Diff, error) {
	if jobID != nil {
		if result, ok := m.getLatestDiffByJobByID[*jobID]; ok {
			m.getLatestDiffByJob.called = true
			m.getLatestDiffByJob.params = jobID
			return result, nil
		}
	}
	if m.getLatestDiffByJobError != nil {
		return store.Diff{}, m.getLatestDiffByJobError
	}
	return m.getLatestDiffByJob.record(jobID)
}

func (m *handlerStore) ListArtifactBundlesByRunAndJob(_ context.Context, params store.ListArtifactBundlesByRunAndJobParams) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesByRunAndJob.record(params)
}

func (m *handlerStore) CreateArtifactBundle(_ context.Context, _ store.CreateArtifactBundleParams) (store.ArtifactBundle, error) {
	return m.createArtifactBundle.ret()
}

func (m *handlerStore) DeleteArtifactBundle(_ context.Context, id pgtype.UUID) error {
	_, err := m.deleteArtifactBundle.record(id)
	return err
}

func (m *handlerStore) DeleteSBOMRowsByJob(_ context.Context, jobID types.JobID) error {
	_, err := m.deleteSBOMRowsByJob.record(jobID)
	return err
}

func (m *handlerStore) UpsertSBOMRow(_ context.Context, params store.UpsertSBOMRowParams) error {
	_, err := m.upsertSBOMRow.record(params)
	return err
}

func (m *handlerStore) CreateLog(_ context.Context, _ store.CreateLogParams) (store.Log, error) {
	return m.createLog.ret()
}

func (m *handlerStore) CreateEvent(_ context.Context, params store.CreateEventParams) (store.Event, error) {
	return m.createEvent.record(params)
}
