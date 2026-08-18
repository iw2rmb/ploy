package handlers

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// Mig, spec, mig-repo, and run-submit store methods.
// Spec methods

func (m *handlerStore) CreateGitSpecSnapshot(ctx context.Context, params store.CreateGitSpecSnapshotParams) (store.Spec, error) {
	result := store.Spec{ID: params.ID, Name: params.Name, Description: params.Description, Source: params.Source, Sha: params.Sha, SourceCommittedAt: params.SourceCommittedAt, Spec: params.Spec, CreatedBy: params.CreatedBy}
	m.createGitSpecSnapshot.val = result
	return m.createGitSpecSnapshot.record(params)
}

func (m *handlerStore) GetGitSpecSnapshot(ctx context.Context, params store.GetGitSpecSnapshotParams) (store.Spec, error) {
	if !m.getGitSpec.called && m.getGitSpec.err == nil && m.getGitSpec.val.ID.IsZero() {
		m.getGitSpec.err = pgx.ErrNoRows
	}
	return m.getGitSpec.record(params)
}

func (m *handlerStore) UpdateMigSpec(ctx context.Context, params store.UpdateMigSpecParams) error {
	_, err := m.updateMigSpec.record(params)
	return err
}

// Mig CRUD methods

func (m *handlerStore) ListMigs(ctx context.Context, params store.ListMigsParams) ([]store.Mig, error) {
	m.listMigs.called = true
	m.listMigs.params = params
	return listPaged(m.listMigs.val, params.Offset, params.Limit), m.listMigs.err
}

func (m *handlerStore) GetMigByName(ctx context.Context, name string) (store.Mig, error) {
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

func (m *handlerStore) DeleteMig(ctx context.Context, id types.MigID) error {
	_, err := m.deleteMig.record(id.String())
	return err
}

func (m *handlerStore) ArchiveMig(ctx context.Context, id types.MigID) error {
	_, err := m.archiveMig.record(id.String())
	return err
}

func (m *handlerStore) UnarchiveMig(ctx context.Context, id types.MigID) error {
	_, err := m.unarchiveMig.record(id.String())
	return err
}

// MigRepo methods

func (m *handlerStore) GetMigRepoByURL(ctx context.Context, arg store.GetMigRepoByURLParams) (store.MigRepo, error) {
	return m.getMigRepoByURL.record(arg)
}

func (m *handlerStore) UpsertMigRepo(ctx context.Context, arg store.UpsertMigRepoParams) (store.MigRepo, error) {
	result := defaultMigRepo(m.upsertMigRepo.val, arg.ID, arg.MigID, arg.BaseRef)
	if m.repoByID == nil {
		m.repoByID = map[types.RepoID]store.Repo{}
	}
	m.repoByID[result.RepoID] = store.Repo{ID: result.RepoID, Url: arg.Url}
	m.upsertMigRepo.val = result
	_, err := m.upsertMigRepo.record(arg)
	return result, err
}

func (m *handlerStore) DeleteMigRepo(ctx context.Context, id types.MigRepoID) error {
	return m.deleteMigRepo.err
}

func (m *handlerStore) HasMigRepoHistory(ctx context.Context, repoID types.RepoID) (bool, error) {
	return m.hasMigRepoHistory.ret()
}

func (m *handlerStore) ListFailedRepoIDsByMig(ctx context.Context, migID types.MigID) ([]types.RepoID, error) {
	return m.listFailedRepoIDsByMig.record(migID.String())
}
