package httpserver_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

func TestNodeStatusPatchPersistsToEtcd(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() { _ = sched.Close() }()

	server := httptest.NewServer(newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
	}))
	defer server.Close()

	// Patch status for a specific node.
	nodeID := "node-xyz"
	status := map[string]any{
		"state":      "ok",
		"heartbeat":  "2025-10-25T12:00:00Z",
		"components": map[string]any{"docker": map[string]any{"state": "ok"}},
	}
	code, _ := patchJSONStatus(t, server.URL+"/v1/nodes/"+nodeID, status)
	if code != 204 {
		t.Fatalf("expected 204, got %d", code)
	}

	// Verify etcd key exists with the same JSON payload.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := client.Get(ctx, "nodes/"+nodeID+"/status")
	if err != nil {
		t.Fatalf("etcd get: %v", err)
	}
	if len(resp.Kvs) != 1 {
		t.Fatalf("expected one status record, got %d", len(resp.Kvs))
	}
	var got map[string]any
	if err := json.Unmarshal(resp.Kvs[0].Value, &got); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if got["state"] != "ok" {
		t.Fatalf("unexpected state: %v", got["state"])
	}
}
