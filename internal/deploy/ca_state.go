//go:build legacy
// +build legacy

package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// State loads the current CA state from storage.
func (m *CARotationManager) State(ctx context.Context) (CAState, error) {
	resp, err := m.client.Get(ctx, m.caCurrentKey())
	if err != nil {
		return CAState{}, fmt.Errorf("deploy: read current CA: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return CAState{}, ErrPKINotBootstrapped
	}
	var current CABundle
	if err := json.Unmarshal(resp.Kvs[0].Value, &current); err != nil {
		return CAState{}, fmt.Errorf("deploy: decode current CA: %w", err)
	}

	nodesResp, err := m.client.Get(ctx, m.nodesKey())
	if err != nil {
		return CAState{}, fmt.Errorf("deploy: read node inventory: %w", err)
	}
	if len(nodesResp.Kvs) == 0 {
		return CAState{}, errors.New("deploy: node inventory missing")
	}
	var nodes NodeSet
	if err := json.Unmarshal(nodesResp.Kvs[0].Value, &nodes); err != nil {
		return CAState{}, fmt.Errorf("deploy: decode node inventory: %w", err)
	}
	nodesRevision := nodesResp.Kvs[0].ModRevision

	beaconCerts, err := m.loadLeafCertificates(ctx, m.beaconPrefix())
	if err != nil {
		return CAState{}, fmt.Errorf("deploy: load beacon certs: %w", err)
	}
	workerCerts, err := m.loadLeafCertificates(ctx, m.workerPrefix())
	if err != nil {
		return CAState{}, fmt.Errorf("deploy: load worker certs: %w", err)
	}

	revokedResp, err := m.client.Get(ctx, m.revokedKey())
	if err != nil {
		return CAState{}, fmt.Errorf("deploy: read revoked list: %w", err)
	}
	revoked := make([]RevokedRecord, 0)
	if len(revokedResp.Kvs) > 0 && len(revokedResp.Kvs[0].Value) > 0 {
		if err := json.Unmarshal(revokedResp.Kvs[0].Value, &revoked); err != nil {
			return CAState{}, fmt.Errorf("deploy: decode revoked list: %w", err)
		}
	}

	sort.Strings(nodes.Beacons)
	sort.Strings(nodes.Workers)

	return CAState{
		ClusterID:          m.clusterID,
		Nodes:              nodes,
		CurrentCA:          current,
		BeaconCertificates: beaconCerts,
		WorkerCertificates: workerCerts,
		Revoked:            revoked,
		Revision:           resp.Kvs[0].ModRevision,
		NodesRevision:      nodesRevision,
	}, nil
}

func (m *CARotationManager) loadLeafCertificates(ctx context.Context, prefix string) (map[string]LeafCertificate, error) {
	resp, err := m.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	certs := make(map[string]LeafCertificate, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var cert LeafCertificate
		if err := json.Unmarshal(kv.Value, &cert); err != nil {
			return nil, err
		}
		certs[cert.NodeID] = cert
	}
	return certs, nil
}
