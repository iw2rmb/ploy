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
	createSpecCalled bool
	createSpecParams store.CreateSpecParams
	createSpecResult store.Spec
	createSpecErr    error

	getSpec       mockCall[string, store.Spec]
	updateModSpec mockCall[store.UpdateMigSpecParams, struct{}]

	// Mig CRUD
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

	deleteMig    mockCall[string, struct{}]
	archiveMig   mockCall[string, struct{}]
	unarchiveMig mockCall[string, struct{}]

	// MigRepo
	createMigRepoCalled bool
	createMigRepoParams store.CreateMigRepoParams
	createMigRepoResult store.MigRepo
	createMigRepoErr    error

	getModRepo mockResult[store.MigRepo]

	listMigReposByModCalled  bool
	listMigReposByModParam   string
	listMigReposByModResult  []store.MigRepo
	listMigReposByModResults map[string][]store.MigRepo
	listMigReposByModErr     error

	getModRepoByURL mockCall[store.GetMigRepoByURLParams, store.MigRepo]

	upsertModRepoCalled bool
	upsertModRepoParams store.UpsertMigRepoParams
	upsertModRepoResult store.MigRepo
	upsertModRepoErr    error

	deleteMigRepo     mockResult[struct{}]
	hasModRepoHistory mockResult[bool]
	updateMigRepoRefs mockCall[store.UpdateMigRepoRefsParams, struct{}]

	listFailedRepoIDsByMod mockCall[string, []types.RepoID]

	repoByID map[types.RepoID]store.Repo

	// Run creation (for migs_runs, runs_submit)
	createRunCalled       bool
	createRunParams       store.CreateRunParams
	createRunResult       store.Run
	createRunErr          error
	createRunErrs         []error
	createRunErrCallCount int
	createRunResults      []store.Run
	createRunCallCount    int

	createRunRepoCalled bool
	createRunRepoParams store.CreateRunRepoParams
	createRunRepoResult store.RunRepo
	createRunRepoErr    error

	// Run/Job queries (for archive validation and migs_ticket)
	getRun             mockCall[string, store.Run]
	listRunsResult     []store.Run
	listRunsErr        error
	listJobsByRunResult []store.Job
	listJobsByRunCalled bool

	// Job creation (for migs_ticket, runs_submit)
	createJobCalled    bool
	createJobCallCount int
	createJobParams    []store.CreateJobParams
	createJobResult    store.Job
	createJobErr       error

	// Artifact (for migs_ticket)
	listArtifactBundlesByRunAndJob mockResult[[]store.ArtifactBundle]

	// Event
	createEvent mockResult[store.Event]
}

// Spec methods

func (m *migStore) CreateSpec(ctx context.Context, params store.CreateSpecParams) (store.Spec, error) {
	m.createSpecCalled = true
	m.createSpecParams = params
	result := m.createSpecResult
	if result.ID.IsZero() {
		result.ID = params.ID
	}
	if result.Spec == nil {
		result.Spec = params.Spec
	}
	result.CreatedBy = params.CreatedBy
	return result, m.createSpecErr
}

func (m *migStore) GetSpec(ctx context.Context, id types.SpecID) (store.Spec, error) {
	return m.getSpec.record(id.String())
}

func (m *migStore) UpdateMigSpec(ctx context.Context, params store.UpdateMigSpecParams) error {
	_, err := m.updateModSpec.record(params)
	return err
}

// Mig CRUD methods

func (m *migStore) CreateMig(ctx context.Context, params store.CreateMigParams) (store.Mig, error) {
	m.createMigCalled = true
	m.createMigParams = params
	result := m.createMigResult
	if result.ID.IsZero() {
		result.ID = params.ID
	}
	if result.Name == "" {
		result.Name = params.Name
	}
	result.SpecID = params.SpecID
	result.CreatedBy = params.CreatedBy
	return result, m.createMigErr
}

func (m *migStore) ListMigs(ctx context.Context, params store.ListMigsParams) ([]store.Mig, error) {
	m.listMigsCalled = true
	m.listMigsParams = params
	if int(params.Offset) >= len(m.listMigsResult) {
		return []store.Mig{}, m.listMigsErr
	}
	end := int(params.Offset) + int(params.Limit)
	if end > len(m.listMigsResult) {
		end = len(m.listMigsResult)
	}
	return m.listMigsResult[params.Offset:end], m.listMigsErr
}

func (m *migStore) GetMig(ctx context.Context, id types.MigID) (store.Mig, error) {
	m.getModCalled = true
	m.getModParam = id.String()
	if m.getModErr != nil {
		return store.Mig{}, m.getModErr
	}
	result := m.getModResult
	if result.ID.IsZero() {
		result.ID = id
	}
	if result.Name == "" {
		result.Name = "mig-" + id.String()
	}
	return result, nil
}

