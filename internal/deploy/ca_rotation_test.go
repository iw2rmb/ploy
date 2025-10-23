package deploy

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"testing"
	"time"

	"crypto/x509"
	"encoding/pem"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/iw2rmb/ploy/internal/controlplane/security"
)

func TestCARotation(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	manager := mustNewCARotationManager(t, client, "cluster-alpha")

	bootstrapState, err := manager.Bootstrap(ctx, BootstrapOptions{
		BeaconIDs: []string{"beacon-main"},
		WorkerIDs: []string{"worker-a", "worker-b"},
	})
	if err != nil {
		t.Fatalf("Bootstrap returned error: %v", err)
	}

	if bootstrapState.CurrentCA.Version == "" {
		t.Fatalf("expected bootstrap to generate CA version")
	}
	wantCASubject := fmt.Sprintf("CN=ploy-%s-root,OU=Control Plane,O=Ploy Deployment", "cluster-alpha")
	if bootstrapState.CurrentCA.Subject != wantCASubject {
		t.Fatalf("expected CA subject %q, got %q", wantCASubject, bootstrapState.CurrentCA.Subject)
	}
	if !slices.Equal(bootstrapState.Nodes.Beacons, []string{"beacon-main"}) {
		t.Fatalf("expected beacon inventory to persist, got %v", bootstrapState.Nodes.Beacons)
	}
	if len(bootstrapState.BeaconCertificates) != 1 {
		t.Fatalf("expected one beacon certificate, got %d", len(bootstrapState.BeaconCertificates))
	}
	beaconCert := bootstrapState.BeaconCertificates["beacon-main"]
	if beaconCert.ParentVersion != bootstrapState.CurrentCA.Version {
		t.Fatalf("expected beacon cert parent %s, got %s", bootstrapState.CurrentCA.Version, beaconCert.ParentVersion)
	}
	assertLeafSubject(t, beaconCert.CertificatePEM, "beacon-beacon-main", wantCASubject)
	if len(bootstrapState.WorkerCertificates) != 2 {
		t.Fatalf("expected worker certificates to be issued, got %d", len(bootstrapState.WorkerCertificates))
	}
	for id, cert := range bootstrapState.WorkerCertificates {
		assertLeafSubject(t, cert.CertificatePEM, "worker-"+id, wantCASubject)
	}

	store := mustNewTrustStore(t, client, "cluster-alpha")
	bundle, _, err := store.Current(ctx)
	if err != nil {
		t.Fatalf("trust store current bundle: %v", err)
	}
	if bundle.Version != bootstrapState.CurrentCA.Version {
		t.Fatalf("expected trust store to record CA version %s, got %s", bootstrapState.CurrentCA.Version, bundle.Version)
	}

	dryRunResult, err := manager.Rotate(ctx, RotateOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Rotate dry-run returned error: %v", err)
	}
	if !dryRunResult.DryRun {
		t.Fatalf("expected dry-run result to set DryRun flag")
	}
	stateAfterDryRun, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("State after dry-run rotation: %v", err)
	}
	if stateAfterDryRun.CurrentCA.Version != bootstrapState.CurrentCA.Version {
		t.Fatalf("expected dry-run to keep CA version %s, got %s", bootstrapState.CurrentCA.Version, stateAfterDryRun.CurrentCA.Version)
	}

	start := time.Now().UTC()
	rotationResult, err := manager.Rotate(ctx, RotateOptions{RequestedAt: start})
	if err != nil {
		t.Fatalf("Rotate returned error: %v", err)
	}
	if rotationResult.DryRun {
		t.Fatalf("expected actual rotation to clear DryRun flag")
	}
	if rotationResult.OldVersion != bootstrapState.CurrentCA.Version {
		t.Fatalf("expected old version %s, got %s", bootstrapState.CurrentCA.Version, rotationResult.OldVersion)
	}
	if rotationResult.NewVersion == rotationResult.OldVersion {
		t.Fatalf("expected new version to differ from old")
	}

	finalState, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("State after rotation: %v", err)
	}
	if finalState.CurrentCA.Version != rotationResult.NewVersion {
		t.Fatalf("expected current CA version %s, got %s", rotationResult.NewVersion, finalState.CurrentCA.Version)
	}
	if len(finalState.Revoked) == 0 {
		t.Fatalf("expected revoked CA entries recorded")
	}
	if finalState.Revoked[len(finalState.Revoked)-1].Version != rotationResult.OldVersion {
		t.Fatalf("expected revoked record for %s, got %+v", rotationResult.OldVersion, finalState.Revoked[len(finalState.Revoked)-1])
	}

	bundle, _, err = store.Current(ctx)
	if err != nil {
		t.Fatalf("trust store updated bundle: %v", err)
	}
	if bundle.Version != rotationResult.NewVersion {
		t.Fatalf("expected trust store to update to %s, got %s", rotationResult.NewVersion, bundle.Version)
	}

	for nodeID, cert := range finalState.WorkerCertificates {
		if cert.ParentVersion != rotationResult.NewVersion {
			t.Fatalf("expected worker %s to trust CA %s, got %s", nodeID, rotationResult.NewVersion, cert.ParentVersion)
		}
		previous := bootstrapState.WorkerCertificates[nodeID]
		if cert.PreviousVersion != previous.Version {
			t.Fatalf("expected worker %s previous version %s, got %s", nodeID, previous.Version, cert.PreviousVersion)
		}
	}

	oldHistoryKey := fmt.Sprintf("/ploy/clusters/cluster-alpha/security/ca/history/%s", rotationResult.OldVersion)
	historyResp, err := client.Get(ctx, oldHistoryKey)
	if err != nil {
		t.Fatalf("read CA history: %v", err)
	}
	if len(historyResp.Kvs) != 1 {
		t.Fatalf("expected CA history record for %s", rotationResult.OldVersion)
	}
}

