package deploy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/iw2rmb/ploy/cmd/ploy/config"
)

func TestRenderBootstrapScriptVersions(t *testing.T) {
	script := RenderBootstrapScript()
	for _, fragment := range []string{
		`ETCD_VERSION="3.6.`,
		`IPFS_CLUSTER_VERSION="1.1.4"`,
		`DOCKER_CHANNEL="28"`,
		`GO_VERSION="1.25`,
		"check_package_manager",
		"check_required_ports",
		"check_disk_space",
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected bootstrap script to contain %q", fragment)
		}
	}
}

func TestRunBootstrapDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()
	opts := Options{
		DryRun:    true,
		Stdout:    &buf,
		ClusterID: "cluster",
	}
	if err := RunBootstrap(ctx, opts); err != nil {
		t.Fatalf("RunBootstrap(dry-run) returned error: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "PLOY_BOOTSTRAP_VERSION") {
		t.Fatalf("expected dry-run output to include script metadata, got:\n%s", got)
	}
}

func TestRunBootstrapRequiresHost(t *testing.T) {
	ctx := context.Background()
	opts := Options{ClusterID: "cluster"}
	if err := RunBootstrap(ctx, opts); err == nil {
		t.Fatalf("expected error when host not provided")
	}
}

func TestRunBootstrapInvokesSSH(t *testing.T) {
	ctx := context.Background()
	var invoked struct {
		Command string
		Args    []string
		Stdin   string
	}
	runner := RunnerFunc(func(_ context.Context, command string, args []string, stdin string, _ IOStreams) error {
		invoked.Command = command
		invoked.Args = append([]string(nil), args...)
		invoked.Stdin = stdin
		return nil
	})

	opts := Options{
		Host:      "bootstrap.example.com",
		User:      "ploy",
		Runner:    runner,
		Stderr:    io.Discard,
		ClusterID: "cluster",
	}

	if err := RunBootstrap(ctx, opts); err != nil {
		t.Fatalf("RunBootstrap returned error: %v", err)
	}

	if invoked.Command != "ssh" {
		t.Fatalf("expected ssh command, got %q", invoked.Command)
	}
	if len(invoked.Args) == 0 || invoked.Args[len(invoked.Args)-1] != "bash -s --" {
		t.Fatalf("expected trailing stdin exec argument, got %v", invoked.Args)
	}
	if !strings.Contains(invoked.Stdin, "ETCD_VERSION=\"3.6.") {
		t.Fatalf("expected stdin script to contain ETCD version, got:\n%s", invoked.Stdin)
	}
}

func TestRunBootstrapUsesAddressOverride(t *testing.T) {
	ctx := context.Background()
	var captured []string
	runner := RunnerFunc(func(_ context.Context, _ string, args []string, _ string, _ IOStreams) error {
		captured = append([]string(nil), args...)
		return nil
	})

	opts := Options{
		Host:      "abcd1234abcd1234.ploy",
		Address:   "45.9.42.212",
		Runner:    runner,
		Stderr:    io.Discard,
		ClusterID: "cluster",
	}

	if err := RunBootstrap(ctx, opts); err != nil {
		t.Fatalf("RunBootstrap returned error: %v", err)
	}

	if len(captured) == 0 {
		t.Fatalf("expected ssh arguments captured")
	}
	target := captured[len(captured)-2]
	if target != "root@45.9.42.212" {
		t.Fatalf("expected ssh target to use address override, got %q", target)
	}
}

func TestImplementScriptInvokesCodex(t *testing.T) {
	docPath := filepath.Join("..", "..", "docs", "v2", "implement.sh")
	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", docPath, err)
	}
	if !strings.Contains(string(data), "CODEX_BIN") {
		t.Fatalf("expected docs/v2/implement.sh to contain Codex automation wrapper")
	}
}

