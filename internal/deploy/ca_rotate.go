package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/controlplane/security"
)

// Rotate replaces the active CA and reissues node certificates.
func (m *CARotationManager) Rotate(ctx context.Context, opts RotateOptions) (RotateResult, error) {
	state, err := m.State(ctx)
	if err != nil {
		return RotateResult{}, err
	}

	now := opts.RequestedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	newCABundle, newCAKey, newCACert, err := generateCABundle(m.clusterID, now, defaultCAValidity)
	if err != nil {
		return RotateResult{}, err
	}

	beaconUpdates := make([]LeafCertificate, 0, len(state.Nodes.Beacons))
	for _, id := range state.Nodes.Beacons {
		cert, err := issueLeafCertificate(id, certificateRoleControlPlane, newCABundle, newCACert, newCAKey, now, defaultLeafValidity, state.BeaconCertificates[id].Version, nil)
		if err != nil {
			return RotateResult{}, err
		}
		beaconUpdates = append(beaconUpdates, cert)
	}
	workerUpdates := make([]LeafCertificate, 0, len(state.Nodes.Workers))
	for _, id := range state.Nodes.Workers {
		prev := state.WorkerCertificates[id]
		cert, err := issueLeafCertificate(id, certificateRoleWorker, newCABundle, newCACert, newCAKey, now, defaultLeafValidity, prev.Version, nil)
		if err != nil {
			return RotateResult{}, err
		}
		workerUpdates = append(workerUpdates, cert)
	}
	sort.Slice(beaconUpdates, func(i, j int) bool { return beaconUpdates[i].NodeID < beaconUpdates[j].NodeID })
	sort.Slice(workerUpdates, func(i, j int) bool { return workerUpdates[i].NodeID < workerUpdates[j].NodeID })

	revokedRecord := RevokedRecord{
		Version:   state.CurrentCA.Version,
		Serial:    state.CurrentCA.SerialNumber,
		RevokedAt: now,
	}

	result := RotateResult{
		DryRun:                    opts.DryRun,
		OldVersion:                state.CurrentCA.Version,
		NewVersion:                newCABundle.Version,
		UpdatedBeaconCertificates: beaconUpdates,
		UpdatedWorkerCertificates: workerUpdates,
		Revoked:                   revokedRecord,
		State:                     state,
		Operator:                  opts.Operator,
		Reason:                    opts.Reason,
	}

	if opts.DryRun {
		return result, nil
	}

	revokedList := append(state.Revoked[:len(state.Revoked):len(state.Revoked)], revokedRecord)
	historyBundle := state.CurrentCA
	historyBundle.Revoked = true
	historyBundle.RevokedAt = now
	historyBytes, err := json.Marshal(historyBundle)
	if err != nil {
		return RotateResult{}, fmt.Errorf("deploy: encode revoked CA: %w", err)
	}

	newCABytes, err := json.Marshal(newCABundle)
	if err != nil {
		return RotateResult{}, fmt.Errorf("deploy: encode new CA bundle: %w", err)
	}
	revokedBytes, err := json.Marshal(revokedList)
	if err != nil {
		return RotateResult{}, fmt.Errorf("deploy: encode revoked list: %w", err)
	}

	ops := []clientv3.Op{
		clientv3.OpPut(m.caCurrentKey(), string(newCABytes)),
		clientv3.OpPut(m.caHistoryKey(state.CurrentCA.Version), string(historyBytes)),
		clientv3.OpPut(m.revokedKey(), string(revokedBytes)),
	}
	for _, cert := range beaconUpdates {
		payload, err := json.Marshal(cert)
		if err != nil {
			return RotateResult{}, fmt.Errorf("deploy: encode beacon cert %s: %w", cert.NodeID, err)
		}
		ops = append(ops, clientv3.OpPut(m.beaconCertKey(cert.NodeID), string(payload)))
	}
	for _, cert := range workerUpdates {
		payload, err := json.Marshal(cert)
		if err != nil {
			return RotateResult{}, fmt.Errorf("deploy: encode worker cert %s: %w", cert.NodeID, err)
		}
		ops = append(ops, clientv3.OpPut(m.workerCertKey(cert.NodeID), string(payload)))
	}

	txn := m.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(m.caCurrentKey()), "=", state.Revision),
	).Then(ops...)

	resp, err := txn.Commit()
	if err != nil {
		return RotateResult{}, fmt.Errorf("deploy: persist rotation: %w", err)
	}
	if !resp.Succeeded {
		return RotateResult{}, ErrConcurrentRotation
	}

	if err := m.trust.Update(ctx, security.TrustBundle{
		Version:     newCABundle.Version,
		UpdatedAt:   now,
		CABundlePEM: newCABundle.CertificatePEM,
	}); err != nil {
		return RotateResult{}, err
	}

	newState, err := m.State(ctx)
	if err != nil {
		return RotateResult{}, err
	}
	result.State = newState
	result.NewVersion = newState.CurrentCA.Version
	return result, nil
}
