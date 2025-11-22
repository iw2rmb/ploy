package status_test

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/status"
)

type snapshotSource struct {
	status map[string]any
}

// LatestStatusMap implements the SnapshotSource interface.
func (s snapshotSource) LatestStatusMap() (map[string]any, bool) {
	if len(s.status) == 0 {
		return nil, false
	}
	return s.status, true
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
	provider := status.New(status.Options{
		Role: "worker",
		Source: snapshotSource{status: map[string]any{
			"state": "custom",
		}},
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
