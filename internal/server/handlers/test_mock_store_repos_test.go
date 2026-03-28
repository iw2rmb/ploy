package handlers

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func (m *mockStore) CreateMigRepo(ctx context.Context, params store.CreateMigRepoParams) (store.MigRepo, error) {
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

func (m *mockStore) GetMigRepo(ctx context.Context, id types.MigRepoID) (store.MigRepo, error) {
	return m.getModRepo.ret()
}

func (m *mockStore) ListMigReposByMig(ctx context.Context, modID types.MigID) ([]store.MigRepo, error) {
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

func (m *mockStore) ListDistinctRepos(ctx context.Context, filter string) ([]store.ListDistinctReposRow, error) {
	return m.listDistinctRepos.record(filter)
}

func (m *mockStore) ListRunsForRepo(ctx context.Context, arg store.ListRunsForRepoParams) ([]store.ListRunsForRepoRow, error) {
	return m.listRunsForRepo.record(arg)
}

func (m *mockStore) GetMigRepoByURL(ctx context.Context, arg store.GetMigRepoByURLParams) (store.MigRepo, error) {
	return m.getModRepoByURL.record(arg)
}

// UpsertMigRepo upserts a mod_repo by mig_id and repo_url.
func (m *mockStore) UpsertMigRepo(ctx context.Context, arg store.UpsertMigRepoParams) (store.MigRepo, error) {
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

func (m *mockStore) GetRepo(ctx context.Context, id types.RepoID) (store.Repo, error) {
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

func (m *mockStore) DeleteMigRepo(ctx context.Context, id types.MigRepoID) error {
	return m.deleteMigRepo.err
}

func (m *mockStore) HasMigRepoHistory(ctx context.Context, repoID types.RepoID) (bool, error) {
	return m.hasModRepoHistory.ret()
}

func (m *mockStore) ListFailedRepoIDsByMig(ctx context.Context, modID types.MigID) ([]types.RepoID, error) {
	return m.listFailedRepoIDsByMod.record(modID.String())
}

func (m *mockStore) UpdateMigRepoRefs(ctx context.Context, params store.UpdateMigRepoRefsParams) error {
	_, err := m.updateMigRepoRefs.record(params)
	return err
}
