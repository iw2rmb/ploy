package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func (m *mockStore) CreateSpecBundle(ctx context.Context, params store.CreateSpecBundleParams) (store.SpecBundle, error) {
	m.createSpecBundleCalled = true
	m.createSpecBundleParams = params
	return m.createSpecBundleResult, m.createSpecBundleErr
}

func (m *mockStore) GetSpecBundle(ctx context.Context, id types.SpecBundleID) (store.SpecBundle, error) {
	m.getSpecBundleCalled = true
	m.getSpecBundleParam = id.String()
	return m.getSpecBundleResult, m.getSpecBundleErr
}

func (m *mockStore) GetSpecBundleByCID(ctx context.Context, cid string) (store.SpecBundle, error) {
	m.getSpecBundleByCIDCalled = true
	m.getSpecBundleByCIDParam = cid
	return m.getSpecBundleByCIDResult, m.getSpecBundleByCIDErr
}

func (m *mockStore) UpdateSpecBundleLastRefAt(ctx context.Context, id types.SpecBundleID) error {
	m.updateSpecBundleLastRefAtCalled = true
	m.updateSpecBundleLastRefAtParam = id.String()
	return m.updateSpecBundleLastRefAtErr
}

func (m *mockStore) DeleteSpecBundle(ctx context.Context, id types.SpecBundleID) error {
	m.deleteSpecBundleCalled = true
	m.deleteSpecBundleParam = id.String()
	return m.deleteSpecBundleErr
}

func (m *mockStore) ListSpecBundles(ctx context.Context, arg store.ListSpecBundlesParams) ([]store.SpecBundle, error) {
	return nil, nil
}

func (m *mockStore) ListSpecBundlesUnreferencedBefore(ctx context.Context, lastRefAt pgtype.Timestamptz) ([]store.SpecBundle, error) {
	return nil, nil
}
