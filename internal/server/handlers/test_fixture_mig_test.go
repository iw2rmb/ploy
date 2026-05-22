package handlers

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// migStore is a focused mock for mig CRUD, spec, mig-repo, and run-submit handler tests.
type migStore struct {
	store.Store

	// Spec
	createSpec mockCall[store.CreateSpecParams, store.Spec]

	getSpec       mockCall[string, store.Spec]
	updateMigSpec mockCall[store.UpdateMigSpecParams, struct{}]

	// Mig CRUD
	createMig mockCall[store.CreateMigParams, store.Mig]

	listMigs mockCall[store.ListMigsParams, []store.Mig]

	getMig mockCall[types.MigID, store.Mig]

	getMigByName mockCall[string, store.Mig]

	deleteMig    mockCall[string, struct{}]
	archiveMig   mockCall[string, struct{}]
	unarchiveMig mockCall[string, struct{}]

	// MigRepo
	createMigRepo mockCall[store.CreateMigRepoParams, store.MigRepo]

	getMigRepo mockResult[store.MigRepo]

	listMigReposByMig        mockCall[types.MigID, []store.MigRepo]
	listMigReposByMigResults map[string][]store.MigRepo

	getMigRepoByURL mockCall[store.GetMigRepoByURLParams, store.MigRepo]

	upsertMigRepo mockCall[store.UpsertMigRepoParams, store.MigRepo]

	deleteMigRepo     mockResult[struct{}]
	hasMigRepoHistory mockResult[bool]
	updateMigRepoRefs mockCall[store.UpdateMigRepoRefsParams, struct{}]

	listFailedRepoIDsByMig mockCall[string, []types.RepoID]

	repoByID map[types.RepoID]store.Repo

	// Run creation (for migs_runs, runs_submit). Sequenced for tests that
	// observe a different result/err for each CreateRun call.
	createRun mockCallSeq[store.CreateRunParams, store.Run]

	createRunRepo       mockCall[store.CreateRunRepoParams, store.RunRepo]
	createRunRepoParams []store.CreateRunRepoParams

	// Run/Job queries (for archive validation and migs_ticket)
	getRun        mockCall[string, store.Run]
	listRuns      mockResult[[]store.Run]
	listJobsByRun mockCall[types.RunID, []store.Job]

	// Job creation (for migs_ticket, runs_submit)
	createJob mockCallSlice[store.CreateJobParams, store.Job]

	// Artifact (for migs_ticket)
	listArtifactBundlesByRunAndJob mockResult[[]store.ArtifactBundle]

	// Event
	createEvent mockResult[store.Event]
}

// Spec methods

func (m *migStore) CreateSpec(ctx context.Context, params store.CreateSpecParams) (store.Spec, error) {
	m.createSpec.called = true
	m.createSpec.params = params
	result := store.Spec{ID: params.ID, Spec: params.Spec, CreatedBy: params.CreatedBy}
	return result, m.createSpec.err
}

func (m *migStore) GetSpec(ctx context.Context, id types.SpecID) (store.Spec, error) {
	return m.getSpec.record(id.String())
}

func (m *migStore) UpdateMigSpec(ctx context.Context, params store.UpdateMigSpecParams) error {
	_, err := m.updateMigSpec.record(params)
	return err
}

// Mig CRUD methods

func (m *migStore) CreateMig(ctx context.Context, params store.CreateMigParams) (store.Mig, error) {
	m.createMig.called = true
	m.createMig.params = params
	result := store.Mig{ID: params.ID, Name: params.Name, SpecID: params.SpecID, CreatedBy: params.CreatedBy}
	return result, m.createMig.err
}

func (m *migStore) ListMigs(ctx context.Context, params store.ListMigsParams) ([]store.Mig, error) {
	m.listMigs.called = true
	m.listMigs.params = params
	return listPaged(m.listMigs.val, params.Offset, params.Limit), m.listMigs.err
}

func (m *migStore) GetMig(ctx context.Context, id types.MigID) (store.Mig, error) {
	m.getMig.called = true
	m.getMig.params = id
	if m.getMig.err != nil {
		return store.Mig{}, m.getMig.err
	}
	result := m.getMig.val
	if result.ID.IsZero() {
		result.ID = id
	}
	if result.Name == "" {
		result.Name = "mig-" + id.String()
	}
	return result, nil
}

func (m *migStore) GetMigByName(ctx context.Context, name string) (store.Mig, error) {
	m.getMigByName.called = true
	m.getMigByName.params = name
	if m.getMigByName.err != nil {
		return store.Mig{}, m.getMigByName.err
	}
	result := m.getMigByName.val
	if result.ID.IsZero() && result.Name == "" {
		return store.Mig{}, pgx.ErrNoRows
	}
	if result.Name == "" {
		result.Name = name
	}
	return result, nil
}

func (m *migStore) DeleteMig(ctx context.Context, id types.MigID) error {
	_, err := m.deleteMig.record(id.String())
	return err
}

func (m *migStore) ArchiveMig(ctx context.Context, id types.MigID) error {
	_, err := m.archiveMig.record(id.String())
	return err
}

func (m *migStore) UnarchiveMig(ctx context.Context, id types.MigID) error {
	_, err := m.unarchiveMig.record(id.String())
	return err
}

// MigRepo methods

