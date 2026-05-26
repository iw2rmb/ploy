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
	upsertDiagnostic    mockCall[store.UpsertNodeDiagnosticParams, store.NodeDiagnostic]
	listDiagnostics     mockCall[types.NodeID, []store.NodeDiagnostic]
	createDaemonLog     mockCallSlice[store.CreateNodeDaemonLogParams, store.NodeDaemonLog]
	listDaemonLogs      mockCall[store.ListNodeDaemonLogsParams, []store.NodeDaemonLog]
	trimDaemonLogs      mockCall[store.TrimNodeDaemonLogsParams, struct{}]
	createNodeAction    mockCall[store.CreateNodeActionParams, store.NodeAction]
	listNodeActions     mockCall[store.ListNodeActionsParams, []store.NodeAction]
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

func (m *nodeStore) UpsertNodeDiagnostic(ctx context.Context, params store.UpsertNodeDiagnosticParams) (store.NodeDiagnostic, error) {
	if m.upsertDiagnostic.val.NodeID.IsZero() {
		m.upsertDiagnostic.val.NodeID = params.NodeID
		m.upsertDiagnostic.val.Component = params.Component
		m.upsertDiagnostic.val.Status = params.Status
		m.upsertDiagnostic.val.LastError = params.LastError
		m.upsertDiagnostic.val.Version = params.Version
		m.upsertDiagnostic.val.ImageRef = params.ImageRef
		m.upsertDiagnostic.val.LocalImageID = params.LocalImageID
		m.upsertDiagnostic.val.RemoteImageID = params.RemoteImageID
		m.upsertDiagnostic.val.Details = params.Details
		m.upsertDiagnostic.val.LastCheckedAt = params.LastCheckedAt
		m.upsertDiagnostic.val.LastSuccessAt = params.LastSuccessAt
	}
	return m.upsertDiagnostic.record(params)
}

func (m *nodeStore) ListNodeDiagnostics(ctx context.Context, nodeID types.NodeID) ([]store.NodeDiagnostic, error) {
	return m.listDiagnostics.record(nodeID)
}

func (m *nodeStore) CreateNodeDaemonLog(ctx context.Context, params store.CreateNodeDaemonLogParams) (store.NodeDaemonLog, error) {
	return m.createDaemonLog.record(params)
}

func (m *nodeStore) ListNodeDaemonLogs(ctx context.Context, params store.ListNodeDaemonLogsParams) ([]store.NodeDaemonLog, error) {
	return m.listDaemonLogs.record(params)
}

func (m *nodeStore) TrimNodeDaemonLogs(ctx context.Context, params store.TrimNodeDaemonLogsParams) error {
	_, err := m.trimDaemonLogs.record(params)
	return err
}

func (m *nodeStore) CreateNodeAction(ctx context.Context, params store.CreateNodeActionParams) (store.NodeAction, error) {
	if m.createNodeAction.val.ID.IsZero() {
		m.createNodeAction.val.ID = params.ID
		m.createNodeAction.val.NodeID = params.NodeID
		m.createNodeAction.val.ActionType = params.ActionType
		m.createNodeAction.val.Status = params.Status
		m.createNodeAction.val.Meta = params.Meta
	}
	return m.createNodeAction.record(params)
}

func (m *nodeStore) ListNodeActions(ctx context.Context, params store.ListNodeActionsParams) ([]store.NodeAction, error) {
	return m.listNodeActions.record(params)
}
