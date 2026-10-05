package handlers

import (
	"context"

	"github.com/iw2rmb/ploy/internal/store"
)

// Global env, config input, and spec bundle store methods.
// Global Env methods

func (m *handlerStore) ListGlobalEnv(ctx context.Context) ([]store.ConfigEnv, error) {
	return m.listGlobalEnv.ret()
}

func (m *handlerStore) GetGlobalEnv(ctx context.Context, arg store.GetGlobalEnvParams) (store.ConfigEnv, error) {
	return m.getGlobalEnv.ret()
}

func (m *handlerStore) UpsertGlobalEnv(ctx context.Context, params store.UpsertGlobalEnvParams) error {
	_, err := m.upsertGlobalEnv.record(params)
	return err
}

func (m *handlerStore) DeleteGlobalEnv(ctx context.Context, arg store.DeleteGlobalEnvParams) error {
	_, err := m.deleteGlobalEnv.record(arg)
	return err
}

// Spec Bundle methods

func (m *handlerStore) CreateSpecBundle(ctx context.Context, params store.CreateSpecBundleParams) (store.SpecBundle, error) {
	return m.createSpecBundle.record(params)
}

func (m *handlerStore) GetSpecBundle(ctx context.Context, id string) (store.SpecBundle, error) {
	result, err := m.getSpecBundle.ret()
	if err == nil && result.ID == "" {
		result.ID = id
	}
	return result, err
}

func (m *handlerStore) GetSpecBundleByCID(ctx context.Context, cid string) (store.SpecBundle, error) {
	return m.getSpecBundleByCID.ret()
}

func (m *handlerStore) UpdateSpecBundleLastRefAt(ctx context.Context, id string) error {
	m.updateSpecBundleLastRefAtCalled = true
	m.updateSpecBundleLastRefAtParam = id
	if m.updateSpecBundleLastRefAtStarted != nil {
		close(m.updateSpecBundleLastRefAtStarted)
	}
	if m.updateSpecBundleLastRefAtProceed != nil {
		<-m.updateSpecBundleLastRefAtProceed
	}
	m.updateSpecBundleLastRefAtCtxErr = ctx.Err()
	if m.updateSpecBundleLastRefAtDone != nil {
		close(m.updateSpecBundleLastRefAtDone)
	}
	return m.updateSpecBundleLastRefAtErr
}

func (m *handlerStore) DeleteSpecBundle(ctx context.Context, id string) error {
	return m.deleteSpecBundle.err
}

// Config In methods

// Bundle Map methods
