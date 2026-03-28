package handlers

import (
	"context"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// nodeStore is a focused mock for node management handler tests.
type nodeStore struct {
	store.Store
	getNode             mockCall[string, store.Node]
	updateNodeDrained   mockCall[store.UpdateNodeDrainedParams, struct{}]
	listNodes           mockCall[struct{}, []store.Node]
	updateNodeHeartbeat mockCall[store.UpdateNodeHeartbeatParams, struct{}]
	updateCertMetadata  mockResult[struct{}]
	createLog           mockResult[store.Log]
}

func (m *nodeStore) GetNode(ctx context.Context, id types.NodeID) (store.Node, error) {
	return m.getNode.record(id.String())
}

func (m *nodeStore) UpdateNodeDrained(ctx context.Context, params store.UpdateNodeDrainedParams) error {
	_, err := m.updateNodeDrained.record(params)
	return err
}

func (m *nodeStore) ListNodes(ctx context.Context) ([]store.Node, error) {
	m.listNodes.called = true
	return m.listNodes.val, m.listNodes.err
}

func (m *nodeStore) UpdateNodeHeartbeat(ctx context.Context, params store.UpdateNodeHeartbeatParams) error {
	_, err := m.updateNodeHeartbeat.record(params)
	return err
}

func (m *nodeStore) UpdateNodeCertMetadata(ctx context.Context, params store.UpdateNodeCertMetadataParams) error {
	return m.updateCertMetadata.err
}

func (m *nodeStore) CreateLog(ctx context.Context, params store.CreateLogParams) (store.Log, error) {
	return m.createLog.ret()
}
