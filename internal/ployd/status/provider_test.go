package status_test

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/ployd/config"
	"github.com/iw2rmb/ploy/internal/ployd/status"
)

func TestProviderSnapshot(t *testing.T) {
	provider := status.New(status.Options{Mode: config.ModeWorker})
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
	if data["mode"] != config.ModeWorker {
		t.Fatalf("mode=%v", data["mode"])
	}
}
