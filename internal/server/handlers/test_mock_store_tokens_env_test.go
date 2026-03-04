package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

func (m *mockStore) InsertAPIToken(ctx context.Context, params store.InsertAPITokenParams) error {
	m.insertAPITokenCalled = true
	m.insertAPITokenParams = params
	return m.insertAPITokenErr
}

func (m *mockStore) ListAPITokens(ctx context.Context, clusterID *string) ([]store.ListAPITokensRow, error) {
	m.listAPITokensCalled = true
	m.listAPITokensParams = clusterID
	return m.listAPITokensResult, m.listAPITokensErr
}

func (m *mockStore) RevokeAPIToken(ctx context.Context, tokenID string) error {
	m.revokeAPITokenCalled = true
	m.revokeAPITokenParam = tokenID
	return m.revokeAPITokenErr
}

func (m *mockStore) CheckAPITokenRevoked(ctx context.Context, tokenID string) (pgtype.Timestamptz, error) {
	m.checkAPITokenRevokedCalled = true
	m.checkAPITokenRevokedParam = tokenID
	return m.checkAPITokenRevokedResult, m.checkAPITokenRevokedErr
}

func (m *mockStore) UpdateAPITokenLastUsed(ctx context.Context, tokenID string) error {
	m.updateAPITokenLastUsedCalled = true
	m.updateAPITokenLastUsedParam = tokenID
	return m.updateAPITokenLastUsedErr
}

// Bootstrap Token methods

func (m *mockStore) InsertBootstrapToken(ctx context.Context, params store.InsertBootstrapTokenParams) error {
	m.insertBootstrapTokenCalled = true
	m.insertBootstrapTokenParams = params
	return m.insertBootstrapTokenErr
}

func (m *mockStore) GetBootstrapToken(ctx context.Context, tokenID string) (store.GetBootstrapTokenRow, error) {
	m.getBootstrapTokenCalled = true
	m.getBootstrapTokenParam = tokenID
	return m.getBootstrapTokenResult, m.getBootstrapTokenErr
}

func (m *mockStore) CheckBootstrapTokenRevoked(ctx context.Context, tokenID string) (pgtype.Timestamptz, error) {
	m.checkBootstrapTokenRevokedCalled = true
	m.checkBootstrapTokenRevokedParam = tokenID
	return m.checkBootstrapTokenRevokedResult, m.checkBootstrapTokenRevokedErr
}

func (m *mockStore) UpdateBootstrapTokenLastUsed(ctx context.Context, tokenID string) error {
	m.updateBootstrapTokenLastUsedCalled = true
	m.updateBootstrapTokenLastUsedParam = tokenID
	return m.updateBootstrapTokenLastUsedErr
}

func (m *mockStore) MarkBootstrapTokenUsed(ctx context.Context, tokenID string) error {
	m.markBootstrapTokenUsedCalled = true
	m.markBootstrapTokenUsedParam = tokenID
	return m.markBootstrapTokenUsedErr
}

// RunRepo methods for batch run handlers

func (m *mockStore) ListGlobalEnv(ctx context.Context) ([]store.ConfigEnv, error) {
	m.listGlobalEnvCalled = true
	return m.listGlobalEnvResult, m.listGlobalEnvErr
}

func (m *mockStore) GetGlobalEnv(ctx context.Context, key string) (store.ConfigEnv, error) {
	m.getGlobalEnvCalled = true
	m.getGlobalEnvParam = key
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
