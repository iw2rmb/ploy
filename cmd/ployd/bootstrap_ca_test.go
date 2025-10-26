package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/iw2rmb/ploy/internal/deploy"
)

func TestRunBootstrapCABootstrapsCluster(t *testing.T) {
	ctx := context.Background()
	etcd, client, endpoint := newBootstrapTestEtcd(t)
	t.Cleanup(func() {
		etcd.Close()
		_ = client.Close()
	})

	t.Setenv("PLOY_ETCD_ENDPOINTS", endpoint)

	temp := t.TempDir()
	caPath := filepath.Join(temp, "control-plane-ca.pem")
	certPath := filepath.Join(temp, "node.pem")
	keyPath := filepath.Join(temp, "node-key.pem")

	args := []string{
		"--cluster-id", "cluster-alpha",
		"--node-id", "control-alpha",
		"--address", "10.42.0.5",
		"--ca", caPath,
		"--cert", certPath,
		"--key", keyPath,
	}
	if err := runBootstrapCA(args); err != nil {
		t.Fatalf("runBootstrapCA: %v", err)
	}

	caPEM := readFileTrimmed(t, caPath)
	certPEM := readFileTrimmed(t, certPath)
	keyPEM := readFileTrimmed(t, keyPath)
	if !strings.Contains(caPEM, "BEGIN CERTIFICATE") {
		t.Fatalf("expected CA PEM, got %q", caPEM)
	}
	if !strings.Contains(certPEM, "BEGIN CERTIFICATE") {
		t.Fatalf("expected node certificate, got %q", certPEM)
	}
	if !strings.Contains(keyPEM, "PRIVATE KEY") {
		t.Fatalf("expected node key, got %q", keyPEM)
	}

	manager, err := deploy.NewCARotationManager(client, "cluster-alpha")
	if err != nil {
		t.Fatalf("NewCARotationManager: %v", err)
	}
	state, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if got := strings.TrimSpace(state.CurrentCA.CertificatePEM); got != caPEM {
		t.Fatalf("expected CA bundle persisted, mismatch")
	}
	if len(state.Nodes.Beacons) != 1 || state.Nodes.Beacons[0] != "control-alpha" {
		t.Fatalf("expected control-alpha beacon registered, got %+v", state.Nodes.Beacons)
	}
	if _, ok := state.BeaconCertificates["control-alpha"]; !ok {
		t.Fatalf("expected control-alpha certificate recorded")
	}
}

func TestRunBootstrapCAReusesExistingCluster(t *testing.T) {
	ctx := context.Background()
	etcd, client, endpoint := newBootstrapTestEtcd(t)
	t.Cleanup(func() {
		etcd.Close()
		_ = client.Close()
	})

	t.Setenv("PLOY_ETCD_ENDPOINTS", endpoint)

	temp := t.TempDir()
	caPath := filepath.Join(temp, "ca.pem")
	certPath := filepath.Join(temp, "cert.pem")
	keyPath := filepath.Join(temp, "key.pem")
	args := []string{
		"--cluster-id", "cluster-beta",
		"--node-id", "control",
		"--address", "cp.beta.local",
		"--ca", caPath,
		"--cert", certPath,
		"--key", keyPath,
	}

	if err := runBootstrapCA(args); err != nil {
		t.Fatalf("first run: %v", err)
	}
	manager, err := deploy.NewCARotationManager(client, "cluster-beta")
	if err != nil {
		t.Fatalf("NewCARotationManager: %v", err)
	}
	initialState, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("State initial: %v", err)
	}
	initialCAVersion := initialState.CurrentCA.Version
	initialCert, ok := initialState.BeaconCertificates["control"]
	if !ok {
		t.Fatalf("expected control certificate present after first run")
	}
	if initialCert.PreviousVersion != "" {
		t.Fatalf("expected no previous version on first issuance")
	}

	// Second invocation should reuse the CA and reissue the control-plane cert.
	if err := runBootstrapCA(args); err != nil {
		t.Fatalf("second run: %v", err)
	}
	finalState, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("State final: %v", err)
	}
	if finalState.CurrentCA.Version != initialCAVersion {
		t.Fatalf("expected CA version to stay %q, got %q", initialCAVersion, finalState.CurrentCA.Version)
	}
	finalCert, ok := finalState.BeaconCertificates["control"]
	if !ok {
		t.Fatalf("expected control certificate present after second run")
	}
	if finalCert.PreviousVersion != initialCert.Version {
		t.Fatalf("expected previous version %q, got %q", initialCert.Version, finalCert.PreviousVersion)
	}
	if strings.TrimSpace(readFileTrimmed(t, caPath)) == "" {
		t.Fatalf("expected CA file to exist after second run")
	}
}

func newBootstrapTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client, string) {
	t.Helper()
	cfg := embed.NewConfig()
	cfg.Dir = t.TempDir()
	cfg.LogLevel = "panic"
	cfg.Logger = "zap"
	cfg.LogOutputs = []string{filepath.Join(cfg.Dir, "etcd.log")}
	clientURL := mustParseURL(t, "http://127.0.0.1:0")
	peerURL := mustParseURL(t, "http://127.0.0.1:0")
	cfg.Name = "bootstrap-ca-test"
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.InitialClusterToken = "bootstrap-ca-test"
	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start embed etcd: %v", err)
	}
	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		e.Server.Stop()
		t.Fatalf("etcd start timeout")
	}
	endpoint := e.Clients[0].Addr().String()
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		e.Close()
		t.Fatalf("etcd client: %v", err)
	}
	return e, client, endpoint
}

func readFileTrimmed(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.TrimSpace(string(data))
}

func mustParseURL(t *testing.T, raw string) url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url %q: %v", raw, err)
	}
	return *parsed
}
