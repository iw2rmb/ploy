package status_test

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/status"
	"github.com/iw2rmb/ploy/internal/worker/lifecycle"
)

// snapshotSource is a test implementation of status.SnapshotSource.
// Returns typed NodeStatus for compile-time safety.
type snapshotSource struct {
	nodeStatus lifecycle.NodeStatus
	hasStatus  bool
}

// LatestStatus implements the SnapshotSource interface.
func (s snapshotSource) LatestStatus() (lifecycle.NodeStatus, bool) {
	if !s.hasStatus {
		return lifecycle.NodeStatus{}, false
	}
	return s.nodeStatus, true
}

func TestProviderSnapshotFallback(t *testing.T) {
	provider := status.New(status.Options{Role: "control-plane"})
	data, err := provider.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if data["state"] != "ok" {
		t.Fatalf("state=%v", data["state"])
	}
	if data["role"] != "control-plane" {
		t.Fatalf("role=%v", data["role"])
	}
	if data["go_version"] == "" {
		t.Fatalf("expected go_version field")
	}
}

func TestProviderSnapshotFromSource(t *testing.T) {
	now := time.Now().UTC()
	provider := status.New(status.Options{
		Role: "worker",
		Source: snapshotSource{
			hasStatus: true,
			nodeStatus: lifecycle.NodeStatus{
				State:     "custom",
				Timestamp: now,
				Heartbeat: now,
				Role:      "worker",
				NodeID:    "test-node",
				Hostname:  "test-host",
			},
		},
	})
	data, err := provider.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if data["state"] != "custom" {
		t.Fatalf("expected cached state, got %v", data["state"])
	}
	if _, ok := data["go_version"]; ok {
		t.Fatalf("go_version should not be set when using cached snapshot")
	}
}
