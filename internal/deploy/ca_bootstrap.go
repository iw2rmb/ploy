//go:build legacy
// +build legacy

package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/controlplane/security"
)

// Bootstrap generates the initial CA and node certificates for a cluster.
func (m *CARotationManager) Bootstrap(ctx context.Context, opts BootstrapOptions) (CAState, error) {
	beacons := normalizeNodeIDs(opts.BeaconIDs)
	workers := normalizeNodeIDs(opts.WorkerIDs)

	now := opts.RequestedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	caValidity := opts.CAValidity
	if caValidity == 0 {
		caValidity = defaultCAValidity
	}
	leafValidity := opts.LeafValidity
	if leafValidity == 0 {
		leafValidity = defaultLeafValidity
	}

	currentResp, err := m.client.Get(ctx, m.caCurrentKey())
	if err != nil {
		return CAState{}, fmt.Errorf("deploy: check existing CA: %w", err)
	}
	if len(currentResp.Kvs) > 0 {
		return CAState{}, ErrPKIAlreadyBootstrapped
	}

	caBundle, caKey, caCert, err := generateCABundle(m.clusterID, now, caValidity)
	if err != nil {
		return CAState{}, err
	}
	beaconCerts := make(map[string]LeafCertificate, len(beacons))
	workerCerts := make(map[string]LeafCertificate, len(workers))

	for _, id := range beacons {
		cert, err := issueLeafCertificate(id, certificateRoleControlPlane, caBundle, caCert, caKey, now, leafValidity, "", nil)
		if err != nil {
			return CAState{}, err
		}
		beaconCerts[id] = cert
	}
	for _, id := range workers {
		cert, err := issueLeafCertificate(id, certificateRoleWorker, caBundle, caCert, caKey, now, leafValidity, "", nil)
		if err != nil {
			return CAState{}, err
		}
		workerCerts[id] = cert
	}

	nodes := NodeSet{Beacons: beacons, Workers: workers}
	nodesBytes, err := json.Marshal(nodes)
	if err != nil {
		return CAState{}, fmt.Errorf("deploy: encode node inventory: %w", err)
	}
	caBytes, err := json.Marshal(caBundle)
	if err != nil {
		return CAState{}, fmt.Errorf("deploy: encode CA bundle: %w", err)
	}

	ops := []clientv3.Op{
		clientv3.OpPut(m.nodesKey(), string(nodesBytes)),
		clientv3.OpPut(m.caCurrentKey(), string(caBytes)),
		clientv3.OpPut(m.revokedKey(), "[]"),
	}
	for id, cert := range beaconCerts {
		value, err := json.Marshal(cert)
		if err != nil {
			return CAState{}, fmt.Errorf("deploy: encode beacon cert %s: %w", id, err)
		}
		ops = append(ops, clientv3.OpPut(m.beaconCertKey(id), string(value)))
	}
	for id, cert := range workerCerts {
		value, err := json.Marshal(cert)
		if err != nil {
			return CAState{}, fmt.Errorf("deploy: encode worker cert %s: %w", id, err)
		}
		ops = append(ops, clientv3.OpPut(m.workerCertKey(id), string(value)))
	}

	txn := m.client.Txn(ctx).If(
		clientv3.Compare(clientv3.CreateRevision(m.caCurrentKey()), "=", 0),
	).Then(ops...)

	resp, err := txn.Commit()
	if err != nil {
		return CAState{}, fmt.Errorf("deploy: persist bootstrap: %w", err)
	}
	if !resp.Succeeded {
		return CAState{}, ErrPKIAlreadyBootstrapped
	}

	if err := m.trust.Update(ctx, security.TrustBundle{
		Version:     caBundle.Version,
		UpdatedAt:   now,
		CABundlePEM: caBundle.CertificatePEM,
	}); err != nil {
		return CAState{}, err
	}

	return m.State(ctx)
}

func normalizeNodeIDs(ids []string) []string {
	seen := make(map[string]struct{})
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		seen[trimmed] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(seen))
	for id := range seen {
		normalized = append(normalized, id)
	}
	sort.Strings(normalized)
	return normalized
}
