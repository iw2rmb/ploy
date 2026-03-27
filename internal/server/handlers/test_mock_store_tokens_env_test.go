package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

func (m *mockStore) InsertAPIToken(ctx context.Context, params store.InsertAPITokenParams) error {
	return m.insertAPITokenErr
}

func (m *mockStore) ListAPITokens(ctx context.Context, clusterID *string) ([]store.ListAPITokensRow, error) {
	return m.listAPITokensResult, m.listAPITokensErr
}

func (m *mockStore) RevokeAPIToken(ctx context.Context, tokenID string) error {
	return m.revokeAPITokenErr
}

func (m *mockStore) CheckAPITokenRevoked(ctx context.Context, tokenID string) (pgtype.Timestamptz, error) {
	return m.checkAPITokenRevokedResult, m.checkAPITokenRevokedErr
}

func (m *mockStore) UpdateAPITokenLastUsed(ctx context.Context, tokenID string) error {
	return m.updateAPITokenLastUsedErr
}

// Bootstrap Token methods

func (m *mockStore) InsertBootstrapToken(ctx context.Context, params store.InsertBootstrapTokenParams) error {
	return m.insertBootstrapTokenErr
}

func (m *mockStore) GetBootstrapToken(ctx context.Context, tokenID string) (store.GetBootstrapTokenRow, error) {
	return m.getBootstrapTokenResult, m.getBootstrapTokenErr
}

func (m *mockStore) CheckBootstrapTokenRevoked(ctx context.Context, tokenID string) (pgtype.Timestamptz, error) {
	return m.checkBootstrapTokenRevokedResult, m.checkBootstrapTokenRevokedErr
}

func (m *mockStore) UpdateBootstrapTokenLastUsed(ctx context.Context, tokenID string) error {
	return m.updateBootstrapTokenLastUsedErr
}

func (m *mockStore) MarkBootstrapTokenUsed(ctx context.Context, tokenID string) error {
	return m.markBootstrapTokenUsedErr
}

// RunRepo methods for batch run handlers

func (m *mockStore) ListGlobalEnv(ctx context.Context) ([]store.ConfigEnv, error) {
	return m.listGlobalEnvResult, m.listGlobalEnvErr
}

func (m *mockStore) GetGlobalEnv(ctx context.Context, key string) (store.ConfigEnv, error) {
	return m.getGlobalEnvResult, m.getGlobalEnvErr
}

func (m *mockStore) UpsertGlobalEnv(ctx context.Context, params store.UpsertGlobalEnvParams) error {
	m.upsertGlobalEnvCalled = true
	m.upsertGlobalEnvParams = params
	return m.upsertGlobalEnvErr
}

func (m *mockStore) DeleteGlobalEnv(ctx context.Context, key string) error {
	m.deleteGlobalEnvCalled = true
	m.deleteGlobalEnvParam = key
	return m.deleteGlobalEnvErr
}