func (m *migStore) GetMigByName(ctx context.Context, name string) (store.Mig, error) {
	m.getModByNameCalled = true
	m.getModByNameParam = name
	if m.getModByNameErr != nil {
		return store.Mig{}, m.getModByNameErr
	}
	result := m.getModByNameResult
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

func (m *migStore) GetMigRepo(ctx context.Context, id types.MigRepoID) (store.MigRepo, error) {
	return m.getModRepo.ret()
}

func (m *migStore) ListMigReposByMig(ctx context.Context, modID types.MigID) ([]store.MigRepo, error) {
	m.listMigReposByModCalled = true
	modIDStr := modID.String()
	m.listMigReposByModParam = modIDStr
	if m.listMigReposByModResults != nil {
		if repos, ok := m.listMigReposByModResults[modIDStr]; ok {
			return repos, m.listMigReposByModErr
		}
	}
	return m.listMigReposByModResult, m.listMigReposByModErr
}

func (m *migStore) GetMigRepoByURL(ctx context.Context, arg store.GetMigRepoByURLParams) (store.MigRepo, error) {
	return m.getModRepoByURL.record(arg)
}

func (m *migStore) UpsertMigRepo(ctx context.Context, arg store.UpsertMigRepoParams) (store.MigRepo, error) {
	m.upsertModRepoCalled = true
	m.upsertModRepoParams = arg
	result := m.upsertModRepoResult
	if result.ID.IsZero() {
		result.ID = arg.ID
	}
	if result.MigID.IsZero() {
		result.MigID = arg.MigID
	}
	if result.RepoID.IsZero() {
		result.RepoID = types.NewRepoID()
	}
	if result.BaseRef == "" {
		result.BaseRef = arg.BaseRef
	}
	if result.TargetRef == "" {
		result.TargetRef = arg.TargetRef
	}
	if m.repoByID == nil {
		m.repoByID = map[types.RepoID]store.Repo{}
	}
	m.repoByID[result.RepoID] = store.Repo{ID: result.RepoID, Url: arg.Url}
	return result, m.upsertModRepoErr
}

func (m *migStore) DeleteMigRepo(ctx context.Context, id types.MigRepoID) error {
	return m.deleteMigRepo.err
}

func (m *migStore) HasMigRepoHistory(ctx context.Context, repoID types.RepoID) (bool, error) {
	return m.hasModRepoHistory.ret()
}

func (m *migStore) ListFailedRepoIDsByMig(ctx context.Context, modID types.MigID) ([]types.RepoID, error) {
	return m.listFailedRepoIDsByMod.record(modID.String())
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
	if !id.IsZero() {
		return store.Repo{ID: id, Url: "https://github.com/user/repo.git"}, nil
	}
	return store.Repo{}, pgx.ErrNoRows
}

// Run creation methods

func (m *migStore) CreateRun(ctx context.Context, params store.CreateRunParams) (store.Run, error) {
	m.createRunCalled = true
	m.createRunParams = params

	err := m.createRunErr
	if len(m.createRunErrs) > 0 {
		idx := m.createRunErrCallCount
		if idx >= len(m.createRunErrs) {
			idx = len(m.createRunErrs) - 1
		}
		m.createRunErrCallCount++
		err = m.createRunErrs[idx]
	}

	if len(m.createRunResults) > 0 {
		idx := m.createRunCallCount
		if idx >= len(m.createRunResults) {
			idx = len(m.createRunResults) - 1
		}
		m.createRunCallCount++
		return m.createRunResults[idx], err
	}
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
	return result, err
}

func (m *migStore) CreateRunRepo(ctx context.Context, params store.CreateRunRepoParams) (store.RunRepo, error) {
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

// Run/Job query methods (for archive validation and migs_ticket)

func (m *migStore) GetRun(ctx context.Context, id types.RunID) (store.Run, error) {
	return m.getRun.record(id.String())
}

func (m *migStore) ListRuns(ctx context.Context, params store.ListRunsParams) ([]store.Run, error) {
	if int(params.Offset) >= len(m.listRunsResult) {
		return []store.Run{}, m.listRunsErr
	}
	end := int(params.Offset) + int(params.Limit)
	if end > len(m.listRunsResult) {
		end = len(m.listRunsResult)
	}
	return m.listRunsResult[params.Offset:end], m.listRunsErr
}

func (m *migStore) ListJobsByRun(ctx context.Context, runID types.RunID) ([]store.Job, error) {
	m.listJobsByRunCalled = true
	return m.listJobsByRunResult, nil
}

func (m *migStore) CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error) {
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

func (m *migStore) ListArtifactBundlesByRunAndJob(ctx context.Context, arg store.ListArtifactBundlesByRunAndJobParams) ([]store.ArtifactBundle, error) {
	return m.listArtifactBundlesByRunAndJob.ret()
}

func (m *migStore) CreateEvent(ctx context.Context, params store.CreateEventParams) (store.Event, error) {
	return m.createEvent.ret()
}

