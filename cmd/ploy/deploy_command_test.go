package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"testing"
	"time"

	"go.etcd.io/etcd/server/v3/embed"

	"github.com/iw2rmb/ploy/internal/deploy"
)

func TestHandleDeployBootstrapRequiresClusterID(t *testing.T) {
	origRunner := deployBootstrapRunner
	defer func() { deployBootstrapRunner = origRunner }()

	deployBootstrapRunner = func(context.Context, deploy.Options) error {
		return errors.New("should not be called")
	}

	err := handleDeployBootstrap([]string{
		"--etcd-endpoints", "http://127.0.0.1:12345",
		"--beacon-url", "https://beacon.example.com",
		"--api-key", "secret",
	}, io.Discard)
	if err == nil {
		t.Fatalf("expected error when cluster-id missing")
	}
}

func TestHandleDeployBootstrapRequiresEtcdEndpoints(t *testing.T) {
	origRunner := deployBootstrapRunner
	defer func() { deployBootstrapRunner = origRunner }()

	deployBootstrapRunner = func(context.Context, deploy.Options) error {
		return errors.New("should not be called")
	}

	err := handleDeployBootstrap([]string{
		"--cluster-id", "cluster-alpha",
		"--beacon-url", "https://beacon.example.com",
		"--api-key", "secret",
		"--beacon-id", "beacon-main",
	}, io.Discard)
	if err == nil {
		t.Fatalf("expected error when etcd endpoints missing")
	}
}

func TestHandleDeployBootstrapRequiresBeaconURL(t *testing.T) {
	origRunner := deployBootstrapRunner
	defer func() { deployBootstrapRunner = origRunner }()

	deployBootstrapRunner = func(context.Context, deploy.Options) error {
		return errors.New("should not be called")
	}

	err := handleDeployBootstrap([]string{
		"--cluster-id", "cluster-alpha",
		"--etcd-endpoints", "http://127.0.0.1:12345",
		"--api-key", "secret",
		"--beacon-id", "beacon-main",
	}, io.Discard)
	if err == nil {
		t.Fatalf("expected error when beacon url missing")
	}
}

func TestHandleDeployBootstrapRequiresAPIKey(t *testing.T) {
	origRunner := deployBootstrapRunner
	defer func() { deployBootstrapRunner = origRunner }()

	deployBootstrapRunner = func(context.Context, deploy.Options) error {
		return errors.New("should not be called")
	}

	err := handleDeployBootstrap([]string{
		"--cluster-id", "cluster-alpha",
		"--etcd-endpoints", "http://127.0.0.1:12345",
		"--beacon-url", "https://beacon.example.com",
		"--beacon-id", "beacon-main",
	}, io.Discard)
	if err == nil {
		t.Fatalf("expected error when api key missing")
	}
}

func TestHandleDeployBootstrapParsesFlags(t *testing.T) {
	_, endpoint := startDeployBootstrapTestEtcd(t)

	origRunner := deployBootstrapRunner
	defer func() { deployBootstrapRunner = origRunner }()

	var captured deploy.Options
	deployBootstrapRunner = func(_ context.Context, opts deploy.Options) error {
		captured = opts
		return nil
	}

	stderr := &bytes.Buffer{}
	err := handleDeployBootstrap([]string{
		"--cluster-id", "cluster-alpha",
		"--host", "bootstrap.example.com",
		"--beacon-url", "https://beacon.example.com",
		"--control-plane-url", "https://control.example.com",
		"--api-key", "super-secret-key",
		"--etcd-endpoints", endpoint,
		"--beacon-id", "beacon-main",
		"--worker-id", "worker-bootstrap",
	}, stderr)
	if err != nil {
		t.Fatalf("handleDeployBootstrap returned error: %v", err)
	}

	if captured.ClusterID != "cluster-alpha" {
		t.Fatalf("expected cluster id cluster-alpha, got %q", captured.ClusterID)
	}
	if captured.BeaconURL != "https://beacon.example.com" {
		t.Fatalf("expected beacon url to propagate")
	}
	if captured.ControlPlaneURL != "https://control.example.com" {
		t.Fatalf("expected control plane url to propagate")
	}
	if captured.APIKey != "super-secret-key" {
		t.Fatalf("expected api key to propagate")
	}
	if len(captured.InitialBeacons) != 1 || captured.InitialBeacons[0] != "beacon-main" {
		t.Fatalf("expected beacon ids to propagate, got %v", captured.InitialBeacons)
	}
	if len(captured.InitialWorkers) != 1 || captured.InitialWorkers[0] != "worker-bootstrap" {
		t.Fatalf("expected worker ids to propagate, got %v", captured.InitialWorkers)
	}
	if captured.EtcdClient == nil {
		t.Fatalf("expected etcd client to be provided")
	}
}

func TestHandleDeployBootstrapAggregatesIDs(t *testing.T) {
	_, endpoint := startDeployBootstrapTestEtcd(t)

	origRunner := deployBootstrapRunner
	defer func() { deployBootstrapRunner = origRunner }()

	var captured deploy.Options
	deployBootstrapRunner = func(_ context.Context, opts deploy.Options) error {
		captured = opts
		return nil
	}

	err := handleDeployBootstrap([]string{
		"--cluster-id", "cluster-gamma",
		"--etcd-endpoints", endpoint,
		"--beacon-url", "https://beacon.example.com",
		"--api-key", "super-secret-key",
		"--beacon-id", "beacon-main",
		"--beacon-id", "beacon-secondary",
		"--worker-id", "worker-bootstrap",
		"--worker-id", "worker-observer",
	}, io.Discard)
	if err != nil {
		t.Fatalf("handleDeployBootstrap returned error: %v", err)
	}

	if got, want := captured.InitialBeacons, []string{"beacon-main", "beacon-secondary"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected beacon ids %v, got %v", want, got)
	}
	if got, want := captured.InitialWorkers, []string{"worker-bootstrap", "worker-observer"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected worker ids %v, got %v", want, got)
	}
}

func TestHandleDeployBootstrapDryRunSkipsEtcd(t *testing.T) {
	origRunner := deployBootstrapRunner
	defer func() { deployBootstrapRunner = origRunner }()

	var captured deploy.Options
	deployBootstrapRunner = func(_ context.Context, opts deploy.Options) error {
		captured = opts
		return nil
	}

	err := handleDeployBootstrap([]string{
		"--cluster-id", "cluster-beta",
		"--beacon-url", "https://beacon.example.com",
		"--api-key", "dry-run-key",
		"--dry-run",
	}, io.Discard)
	if err != nil {
		t.Fatalf("handleDeployBootstrap(dry-run) returned error: %v", err)
	}
	if !captured.DryRun {
		t.Fatalf("expected dry-run flag propagated")
	}
	if captured.EtcdClient != nil {
		t.Fatalf("expected etcd client to be nil for dry-run")
	}
}

func startDeployBootstrapTestEtcd(t *testing.T) (*embed.Etcd, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := embed.NewConfig()
	cfg.Dir = dir
	clientURL := mustParseBootstrapURL("http://127.0.0.1:0")
	peerURL := mustParseBootstrapURL("http://127.0.0.1:0")
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.Name = "deploy-bootstrap-cli"
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "deploy-cli-bootstrap"
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

	clientEndpoint := e.Clients[0].Addr().String()

	t.Cleanup(func() {
		e.Close()
	})

	return e, clientEndpoint
}

func mustParseBootstrapURL(raw string) url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return *parsed
}
