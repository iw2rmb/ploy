package deploy

import (
    "context"
    "crypto/ecdsa"
    "crypto/elliptic"
    "crypto/rand"
    "crypto/x509"
    "crypto/x509/pkix"
    "encoding/hex"
    "encoding/json"
    "encoding/pem"
    "errors"
    "fmt"
    "math/big"
    "sort"
    "strings"
    "time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/controlplane/security"
)

const (
	defaultCAValidity    = 365 * 24 * time.Hour
	defaultLeafValidity  = 90 * 24 * time.Hour
	certSerialBitSize    = 160
	clusterSecurityRoot  = "/ploy/clusters"
	beaconCertificateUse = "beacon"
	workerCertificateUse = "worker"
)

var (
	// ErrPKINotBootstrapped indicates the cluster security materials have not been generated yet.
	ErrPKINotBootstrapped = errors.New("deploy: cluster PKI not bootstrapped")
	// ErrPKIAlreadyBootstrapped indicates bootstrap was attempted when materials already exist.
	ErrPKIAlreadyBootstrapped = errors.New("deploy: cluster PKI already bootstrapped")
	// ErrConcurrentRotation signals the CA changed while processing a rotation.
	ErrConcurrentRotation = errors.New("deploy: concurrent CA rotation detected")
	// ErrWorkerExists indicates the worker has already been registered.
	ErrWorkerExists = errors.New("deploy: worker already registered")
	// ErrWorkerNotFound indicates the worker is missing from the inventory.
	ErrWorkerNotFound = errors.New("deploy: worker not registered")
	// ErrConcurrentWorkerUpdate signals a concurrent modification to the worker inventory.
	ErrConcurrentWorkerUpdate = errors.New("deploy: concurrent worker update detected")
)

// CARotationManager manages certificate authority lifecycle for a cluster.
type CARotationManager struct {
	client    *clientv3.Client
	clusterID string
	prefix    string
	trust     *security.TrustStore
}

// BootstrapOptions configure the initial PKI generation.
type BootstrapOptions struct {
	BeaconIDs    []string
	WorkerIDs    []string
	CAValidity   time.Duration
	LeafValidity time.Duration
	RequestedAt  time.Time
}

// RotateOptions configure a CA rotation request.
type RotateOptions struct {
	DryRun      bool
	RequestedAt time.Time
	Operator    string
	Reason      string
}

// CAState describes the persisted CA state for a cluster.
type CAState struct {
	ClusterID          string
	Nodes              NodeSet
	CurrentCA          CABundle
	BeaconCertificates map[string]LeafCertificate
	WorkerCertificates map[string]LeafCertificate
	Revoked            []RevokedRecord
	Revision           int64
	NodesRevision      int64
}

// CABundle captures certificate authority metadata and PEM-encoded material.
type CABundle struct {
	Version        string    `json:"version"`
	SerialNumber   string    `json:"serial_number"`
	Subject        string    `json:"subject"`
	IssuedAt       time.Time `json:"issued_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	CertificatePEM string    `json:"certificate_pem"`
	KeyPEM         string    `json:"key_pem"`
	Revoked        bool      `json:"revoked,omitempty"`
	RevokedAt      time.Time `json:"revoked_at,omitempty"`
}

// LeafCertificate describes node certificates issued by the CA.
type LeafCertificate struct {
	NodeID          string    `json:"node_id"`
	Usage           string    `json:"usage"`
	Version         string    `json:"version"`
	ParentVersion   string    `json:"parent_version"`
	SerialNumber    string    `json:"serial_number"`
	CertificatePEM  string    `json:"certificate_pem"`
	KeyPEM          string    `json:"key_pem"`
	IssuedAt        time.Time `json:"issued_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	PreviousVersion string    `json:"previous_version,omitempty"`
}

// RevokedRecord records a revoked CA version.
type RevokedRecord struct {
	Version   string    `json:"version"`
	Serial    string    `json:"serial_number"`
	RevokedAt time.Time `json:"revoked_at"`
}

// RotateResult summarises a rotation request.
type RotateResult struct {
	DryRun                    bool
	OldVersion                string
	NewVersion                string
	UpdatedBeaconCertificates []LeafCertificate
	UpdatedWorkerCertificates []LeafCertificate
	Revoked                   RevokedRecord
	State                     CAState
	Operator                  string
	Reason                    string
}

