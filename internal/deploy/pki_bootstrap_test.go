//go:build legacy
// +build legacy

package deploy

import (
	"context"
	"testing"
	"time"
)

func TestEnsureClusterPKIBootstrapsOnce(t *testing.T) {
	ctx := context.Background()
	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	created, err := EnsureClusterPKI(ctx, client, "cluster-auto", EnsurePKIOptions{
		RequestedAt: time.Date(2025, 10, 25, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("EnsureClusterPKI initial: %v", err)
	}
	if !created {
		t.Fatalf("expected first EnsureClusterPKI call to create CA")
	}

	manager := mustNewCARotationManager(t, client, "cluster-auto")
	state, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("State after EnsureClusterPKI: %v", err)
	}
	if state.CurrentCA.Version == "" {
		t.Fatalf("expected CA version recorded after EnsureClusterPKI")
	}

	replayed, err := EnsureClusterPKI(ctx, client, "cluster-auto", EnsurePKIOptions{})
	if err != nil {
		t.Fatalf("EnsureClusterPKI second call: %v", err)
	}
	if replayed {
		t.Fatalf("expected second EnsureClusterPKI call to no-op")
	}
}
