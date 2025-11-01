//go:build legacy
// +build legacy

package deploy

import (
	"context"
	"errors"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EnsurePKIOptions configures EnsureClusterPKI behaviour.
type EnsurePKIOptions struct {
	RequestedAt time.Time
}

// EnsureClusterPKI ensures the cluster PKI is bootstrapped, returning true when a new CA was created.
func EnsureClusterPKI(ctx context.Context, client *clientv3.Client, clusterID string, opts EnsurePKIOptions) (bool, error) {
	if client == nil {
		return false, errors.New("deploy: etcd client required")
	}
	trimmed := strings.TrimSpace(clusterID)
	if trimmed == "" {
		return false, errors.New("deploy: cluster id required")
	}
	manager, err := NewCARotationManager(client, trimmed)
	if err != nil {
		return false, err
	}
	if _, err := manager.State(ctx); err == nil {
		return false, nil
	} else if !errors.Is(err, ErrPKINotBootstrapped) {
		return false, err
	}

	bootstrap := BootstrapOptions{
		RequestedAt: opts.RequestedAt,
	}
	if bootstrap.RequestedAt.IsZero() {
		bootstrap.RequestedAt = time.Now().UTC()
	}
	if _, err := manager.Bootstrap(ctx, bootstrap); err != nil {
		if errors.Is(err, ErrPKIAlreadyBootstrapped) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
