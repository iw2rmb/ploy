package httpserver_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/deploy"
)

func mustBootstrapCluster(t *testing.T, client *clientv3.Client, clusterID string) *deploy.CARotationManager {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	manager, err := deploy.NewCARotationManager(client, clusterID)
	if err != nil {
		t.Fatalf("new ca rotation manager: %v", err)
	}
	_, err = manager.Bootstrap(ctx, deploy.BootstrapOptions{
		BeaconIDs: []string{"beacon-main"},
	})
	if err != nil && !errors.Is(err, deploy.ErrPKIAlreadyBootstrapped) {
		t.Fatalf("bootstrap ca: %v", err)
	}
	return manager
}

func stateCurrentCACert(t *testing.T, manager *deploy.CARotationManager, ctx context.Context) string {
	t.Helper()
	state, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("manager state: %v", err)
	}
	return state.CurrentCA.CertificatePEM
}

func beaconCanonicalKey(clusterID string) string {
	return fmt.Sprintf("/ploy/clusters/%s/beacon/canonical", clusterID)
}