// NodeSet lists tracked beacon and worker identifiers.
type NodeSet struct {
	Beacons []string `json:"beacons"`
	Workers []string `json:"workers"`
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

// Bootstrap generates the initial CA and node certificates for a cluster.
func (m *CARotationManager) Bootstrap(ctx context.Context, opts BootstrapOptions) (CAState, error) {
	beacons := normalizeNodeIDs(opts.BeaconIDs)
	if len(beacons) == 0 {
		return CAState{}, errors.New("deploy: at least one beacon id required")
	}
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
		cert, err := issueLeafCertificate(id, beaconCertificateUse, caBundle, caCert, caKey, now, leafValidity, "")
		if err != nil {
			return CAState{}, err
		}
		beaconCerts[id] = cert
	}
	for _, id := range workers {
		cert, err := issueLeafCertificate(id, workerCertificateUse, caBundle, caCert, caKey, now, leafValidity, "")
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
		cert, err := issueLeafCertificate(id, beaconCertificateUse, newCABundle, newCACert, newCAKey, now, defaultLeafValidity, state.BeaconCertificates[id].Version)
		if err != nil {
			return RotateResult{}, err
		}
		beaconUpdates = append(beaconUpdates, cert)
	}
	workerUpdates := make([]LeafCertificate, 0, len(state.Nodes.Workers))
	for _, id := range state.Nodes.Workers {
		prev := state.WorkerCertificates[id]
		cert, err := issueLeafCertificate(id, workerCertificateUse, newCABundle, newCACert, newCAKey, now, defaultLeafValidity, prev.Version)
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
	cert, err := issueLeafCertificate(id, workerCertificateUse, state.CurrentCA, caCert, caKey, now, defaultLeafValidity, "")
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

func decodeCABundleMaterials(bundle CABundle) (*ecdsa.PrivateKey, *x509.Certificate, error) {
	keyBlock, _ := pem.Decode([]byte(bundle.KeyPEM))
	if keyBlock == nil {
		return nil, nil, errors.New("deploy: decode CA private key: missing PEM block")
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("deploy: parse CA private key: %w", err)
	}
	privateKey, ok := keyAny.(*ecdsa.PrivateKey)
	if !ok {
		return nil, nil, errors.New("deploy: CA private key must be ecdsa")
	}
	certBlock, _ := pem.Decode([]byte(bundle.CertificatePEM))
	if certBlock == nil {
		return nil, nil, errors.New("deploy: decode CA certificate: missing PEM block")
	}
	caCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("deploy: parse CA certificate: %w", err)
	}
	return privateKey, caCert, nil
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

	func generateCABundle(clusterID string, now time.Time, validity time.Duration) (CABundle, *ecdsa.PrivateKey, *x509.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return CABundle{}, nil, nil, fmt.Errorf("deploy: generate CA key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return CABundle{}, nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:         fmt.Sprintf("ploy-%s-root", clusterID),
			Organization:       []string{"Ploy Deployment"},
			OrganizationalUnit: []string{"Control Plane"},
		},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(validity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
		MaxPathLenZero:        false,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return CABundle{}, nil, nil, fmt.Errorf("deploy: create CA certificate: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return CABundle{}, nil, nil, fmt.Errorf("deploy: marshal CA private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	version := buildVersion(now)
	return CABundle{
		Version:        version,
		SerialNumber:   serial.Text(16),
		Subject:        template.Subject.String(),
		IssuedAt:       now,
		ExpiresAt:      template.NotAfter,
		CertificatePEM: string(certPEM),
		KeyPEM:         string(keyPEM),
	}, priv, template, nil
}

func issueLeafCertificate(nodeID, usage string, ca CABundle, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, now time.Time, validity time.Duration, previousVersion string) (LeafCertificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return LeafCertificate{}, fmt.Errorf("deploy: generate %s key for %s: %w", usage, nodeID, err)
	}
	serial, err := randomSerial()
	if err != nil {
		return LeafCertificate{}, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:         fmt.Sprintf("%s-%s", usage, nodeID),
			Organization:       []string{"Ploy Deployment"},
			OrganizationalUnit: []string{"Ploy " + usage},
		},
		NotBefore: now.Add(-1 * time.Minute),
		NotAfter:  now.Add(validity),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &priv.PublicKey, caKey)
	if err != nil {
		return LeafCertificate{}, fmt.Errorf("deploy: create %s certificate for %s: %w", usage, nodeID, err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return LeafCertificate{}, fmt.Errorf("deploy: marshal %s private key for %s: %w", usage, nodeID, err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return LeafCertificate{
		NodeID:          nodeID,
		Usage:           usage,
		Version:         buildVersion(now),
		ParentVersion:   ca.Version,
		SerialNumber:    hex.EncodeToString(serial.Bytes()),
		CertificatePEM:  string(certPEM),
		KeyPEM:          string(keyPEM),
		IssuedAt:        now,
		ExpiresAt:       now.Add(validity),
		PreviousVersion: previousVersion,
	}, nil
}

func randomSerial() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), certSerialBitSize)
	serial, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("deploy: generate serial number: %w", err)
	}
	if serial.Sign() == 0 {
		return randomSerial()
	}
	return serial, nil
}

func buildVersion(now time.Time) string {
	ts := now.UTC().Format("20060102T150405Z")
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		return ts
	}
	return fmt.Sprintf("%s-%s", ts, hex.EncodeToString(randomBytes))
}
