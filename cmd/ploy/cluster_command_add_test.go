package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/deploy"
)

func TestHandleClusterAddRequiresAddress(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	orig := clusterBootstrapRunner
	defer func() { clusterBootstrapRunner = orig }()
	clusterBootstrapRunner = func(context.Context, deploy.Options) error {
		t.Fatalf("bootstrap runner should not be invoked without address")
		return nil
	}
	identityPath := identityFixture(t, identityBootstrapKey)
	ploydPath := ploydFixture(t)
	err := handleCluster([]string{"add", "--identity", identityPath, "--ployd-binary", ploydPath}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "address") {
		t.Fatalf("expected address error, got %v", err)
	}
}

func TestHandleClusterAddWithoutClusterIDBootstrapsControlPlane(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PLOY_SKIP_REMOTE_CA_FETCH", "true")
	orig := clusterBootstrapRunner
	defer func() { clusterBootstrapRunner = orig }()
	var captured deploy.Options
	clusterBootstrapRunner = func(_ context.Context, opts deploy.Options) error {
		captured = opts
		return nil
	}
	identityPath := identityFixture(t, identityBootstrapKey)
	ploydPath := ploydFixture(t)
	buf := &bytes.Buffer{}
	err := handleCluster([]string{"add", "--address", "192.0.2.10", "--identity", identityPath, "--ployd-binary", ploydPath}, buf)
	if err != nil {
		t.Fatalf("cluster add bootstrap returned error: %v", err)
	}
	if captured.Address != "192.0.2.10" {
		t.Fatalf("expected address propagated, got %q", captured.Address)
	}
	if captured.IdentityFile != identityPath {
		t.Fatalf("expected identity path propagated, got %q", captured.IdentityFile)
	}
	if captured.PloydBinaryPath != ploydPath {
		t.Fatalf("expected ployd binary path propagated, got %q", captured.PloydBinaryPath)
	}
	if captured.DescriptorID != "192.0.2.10" {
		t.Fatalf("expected descriptor id to mirror address, got %q", captured.DescriptorID)
	}
}

func TestHandleClusterAddAllowsWorkerFlagsWithoutClusterID(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PLOY_SKIP_REMOTE_CA_FETCH", "true")
	orig := clusterBootstrapRunner
	defer func() { clusterBootstrapRunner = orig }()
	var invoked bool
	clusterBootstrapRunner = func(context.Context, deploy.Options) error {
		invoked = true
		return nil
	}
	identityPath := identityFixture(t, identityBootstrapKey)
	ploydPath := ploydFixture(t)
	err := handleCluster([]string{"add",
		"--address", "192.0.2.10",
		"--label", "role=worker",
		"--health-probe", "ready=https://192.0.2.10:9443/status",
		"--identity", identityPath,
		"--ployd-binary", ploydPath,
	}, io.Discard)
	if err != nil {
		t.Fatalf("cluster add bootstrap returned error: %v", err)
	}
	if !invoked {
		t.Fatalf("expected bootstrap runner invoked")
	}
}

func TestHandleClusterAddWithClusterIDAddsWorker(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	descriptor, err := config.SaveDescriptor(config.Descriptor{
		ClusterID:       "lab",
		Address:         "203.0.113.10",
		SSHIdentityPath: "/home/ploy/.ssh/id_lab",
	})
	if err != nil {
		t.Fatalf("save descriptor: %v", err)
	}
	identityPath := identityFixture(t, identityBootstrapKey)
	ploydPath := ploydFixture(t)
	origProvision := clusterProvisionHost
	defer func() { clusterProvisionHost = origProvision }()
	var provision deploy.ProvisionOptions
	clusterProvisionHost = func(_ context.Context, opts deploy.ProvisionOptions) error {
		provision = opts
		return nil
	}
	origRegister := clusterWorkerRegister
	defer func() { clusterWorkerRegister = origRegister }()
	var payload nodeJoinRequest
	clusterWorkerRegister = func(_ context.Context, _ *http.Client, baseURL string, req nodeJoinRequest) (nodeJoinResponse, error) {
		payload = req
		if baseURL == "" {
			t.Fatalf("expected base URL propagated")
		}
		return nodeJoinResponse{WorkerID: "worker-1"}, nil
	}
	origFactory := clusterHTTPClientFactory
	defer func() { clusterHTTPClientFactory = origFactory }()
	var factoryDescriptor config.Descriptor
	clusterHTTPClientFactory = func(desc config.Descriptor) (*http.Client, func(), error) {
		factoryDescriptor = desc
		return &http.Client{}, func() {}, nil
	}
	buf := &bytes.Buffer{}
	err = handleCluster([]string{"add",
		"--cluster-id", descriptor.ClusterID,
		"--address", "198.51.100.7",
		"--identity", identityPath,
		"--ployd-binary", ploydPath,
	}, buf)
	if err != nil {
		t.Fatalf("cluster add worker returned error: %v", err)
	}
	if provision.Host != "198.51.100.7" || provision.Address != "198.51.100.7" {
		t.Fatalf("expected worker address propagated, got %+v", provision)
	}
	if _, ok := provision.ScriptEnv["PLOYD_MODE"]; ok {
		t.Fatalf("expected bootstrap env to omit PLOYD_MODE")
	}
	if len(provision.ScriptEnv) != 0 {
		t.Fatalf("expected no script env overrides, got %+v", provision.ScriptEnv)
	}
	if got := strings.Join(provision.ScriptArgs, " "); got != "--cluster-id "+descriptor.ClusterID {
		t.Fatalf("expected cluster id script arg, got %q", provision.ScriptArgs)
	}
	if provision.IdentityFile != identityPath {
		t.Fatalf("expected identity path propagated to provisioner, got %q", provision.IdentityFile)
	}
	if factoryDescriptor.ClusterID != descriptor.ClusterID {
		t.Fatalf("expected descriptor passed to HTTP client factory")
	}
	if payload.ClusterID != descriptor.ClusterID {
		t.Fatalf("expected payload cluster id %s, got %s", descriptor.ClusterID, payload.ClusterID)
	}
	if payload.Address != "198.51.100.7" {
		t.Fatalf("expected payload address propagated, got %s", payload.Address)
	}
	if !strings.Contains(buf.String(), "worker-1") {
		t.Fatalf("expected worker join output, got %q", buf.String())
	}
}
