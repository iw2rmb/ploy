package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

func (m *mockStore) InsertAPIToken(ctx context.Context, params store.InsertAPITokenParams) error {
	return m.insertAPIToken.err
}

func (m *mockStore) ListAPITokens(ctx context.Context, clusterID *string) ([]store.ListAPITokensRow, error) {
	return m.listAPITokens.ret()
}

func (m *mockStore) RevokeAPIToken(ctx context.Context, tokenID string) error {
	return m.revokeAPIToken.err
}

func (m *mockStore) CheckAPITokenRevoked(ctx context.Context, tokenID string) (pgtype.Timestamptz, error) {
	return m.checkAPITokenRevoked.ret()
}

func (m *mockStore) UpdateAPITokenLastUsed(ctx context.Context, tokenID string) error {
	return m.updateAPITokenLastUsed.err
}

// Bootstrap Token methods

func (m *mockStore) InsertBootstrapToken(ctx context.Context, params store.InsertBootstrapTokenParams) error {
	return m.insertBootstrapToken.err
}

func (m *mockStore) GetBootstrapToken(ctx context.Context, tokenID string) (store.GetBootstrapTokenRow, error) {
	return m.getBootstrapToken.ret()
}

func (m *mockStore) CheckBootstrapTokenRevoked(ctx context.Context, tokenID string) (pgtype.Timestamptz, error) {
	return m.checkBootstrapTokenRevoked.ret()
}

func (m *mockStore) UpdateBootstrapTokenLastUsed(ctx context.Context, tokenID string) error {
	return m.updateBootstrapTokenLastUsed.err
}

func (m *mockStore) MarkBootstrapTokenUsed(ctx context.Context, tokenID string) error {
	return m.markBootstrapTokenUsed.err
}

// Global Env methods

func (m *mockStore) ListGlobalEnv(ctx context.Context) ([]store.ConfigEnv, error) {
	return m.listGlobalEnv.ret()
}

func (m *mockStore) GetGlobalEnv(ctx context.Context, key string) (store.ConfigEnv, error) {
	return m.getGlobalEnv.ret()
}

func (m *mockStore) UpsertGlobalEnv(ctx context.Context, params store.UpsertGlobalEnvParams) error {
	_, err := m.upsertGlobalEnv.record(params)
	return err
}

func (m *mockStore) DeleteGlobalEnv(ctx context.Context, key string) error {
	_, err := m.deleteGlobalEnv.record(key)
	return err
}
