package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// IssueWorkerCertificate adds the worker to the cluster inventory and issues a leaf certificate.
func (m *CARotationManager) IssueWorkerCertificate(ctx context.Context, workerID string, now time.Time) (LeafCertificate, error) {
	id := strings.TrimSpace(workerID)
	if id == "" {
		return LeafCertificate{}, errors.New("deploy: worker id required")
	}
	state, err := m.State(ctx)
	if err != nil {
		return LeafCertificate{}, err
	}
	for _, existing := range state.Nodes.Workers {
		if existing == id {
			return LeafCertificate{}, ErrWorkerExists
		}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	caKey, caCert, err := decodeCABundleMaterials(state.CurrentCA)
	if err != nil {
		return LeafCertificate{}, err
	}
	cert, err := issueLeafCertificate(id, certificateRoleWorker, state.CurrentCA, caCert, caKey, now, defaultLeafValidity, "", nil)
	if err != nil {
		return LeafCertificate{}, err
	}

	updatedWorkers := append([]string(nil), state.Nodes.Workers...)
	updatedWorkers = append(updatedWorkers, id)
	sort.Strings(updatedWorkers)
	nodes := NodeSet{
		Beacons: append([]string(nil), state.Nodes.Beacons...),
		Workers: updatedWorkers,
	}
	nodesBytes, err := json.Marshal(nodes)
	if err != nil {
		return LeafCertificate{}, fmt.Errorf("deploy: encode worker nodes: %w", err)
	}
	certBytes, err := json.Marshal(cert)
	if err != nil {
		return LeafCertificate{}, fmt.Errorf("deploy: encode worker certificate: %w", err)
	}

	txn := m.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(m.caCurrentKey()), "=", state.Revision),
		clientv3.Compare(clientv3.ModRevision(m.nodesKey()), "=", state.NodesRevision),
		clientv3.Compare(clientv3.CreateRevision(m.workerCertKey(id)), "=", 0),
	).Then(
		clientv3.OpPut(m.nodesKey(), string(nodesBytes)),
		clientv3.OpPut(m.workerCertKey(id), string(certBytes)),
	)
	resp, err := txn.Commit()
	if err != nil {
		return LeafCertificate{}, fmt.Errorf("deploy: persist worker certificate: %w", err)
	}
	if !resp.Succeeded {
		return LeafCertificate{}, ErrConcurrentWorkerUpdate
	}
	return cert, nil
}

// IssueControlPlaneCertificate issues or refreshes the control-plane node certificate.
func (m *CARotationManager) IssueControlPlaneCertificate(ctx context.Context, nodeID, address string, now time.Time) (LeafCertificate, error) {
	id := strings.TrimSpace(nodeID)
	if id == "" {
		return LeafCertificate{}, errors.New("deploy: control-plane node id required")
	}
	addr := strings.TrimSpace(address)
	if addr == "" {
		return LeafCertificate{}, errors.New("deploy: control-plane node address required")
	}
	state, err := m.State(ctx)
	if err != nil {
		return LeafCertificate{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	caKey, caCert, err := decodeCABundleMaterials(state.CurrentCA)
	if err != nil {
		return LeafCertificate{}, err
	}
	var profile leafProfile
	if ip := net.ParseIP(addr); ip != nil {
		profile.IPAddresses = []net.IP{ip}
	} else {
		profile.DNSNames = []string{addr}
	}
	prevVersion := ""
	if existing, ok := state.BeaconCertificates[id]; ok {
		prevVersion = existing.Version
	}
	cert, err := issueLeafCertificate(id, certificateRoleControlPlane, state.CurrentCA, caCert, caKey, now, defaultLeafValidity, prevVersion, &profile)
	if err != nil {
		return LeafCertificate{}, err
	}

	updatedBeacons := append([]string(nil), state.Nodes.Beacons...)
	found := false
	for _, existing := range updatedBeacons {
		if existing == id {
			found = true
			break
		}
	}
	if !found {
		updatedBeacons = append(updatedBeacons, id)
	}
	sort.Strings(updatedBeacons)
	nodes := NodeSet{
		Beacons: updatedBeacons,
		Workers: append([]string(nil), state.Nodes.Workers...),
	}
	nodesBytes, err := json.Marshal(nodes)
	if err != nil {
		return LeafCertificate{}, fmt.Errorf("deploy: encode control-plane nodes: %w", err)
	}
	certBytes, err := json.Marshal(cert)
	if err != nil {
		return LeafCertificate{}, fmt.Errorf("deploy: encode control-plane certificate: %w", err)
	}
	txn := m.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(m.caCurrentKey()), "=", state.Revision),
		clientv3.Compare(clientv3.ModRevision(m.nodesKey()), "=", state.NodesRevision),
	).Then(
		clientv3.OpPut(m.nodesKey(), string(nodesBytes)),
		clientv3.OpPut(m.beaconCertKey(id), string(certBytes)),
	)
	resp, err := txn.Commit()
	if err != nil {
		return LeafCertificate{}, fmt.Errorf("deploy: persist control-plane certificate: %w", err)
	}
	if !resp.Succeeded {
		return LeafCertificate{}, ErrConcurrentWorkerUpdate
	}
	return cert, nil
}

// RemoveWorker removes the worker from the inventory and deletes certificate materials.
func (m *CARotationManager) RemoveWorker(ctx context.Context, workerID string) error {
	id := strings.TrimSpace(workerID)
	if id == "" {
		return errors.New("deploy: worker id required")
	}
	state, err := m.State(ctx)
	if err != nil {
		return err
	}
	index := -1
	for i, existing := range state.Nodes.Workers {
		if existing == id {
			index = i
			break
		}
	}
	if index == -1 {
		return ErrWorkerNotFound
	}
	updatedWorkers := append([]string(nil), state.Nodes.Workers...)
	updatedWorkers = append(updatedWorkers[:index], updatedWorkers[index+1:]...)
	nodes := NodeSet{
		Beacons: append([]string(nil), state.Nodes.Beacons...),
		Workers: updatedWorkers,
	}
	nodesBytes, err := json.Marshal(nodes)
	if err != nil {
		return fmt.Errorf("deploy: encode worker nodes: %w", err)
	}

	txn := m.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(m.caCurrentKey()), "=", state.Revision),
		clientv3.Compare(clientv3.ModRevision(m.nodesKey()), "=", state.NodesRevision),
		clientv3.Compare(clientv3.CreateRevision(m.workerCertKey(id)), ">", 0),
	).Then(
		clientv3.OpPut(m.nodesKey(), string(nodesBytes)),
		clientv3.OpDelete(m.workerCertKey(id)),
	)
	resp, err := txn.Commit()
	if err != nil {
		return fmt.Errorf("deploy: remove worker: %w", err)
	}
	if !resp.Succeeded {
		return ErrConcurrentWorkerUpdate
	}
	return nil
}
