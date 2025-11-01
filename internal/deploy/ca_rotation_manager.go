//go:build legacy
// +build legacy

package deploy

import (
	"errors"
	"fmt"
	"strings"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/controlplane/security"
)

// CARotationManager manages certificate authority lifecycle for a cluster.
type CARotationManager struct {
	client    *clientv3.Client
	clusterID string
	prefix    string
	trust     *security.TrustStore
}

// NewCARotationManager constructs a manager for the supplied cluster identifier.
func NewCARotationManager(client *clientv3.Client, clusterID string) (*CARotationManager, error) {
	if client == nil {
		return nil, errors.New("deploy: etcd client required")
	}
	trimmed := strings.TrimSpace(clusterID)
	if trimmed == "" {
		return nil, errors.New("deploy: cluster id required")
	}
	trust, err := security.NewTrustStore(client, trimmed)
	if err != nil {
		return nil, err
	}
	prefix := fmt.Sprintf("%s/%s/security", clusterSecurityRoot, trimmed)
	return &CARotationManager{
		client:    client,
		clusterID: trimmed,
		prefix:    prefix,
		trust:     trust,
	}, nil
}

func (m *CARotationManager) caCurrentKey() string {
	return m.prefix + "/ca/current"
}

func (m *CARotationManager) caHistoryKey(version string) string {
	return m.prefix + "/ca/history/" + version
}

func (m *CARotationManager) revokedKey() string {
	return m.prefix + "/ca/revoked"
}

func (m *CARotationManager) nodesKey() string {
	return m.prefix + "/nodes"
}

func (m *CARotationManager) beaconPrefix() string {
	return m.prefix + "/certs/beacon/"
}

func (m *CARotationManager) workerPrefix() string {
	return m.prefix + "/certs/worker/"
}

func (m *CARotationManager) beaconCertKey(id string) string {
	return m.beaconPrefix() + id
}

func (m *CARotationManager) workerCertKey(id string) string {
	return m.workerPrefix() + id
}
