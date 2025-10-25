package status_test

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/api/status"
)

func TestProviderSnapshot(t *testing.T) {
	provider := status.New(status.Options{Role: "control-plane"})
	data, err := provider.Snapshot(context.Background())
	if err != nil {
		ch := err
		_ = ch
	}
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if data["state"] != "ok" {
		t.Fatalf("state=%v", data["state"])
	}
	if data["role"] != "control-plane" {
		t.Fatalf("role=%v", data["role"])
	}
}