func (m *migStore) CreateMigRepo(ctx context.Context, params store.CreateMigRepoParams) (store.MigRepo, error) {
	result := defaultMigRepo(m.createMigRepo.val, params.ID, params.MigID, params.BaseRef, params.TargetRef)
	if m.repoByID == nil {
		m.repoByID = map[types.RepoID]store.Repo{}
	}
	m.repoByID[result.RepoID] = store.Repo{ID: result.RepoID, Url: params.Url}
	m.createMigRepo.val = result
	_, err := m.createMigRepo.record(params)
	return result, err
}

func (m *migStore) GetMigRepo(ctx context.Context, id types.MigRepoID) (store.MigRepo, error) {
	return m.getMigRepo.ret()
}

func (m *migStore) ListMigReposByMig(ctx context.Context, migID types.MigID) ([]store.MigRepo, error) {
	if m.listMigReposByMigResults != nil {
		if repos, ok := m.listMigReposByMigResults[migID.String()]; ok {
			m.listMigReposByMig.called = true
			m.listMigReposByMig.params = migID
			return repos, m.listMigReposByMig.err
		}
	}
	return m.listMigReposByMig.record(migID)
}

func (m *migStore) GetMigRepoByURL(ctx context.Context, arg store.GetMigRepoByURLParams) (store.MigRepo, error) {
	return m.getMigRepoByURL.record(arg)
}

func (m *migStore) UpsertMigRepo(ctx context.Context, arg store.UpsertMigRepoParams) (store.MigRepo, error) {
	result := defaultMigRepo(m.upsertMigRepo.val, arg.ID, arg.MigID, arg.BaseRef, arg.TargetRef)
	if m.repoByID == nil {
		m.repoByID = map[types.RepoID]store.Repo{}
	}
	m.repoByID[result.RepoID] = store.Repo{ID: result.RepoID, Url: arg.Url}
	m.upsertMigRepo.val = result
	_, err := m.upsertMigRepo.record(arg)
	return result, err
}

func (m *migStore) DeleteMigRepo(ctx context.Context, id types.MigRepoID) error {
	return m.deleteMigRepo.err
}

func (m *migStore) HasMigRepoHistory(ctx context.Context, repoID types.RepoID) (bool, error) {
	return m.hasMigRepoHistory.ret()
}

func (m *migStore) ListFailedRepoIDsByMig(ctx context.Context, migID types.MigID) ([]types.RepoID, error) {
	return m.listFailedRepoIDsByMig.record(migID.String())
}

func (m *migStore) UpdateMigRepoRefs(ctx context.Context, params store.UpdateMigRepoRefsParams) error {
	_, err := m.updateMigRepoRefs.record(params)
	return err
}

func (m *migStore) GetRepo(ctx context.Context, id types.RepoID) (store.Repo, error) {
	if m.repoByID != nil {
		if repo, ok := m.repoByID[id]; ok {
			return repo, nil
		}
	}
	return defaultRepo(id)
}

// Run creation methods

func (m *migStore) CreateRun(ctx context.Context, params store.CreateRunParams) (store.Run, error) {
	if len(m.createRun.vals) > 0 || len(m.createRun.errs) > 0 {
		return m.createRun.record(params)
	}
	// No configured result: synthesize per-call from params.
	m.createRun.called = true
	m.createRun.params = params
	m.createRun.n++
	return defaultRun(store.Run{}, params), nil
}

func (m *migStore) CreateRunRepo(ctx context.Context, params store.CreateRunRepoParams) (store.RunRepo, error) {
	result := defaultRunRepo(m.createRunRepo.val, params)
	m.createRunRepoParams = append(m.createRunRepoParams, params)
	_, err := m.createRunRepo.record(params)
	return result, err
}

func (m *migStore) CreateRunWithRepos(ctx context.Context, params store.CreateRunWithReposParams) (store.Run, []store.RunRepo, error) {
	run, err := m.CreateRun(ctx, params.Run)
	if err != nil {
		return store.Run{}, nil, err
	}

	repos := make([]store.RunRepo, 0, len(params.Repos))
	for _, repoParams := range params.Repos {
		repo, err := m.CreateRunRepo(ctx, repoParams)
		if err != nil {
			return store.Run{}, nil, err
		}
		repos = append(repos, repo)
	}
	return run, repos, nil
}

// Run/Job query methods (for archive validation and migs_ticket)

func (m *migStore) GetRun(ctx context.Context, id types.RunID) (store.Run, error) {
	return m.getRun.record(id.String())
}

func (m *migStore) ListRuns(ctx context.Context, params store.ListRunsParams) ([]store.Run, error) {
	return listPaged(m.listRuns.val, params.Offset, params.Limit), m.listRuns.err
}

func (m *migStore) ListJobsByRun(ctx context.Context, runID types.RunID) ([]store.Job, error) {
	m.listJobsByRun.called = true
	m.listJobsByRun.params = runID
	return m.listJobsByRun.val, m.listJobsByRun.err
}

func (m *migStore) CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error) {
	m.createJob.called = true
	m.createJob.calls = append(m.createJob.calls, params)
	result := buildCreateJobResult(m.createJob.val, params)
	return result, m.createJob.err
}

func (m *migStore) ListArtifactBundlesByRunAndJob(ctx context.Context, arg store.ListArtifactBundlesByRunAndJobParams) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesByRunAndJob.ret()
}

func (m *migStore) CreateEvent(ctx context.Context, params store.CreateEventParams) (store.Event, error) {
	return m.createEvent.ret()
}
