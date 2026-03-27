package handlers

import (
	"context"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func (m *mockStore) CreateSpecBundle(ctx context.Context, params store.CreateSpecBundleParams) (store.SpecBundle, error) {
	m.createSpecBundleParams = params
	return m.createSpecBundleResult, m.createSpecBundleErr
}

func (m *mockStore) GetSpecBundle(ctx context.Context, id types.SpecBundleID) (store.SpecBundle, error) {
	return m.getSpecBundleResult, m.getSpecBundleErr
}

func (m *mockStore) GetSpecBundleByCID(ctx context.Context, cid string) (store.SpecBundle, error) {
	return m.getSpecBundleByCIDResult, m.getSpecBundleByCIDErr
}

func (m *mockStore) UpdateSpecBundleLastRefAt(ctx context.Context, id types.SpecBundleID) error {
	m.updateSpecBundleLastRefAtCalled = true
	m.updateSpecBundleLastRefAtParam = id.String()
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

func (m *mockStore) DeleteSpecBundle(ctx context.Context, id types.SpecBundleID) error {
	return m.deleteSpecBundleErr
}