func TestRunBootstrapBootstrapsPKIAndDescriptor(t *testing.T) {
	ctx := context.Background()
	etcd, client := newBootstrapTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	t.Setenv("PLOY_CONFIG_HOME", "")

	var invoked bool
	runner := RunnerFunc(func(_ context.Context, command string, args []string, stdin string, _ IOStreams) error {
		invoked = true
		if command != "ssh" {
			t.Fatalf("expected ssh command, got %q", command)
		}
		if len(args) == 0 {
			t.Fatalf("expected ssh arguments to be populated")
		}
		if !strings.Contains(stdin, "PLOY_BOOTSTRAP_VERSION") {
			t.Fatalf("expected bootstrap script to render into stdin")
		}
		return nil
	})

	opts := Options{
		Host:            "bootstrap.example.com",
		Address:         "203.0.113.7",
		Runner:          runner,
		Stderr:          io.Discard,
		ClusterID:       "cluster-alpha",
		EtcdClient:      client,
		InitialBeacons:  []string{"beacon-main"},
		InitialWorkers:  []string{"worker-bootstrap"},
		BeaconURL:       "https://beacon.example.com",
		ControlPlaneURL: "https://control.example.com",
		APIKey:          "secret-api-key",
	}

	if err := RunBootstrap(ctx, opts); err != nil {
		t.Fatalf("RunBootstrap returned error: %v", err)
	}
	if !invoked {
		t.Fatalf("expected bootstrap runner to execute ssh command")
	}

	manager, err := NewCARotationManager(client, "cluster-alpha")
	if err != nil {
		t.Fatalf("NewCARotationManager: %v", err)
	}
	state, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("State returned error: %v", err)
	}
	if len(state.BeaconCertificates) != 1 {
		t.Fatalf("expected one beacon certificate, got %d", len(state.BeaconCertificates))
	}
	if _, ok := state.BeaconCertificates["beacon-main"]; !ok {
		t.Fatalf("expected beacon-main certificate to be issued")
	}
	if len(state.WorkerCertificates) != 1 {
		t.Fatalf("expected worker certificate issued, got %d", len(state.WorkerCertificates))
	}
	if _, ok := state.WorkerCertificates["worker-bootstrap"]; !ok {
		t.Fatalf("expected worker-bootstrap certificate to be issued")
	}

	desc, err := config.LoadDescriptor("cluster-alpha")
	if err != nil {
		t.Fatalf("LoadDescriptor returned error: %v", err)
	}
	if desc.BeaconURL != "https://beacon.example.com" {
		t.Fatalf("expected descriptor beacon url, got %q", desc.BeaconURL)
	}
	if desc.APIKey != "secret-api-key" {
		t.Fatalf("expected descriptor api key, got %q", desc.APIKey)
	}
	if desc.ControlPlaneURL != "https://control.example.com" {
		t.Fatalf("expected descriptor control plane url, got %q", desc.ControlPlaneURL)
	}
	if desc.CABundlePath == "" {
		t.Fatalf("expected descriptor to include ca bundle path")
	}
	caData, err := os.ReadFile(desc.CABundlePath)
	if err != nil {
		t.Fatalf("read ca bundle: %v", err)
	}
	if !strings.Contains(string(caData), "BEGIN CERTIFICATE") {
		t.Fatalf("expected ca bundle file to contain certificate block")
	}

	result, err := RunWorkerJoin(ctx, client, WorkerJoinOptions{
		ClusterID: "cluster-alpha",
		WorkerID:  "worker-dryrun",
		Address:   "192.0.2.10",
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("RunWorkerJoin dry-run returned error: %v", err)
	}
	if !result.DryRun {
		t.Fatalf("expected dry run flag propagated")
	}
}

func TestRunBootstrapRequiresClusterID(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		Host:   "bootstrap.example.com",
		Runner: RunnerFunc(func(context.Context, string, []string, string, IOStreams) error { return nil }),
	}
	if err := RunBootstrap(ctx, opts); err == nil {
		t.Fatalf("expected error when cluster id not provided")
	}
}

func TestRunBootstrapRequiresEtcdClientWhenNotDryRun(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		Host:      "bootstrap.example.com",
		ClusterID: "cluster-alpha",
		Runner:    RunnerFunc(func(context.Context, string, []string, string, IOStreams) error { return nil }),
	}
	if err := RunBootstrap(ctx, opts); err == nil {
		t.Fatalf("expected error when etcd client not provided")
	}
}

func newBootstrapTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
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
	cfg.Name = "bootstrap"
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "deploy-bootstrap-test"
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
