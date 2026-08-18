package handlers

import (
	"context"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// Node management store methods.
func (m *handlerStore) UpdateNodeDrained(ctx context.Context, params store.UpdateNodeDrainedParams) error {
	_, err := m.updateNodeDrained.record(params)
	return err
}

func (m *handlerStore) ListNodes(ctx context.Context) ([]store.Node, error) {
	m.listNodes.called = true
	return m.listNodes.val, m.listNodes.err
}

func (m *handlerStore) UpdateNodeCertMetadata(ctx context.Context, params store.UpdateNodeCertMetadataParams) error {
	return m.updateCertMetadata.err
}

func (m *handlerStore) UpsertNodeDiagnostic(ctx context.Context, params store.UpsertNodeDiagnosticParams) (store.NodeDiagnostic, error) {
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

func (m *handlerStore) ListNodeDiagnostics(ctx context.Context, nodeID types.NodeID) ([]store.NodeDiagnostic, error) {
	return m.listDiagnostics.record(nodeID)
}

func (m *handlerStore) CreateNodeDaemonLog(ctx context.Context, params store.CreateNodeDaemonLogParams) (store.NodeDaemonLog, error) {
	return m.createDaemonLog.record(params)
}

func (m *handlerStore) ListNodeDaemonLogs(ctx context.Context, params store.ListNodeDaemonLogsParams) ([]store.NodeDaemonLog, error) {
	return m.listDaemonLogs.record(params)
}

func (m *handlerStore) TrimNodeDaemonLogs(ctx context.Context, params store.TrimNodeDaemonLogsParams) error {
	_, err := m.trimDaemonLogs.record(params)
	return err
}
