package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/deploy"
)

const identityBootstrapKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJ076bootTestAdmin deploy-admin"

func TestHandleClusterRequiresSubcommand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	buf := &bytes.Buffer{}
	err := handleCluster(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing cluster subcommand")
	}
	if buf.Len() == 0 {
		t.Fatalf("expected usage output when cluster subcommand missing")
	}
}

func TestHandleClusterListOutputsDescriptors(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	_, err := config.SaveDescriptor(config.Descriptor{
		ClusterID:       "alpha",
		Address:         "10.10.0.1",
		SSHIdentityPath: "/home/ploy/.ssh/id_alpha",
		Labels:          map[string]string{"env": "dev"},
	})
	if err != nil {
		t.Fatalf("save alpha: %v", err)
	}
	_, err = config.SaveDescriptor(config.Descriptor{
		ClusterID:       "beta",
		Address:         "10.10.0.2",
		SSHIdentityPath: "/home/ploy/.ssh/id_beta",
		Labels:          map[string]string{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("save beta: %v", err)
	}
	if err := config.SetDefault("beta"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	buf := &bytes.Buffer{}
	if err := handleCluster([]string{"list"}, buf); err != nil {
		t.Fatalf("handleCluster list: %v", err)
	}
	output := buf.String()
	if !containsAll(output,
		"alpha",
		"address=10.10.0.1",
		"identity=/home/ploy/.ssh/id_alpha",
		"labels=env=dev",
		"beta (default)",
		"address=10.10.0.2",
		"labels=env=prod") {
		t.Fatalf("unexpected cluster list output:\n%s", output)
	}
}

func containsAll(s string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(s, needle) {
			return false
		}
	}
	return true
}

func TestHandleClusterConnectStoresDescriptor(t *testing.T) {
	t.Skip("cluster connect implementation pending")
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	const caBody = "-----BEGIN CERTIFICATE-----\nMIIC...\n-----END CERTIFICATE-----\n"
	var configCalls, caCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/beacon/config":
			configCalls++
			if got := r.Header.Get("Authorization"); got != "Bearer api-key" {
				t.Fatalf("expected Authorization header, got %q", got)
			}
			_, _ = w.Write([]byte(`{"control_plane_url":"https://api.example","version":"2025.10.21"}`))
		case "/v1/beacon/ca":
			caCalls++
			if got := r.Header.Get("Authorization"); got != "Bearer api-key" {
				t.Fatalf("expected Authorization header for CA request, got %q", got)
			}
			_, _ = w.Write([]byte(caBody))
		default:
			t.Fatalf("unexpected beacon path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	buf := &bytes.Buffer{}
	args := []string{"connect", "--id", "lab", "--beacon-url", server.URL, "--api-key", "api-key", "--set-default"}
	if err := handleCluster(args, buf); err != nil {
		t.Fatalf("handleCluster connect: %v", err)
	}
	if configCalls != 1 || caCalls != 1 {
		t.Fatalf("expected 1 config and 1 ca call, got config=%d ca=%d", configCalls, caCalls)
	}
	desc, err := config.LoadDescriptor("lab")
	if err != nil {
		t.Fatalf("LoadDescriptor: %v", err)
	}
	if !desc.Default {
		t.Fatalf("expected descriptor marked default")
	}
	if desc.Address == "" {
		t.Fatalf("expected Address recorded, got empty")
	}
	if desc.SSHIdentityPath == "" {
		t.Fatalf("expected SSH identity recorded, got empty")
	}
	if buf.Len() == 0 {
		t.Fatalf("expected output summarizing cluster connection")
	}
}

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
	if provision.ScriptEnv["PLOY_CONTROL_PLANE_ENDPOINT"] == "" {
		t.Fatalf("expected control plane endpoint exported to script env")
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

func TestHandleClusterCertStatus(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	desc, err := config.SaveDescriptor(config.Descriptor{
		ClusterID:       "cluster-alpha",
		Address:         "203.0.113.10",
		SSHIdentityPath: "/home/ploy/.ssh/id_alpha",
	})
	if err != nil {
		t.Fatalf("save descriptor: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cluster_id") != desc.ClusterID {
			t.Fatalf("expected cluster_id query, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"cluster_id":"cluster-alpha","current_ca":{"version":"20251025","issued_at":"2025-10-25T12:00:00Z","expires_at":"2026-10-25T12:00:00Z","serial_number":"abcd"},"workers":{"total":3},"trust_bundle_hash":"deadbeef"}`)
	}))
	defer server.Close()
	t.Setenv("PLOYD_ADMIN_ENDPOINT", server.URL)
	origFactory := clusterHTTPClientFactory
	defer func() { clusterHTTPClientFactory = origFactory }()
	clusterHTTPClientFactory = func(config.Descriptor) (*http.Client, func(), error) {
		return server.Client(), func() {}, nil
	}
	buf := &bytes.Buffer{}
	if err := handleCluster([]string{"cert", "status", "--cluster-id", desc.ClusterID}, buf); err != nil {
		t.Fatalf("cluster cert status returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Version: 20251025") {
		t.Fatalf("expected version in output, got %q", output)
	}
	if !strings.Contains(output, "Workers: 3") {
		t.Fatalf("expected worker count in output, got %q", output)
	}
}

func ploydFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ployd")
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write temp ployd binary: %v", err)
	}
	return path
}

func identityFixture(t *testing.T, key string) string {
	t.Helper()
	dir := t.TempDir()
	priv := filepath.Join(dir, "id_rsa")
	if err := os.WriteFile(priv, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatalf("write identity private key: %v", err)
	}
	pub := priv + ".pub"
	if err := os.WriteFile(pub, []byte(key+"\n"), 0o644); err != nil {
		t.Fatalf("write identity public key: %v", err)
	}
	return priv
}

func TestDescriptorControlPlaneURL(t *testing.T) {
	desc := config.Descriptor{ClusterID: "lab", Address: "203.0.113.10"}
	t.Setenv("PLOYD_ADMIN_ENDPOINT", "")
	t.Setenv("PLOYD_ADMIN_SCHEME", "")
	t.Setenv("PLOYD_ADMIN_PORT", "")

	url, err := descriptorControlPlaneURL(desc)
	if err != nil {
		t.Fatalf("descriptorControlPlaneURL default failed: %v", err)
	}
	if url != "http://203.0.113.10:8443" {
		t.Fatalf("expected default http url, got %s", url)
	}

	t.Run("scheme override", func(t *testing.T) {
		t.Setenv("PLOYD_ADMIN_SCHEME", "https")
		t.Setenv("PLOYD_ADMIN_PORT", "9443")
		url, err := descriptorControlPlaneURL(desc)
		if err != nil {
			t.Fatalf("descriptorControlPlaneURL scheme override failed: %v", err)
		}
		if url != "https://203.0.113.10:9443" {
			t.Fatalf("expected override url, got %s", url)
		}
	})

	t.Run("endpoint override", func(t *testing.T) {
		t.Setenv("PLOYD_ADMIN_ENDPOINT", "https://control.example.com:9000")
		url, err := descriptorControlPlaneURL(desc)
		if err != nil {
			t.Fatalf("descriptorControlPlaneURL endpoint override failed: %v", err)
		}
		if url != "https://control.example.com:9000" {
			t.Fatalf("expected endpoint override, got %s", url)
		}
	})

	t.Run("invalid port", func(t *testing.T) {
		t.Setenv("PLOYD_ADMIN_ENDPOINT", "")
		t.Setenv("PLOYD_ADMIN_SCHEME", "")
		t.Setenv("PLOYD_ADMIN_PORT", "not-a-number")
		if _, err := descriptorControlPlaneURL(desc); err == nil {
			t.Fatalf("expected error for invalid admin port")
		}
	})
}
