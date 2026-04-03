package handlers

import (
	"context"

	"github.com/iw2rmb/ploy/internal/store"
)

// configStore is a focused mock for global env, config ca, config home, and spec bundle handler tests.
type configStore struct {
	store.Store

	// Global Env
	listGlobalEnv   mockResult[[]store.ConfigEnv]
	getGlobalEnv    mockResult[store.ConfigEnv]
	upsertGlobalEnv mockCall[store.UpsertGlobalEnvParams, struct{}]
	deleteGlobalEnv mockCall[store.DeleteGlobalEnvParams, struct{}]

	// Config CA
	upsertConfigCA mockCall[store.UpsertConfigCAParams, struct{}]
	deleteConfigCA mockCall[store.DeleteConfigCAParams, struct{}]

	// Config Home
	upsertConfigHome mockCall[store.UpsertConfigHomeParams, struct{}]
	deleteConfigHome mockCall[store.DeleteConfigHomeParams, struct{}]

	// Config In
	upsertConfigIn mockCall[store.UpsertConfigInParams, struct{}]
	deleteConfigIn mockCall[store.DeleteConfigInParams, struct{}]

	// Bundle Map
	upsertConfigBundleMap mockCall[store.UpsertConfigBundleMapParams, struct{}]

	// Spec Bundles
	createSpecBundle mockCall[store.CreateSpecBundleParams, store.SpecBundle]
	getSpecBundle    mockResult[store.SpecBundle]
	getSpecBundleByCID mockResult[store.SpecBundle]
	deleteSpecBundle   mockResult[struct{}]

	updateSpecBundleLastRefAtCalled  bool
	updateSpecBundleLastRefAtParam   string
	updateSpecBundleLastRefAtErr     error
	updateSpecBundleLastRefAtStarted chan struct{}
	updateSpecBundleLastRefAtProceed chan struct{}
	updateSpecBundleLastRefAtDone    chan struct{}
	updateSpecBundleLastRefAtCtxErr  error
}

// Global Env methods

func (m *configStore) ListGlobalEnv(ctx context.Context) ([]store.ConfigEnv, error) {
	return m.listGlobalEnv.ret()
}

func (m *configStore) GetGlobalEnv(ctx context.Context, arg store.GetGlobalEnvParams) (store.ConfigEnv, error) {
	return m.getGlobalEnv.ret()
}

func (m *configStore) UpsertGlobalEnv(ctx context.Context, params store.UpsertGlobalEnvParams) error {
	_, err := m.upsertGlobalEnv.record(params)
	return err
}

func (m *configStore) DeleteGlobalEnv(ctx context.Context, arg store.DeleteGlobalEnvParams) error {
	_, err := m.deleteGlobalEnv.record(arg)
	return err
}

// Config CA methods

func (m *configStore) UpsertConfigCA(ctx context.Context, params store.UpsertConfigCAParams) error {
	_, err := m.upsertConfigCA.record(params)
	return err
}

func (m *configStore) DeleteConfigCA(ctx context.Context, arg store.DeleteConfigCAParams) error {
	_, err := m.deleteConfigCA.record(arg)
	return err
}

// Config Home methods

func (m *configStore) UpsertConfigHome(ctx context.Context, params store.UpsertConfigHomeParams) error {
	_, err := m.upsertConfigHome.record(params)
	return err
}

func (m *configStore) DeleteConfigHome(ctx context.Context, arg store.DeleteConfigHomeParams) error {
	_, err := m.deleteConfigHome.record(arg)
	return err
}

// Spec Bundle methods

func (m *configStore) CreateSpecBundle(ctx context.Context, params store.CreateSpecBundleParams) (store.SpecBundle, error) {
	return m.createSpecBundle.record(params)
}

func (m *configStore) GetSpecBundle(ctx context.Context, id string) (store.SpecBundle, error) {
	return m.getSpecBundle.ret()
}

func (m *configStore) GetSpecBundleByCID(ctx context.Context, cid string) (store.SpecBundle, error) {
	return m.getSpecBundleByCID.ret()
}

func (m *configStore) UpdateSpecBundleLastRefAt(ctx context.Context, id string) error {
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

func (m *configStore) DeleteSpecBundle(ctx context.Context, id string) error {
	return m.deleteSpecBundle.err
}

// Config In methods

func (m *configStore) UpsertConfigIn(ctx context.Context, params store.UpsertConfigInParams) error {
	_, err := m.upsertConfigIn.record(params)
	return err
}

func (m *configStore) DeleteConfigIn(ctx context.Context, arg store.DeleteConfigInParams) error {
	_, err := m.deleteConfigIn.record(arg)
	return err
}

// Bundle Map methods

func (m *configStore) UpsertConfigBundleMap(ctx context.Context, params store.UpsertConfigBundleMapParams) error {
	_, err := m.upsertConfigBundleMap.record(params)
	return err
}
