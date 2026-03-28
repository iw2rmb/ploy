package handlers

import (
	"context"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func (m *mockStore) CreateSpecBundle(ctx context.Context, params store.CreateSpecBundleParams) (store.SpecBundle, error) {
	return m.createSpecBundle.record(params)
}

func (m *mockStore) GetSpecBundle(ctx context.Context, id types.SpecBundleID) (store.SpecBundle, error) {
	return m.getSpecBundle.ret()
}

func (m *mockStore) GetSpecBundleByCID(ctx context.Context, cid string) (store.SpecBundle, error) {
	return m.getSpecBundleByCID.ret()
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
	return m.deleteSpecBundle.err
}