func mustNewCARotationManager(t *testing.T, client *clientv3.Client, clusterID string) *CARotationManager {
	t.Helper()
	manager, err := NewCARotationManager(client, clusterID)
	if err != nil {
		t.Fatalf("NewCARotationManager(%s): %v", clusterID, err)
	}
	return manager
}

func mustNewTrustStore(t *testing.T, client *clientv3.Client, clusterID string) *security.TrustStore {
	t.Helper()
	store, err := security.NewTrustStore(client, clusterID)
	if err != nil {
		t.Fatalf("NewTrustStore(%s): %v", clusterID, err)
	}
	return store
}

func newTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
	t.Helper()
	dir := t.TempDir()
	cfg := embed.NewConfig()
	cfg.Dir = dir
	clientURL := mustParseURL("http://127.0.0.1:0")
	peerURL := mustParseURL("http://127.0.0.1:0")
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.Name = "default"
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "deploy-ca-rotation-test"
	cfg.LogLevel = "panic"
	cfg.Logger = "zap"
	cfg.LogOutputs = []string{fmt.Sprintf("%s/etcd.log", dir)}

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start etcd: %v", err)
	}
	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		e.Server.Stop()
		t.Fatalf("etcd start timeout")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{e.Clients[0].Addr().String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		e.Close()
		t.Fatalf("client: %v", err)
	}

	return e, client
}

func mustParseURL(raw string) url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return *parsed
}

func assertLeafSubject(t *testing.T, pemData string, wantCN string, wantIssuer string) {
	t.Helper()
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		t.Fatalf("expected PEM block for certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	if cert.Subject.CommonName != wantCN {
		t.Fatalf("expected certificate CN %q, got %q", wantCN, cert.Subject.CommonName)
	}
	if cert.Issuer.String() != wantIssuer {
		t.Fatalf("expected certificate issuer %q, got %q", wantIssuer, cert.Issuer.String())
	}
}
