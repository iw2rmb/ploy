package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestNodeDiagnosticsHandlers_CurrentContract(t *testing.T) {
	now := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	nodeID := types.NodeID("aB3xY9")

	tests := []struct {
		name   string
		run    func(t *testing.T, st *nodeStore)
		verify func(t *testing.T, st *nodeStore)
	}{
		{
			name: "post diagnostic stores structured state",
			run: func(t *testing.T, st *nodeStore) {
				h := upsertNodeDiagnosticHandler(st)
				body := `{
  "component": "node-updater",
  "status": "ok",
  "version": "v1",
  "image_ref": "registry/node:latest",
  "local_image_id": "sha256:local",
  "remote_image_id": "sha256:remote",
  "details": {"phase":"manifest"},
  "last_checked_at": "2026-05-22T10:00:00Z",
  "last_success_at": "2026-05-22T10:00:00Z"
}`
				rr := doRequest(t, h, http.MethodPost, "/v1/nodes/"+nodeID.String()+"/diagnostics", body, "id", nodeID.String())
				assertStatus(t, rr, http.StatusOK)
			},
			verify: func(t *testing.T, st *nodeStore) {
				assertCalled(t, "UpsertNodeDiagnostic", st.upsertDiagnostic.called)
				got := st.upsertDiagnostic.params
				if got.NodeID != nodeID || got.Component != "node-updater" || got.Status != "ok" {
					t.Fatalf("diagnostic key/status = %s/%s/%s", got.NodeID, got.Component, got.Status)
				}
				if string(got.Details) != `{"phase":"manifest"}` {
					t.Fatalf("details = %s", got.Details)
				}
				if !got.LastCheckedAt.Valid || !got.LastCheckedAt.Time.Equal(now) {
					t.Fatalf("last_checked_at = %+v", got.LastCheckedAt)
				}
			},
		},
		{
			name: "list diagnostics returns rows",
			run: func(t *testing.T, st *nodeStore) {
				st.listDiagnostics.val = []store.NodeDiagnostic{{
					NodeID:    nodeID,
					Component: "node",
					Status:    "ok",
					Details:   []byte(`{"docker_root":"/mnt/ploy/docker"}`),
					UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
				}}
				h := listNodeDiagnosticsHandler(st)
				rr := doRequest(t, h, http.MethodGet, "/v1/nodes/"+nodeID.String()+"/diagnostics", "", "id", nodeID.String())
				assertStatus(t, rr, http.StatusOK)
				var out []nodeDiagnosticResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if len(out) != 1 || out[0].Component != "node" || string(out[0].Details) != `{"docker_root":"/mnt/ploy/docker"}` {
					t.Fatalf("response = %+v", out)
				}
			},
			verify: func(t *testing.T, st *nodeStore) {
				assertCalled(t, "ListNodeDiagnostics", st.listDiagnostics.called)
			},
		},
		{
			name: "post daemon logs stores lines and trims",
			run: func(t *testing.T, st *nodeStore) {
				h := createNodeDaemonLogsHandler(st)
				body := `{"component":"node-updater","stream":"stderr","lines":["pull ok","cycle ok"]}`
				rr := doRequest(t, h, http.MethodPost, "/v1/nodes/"+nodeID.String()+"/daemon-logs", body, "id", nodeID.String())
				assertStatus(t, rr, http.StatusCreated)
			},
			verify: func(t *testing.T, st *nodeStore) {
				if len(st.createDaemonLog.calls) != 2 {
					t.Fatalf("CreateNodeDaemonLog calls = %d, want 2", len(st.createDaemonLog.calls))
				}
				if st.createDaemonLog.calls[0].Message != "pull ok" || st.createDaemonLog.calls[1].Message != "cycle ok" {
					t.Fatalf("messages = %+v", st.createDaemonLog.calls)
				}
				assertCalled(t, "TrimNodeDaemonLogs", st.trimDaemonLogs.called)
			},
		},
		{
			name: "list daemon logs forwards filter and limit",
			run: func(t *testing.T, st *nodeStore) {
				st.listDaemonLogs.val = []store.NodeDaemonLog{{
					ID:        7,
					NodeID:    nodeID,
					Component: "node-updater",
					Stream:    "stderr",
					Message:   "unauthorized",
					CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
				}}
				h := listNodeDaemonLogsHandler(st)
				rr := doRequest(t, h, http.MethodGet, "/v1/nodes/"+nodeID.String()+"/daemon-logs?component=node-updater&limit=50", "", "id", nodeID.String())
				assertStatus(t, rr, http.StatusOK)
			},
			verify: func(t *testing.T, st *nodeStore) {
				assertCalled(t, "ListNodeDaemonLogs", st.listDaemonLogs.called)
				if st.listDaemonLogs.params.Component == nil || *st.listDaemonLogs.params.Component != "node-updater" {
					t.Fatalf("component filter = %+v", st.listDaemonLogs.params.Component)
				}
				if st.listDaemonLogs.params.LimitCount != 50 {
					t.Fatalf("limit = %d, want 50", st.listDaemonLogs.params.LimitCount)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &nodeStore{}
			st.getNode.val = store.Node{ID: nodeID}
			tt.run(t, st)
			tt.verify(t, st)
		})
	}
}
