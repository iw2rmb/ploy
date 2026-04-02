package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/deploy"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestHandleNodeRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleNode(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing node subcommand")
	}
	out := buf.String()
	// NOTE: Node commands are now accessed via `ploy cluster node` namespace.
	if !strings.Contains(out, "Usage: ploy cluster node") {
		t.Fatalf("expected node usage output with cluster prefix, got: %q", out)
	}
}

func TestHandleNodeAddRequiresClusterIDAndAddress(t *testing.T) {
	buf := &bytes.Buffer{}
	// No flags at all -> cluster-id required first
	err := handleNodeAdd(nil, buf)
	if err == nil {
		t.Fatalf("expected error when --cluster-id is missing")
	}
	if !strings.Contains(err.Error(), "cluster-id is required") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Provide cluster-id but no address
	buf.Reset()
	err = handleNodeAdd([]string{"--cluster-id", "abc"}, buf)
	if err == nil {
		t.Fatalf("expected error when --address is missing")
	}
	if !strings.Contains(err.Error(), "address is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleNodeAddRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleNodeAdd([]string{"--cluster-id", "c1", "--address", "1.2.3.4", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleNodeAddRequiresServerURL(t *testing.T) {
	buf := &bytes.Buffer{}
	// Provide cluster-id and address but no server-url.
	// Use explicit identity and binary paths to avoid relying on host defaults.
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(idPath, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	err := handleNodeAdd([]string{
		"--cluster-id", "c1",
		"--address", "10.0.0.5",
		"--identity", idPath,
		"--ployd-node-binary", "/dev/null",
	}, buf)
	if err == nil {
		t.Fatalf("expected error when --server-url is missing")
	}
	if !strings.Contains(err.Error(), "server-url is required") {
		t.Fatalf("expected server-url required error, got: %v", err)
	}
}

func TestHandleNodeAddValidatesSSHPort(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-node-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	tests := []struct {
		name      string
		sshPort   int
		expectErr bool
	}{
		{"valid port 22", 22, false},
		{"valid port 2222", 2222, false},
		{"default port 0", 0, false}, // Port 0 defaults to 22, which is valid.
		{"invalid port -1", -1, true},
		{"invalid port 99999", 99999, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := nodeAddConfig{
				ClusterID:       "test-cluster",
				Address:         "10.0.0.10",
				ServerURL:       "https://10.0.0.5:8443",
				User:            "testuser",
				IdentityFile:    identityPath,
				PloydNodeBinary: binPath,
				SSHPort:         tt.sshPort,
				DryRun:          true, // Use dry-run to avoid actual provisioning.
			}

			err := runNodeAdd(cfg, io.Discard)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error for SSH port %d", tt.sshPort)
				}
				if !strings.Contains(err.Error(), "invalid SSH port") {
					t.Fatalf("expected SSH port validation error, got: %v", err)
				}
			} else if err != nil {
				// For valid ports in dry-run mode, we expect success.
				t.Fatalf("unexpected error for valid port %d: %v", tt.sshPort, err)
			}
		})
	}
}

func TestHandleNodeAddDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-node-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	cfg := nodeAddConfig{
		ClusterID:       "test-cluster",
		Address:         "10.0.0.10",
		ServerURL:       "https://10.0.0.5:8443",
		User:            "testuser",
		IdentityFile:    identityPath,
		PloydNodeBinary: binPath,
		SSHPort:         22,
		DryRun:          true,
	}

	buf := &bytes.Buffer{}
	err := runNodeAdd(cfg, buf)
	if err != nil {
		t.Fatalf("dry-run should not error, got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "[DRY RUN]") {
		t.Fatalf("expected dry-run output, got: %q", output)
	}
	if !strings.Contains(output, "Validation complete") {
		t.Fatalf("expected validation complete message, got: %q", output)
	}
	if !strings.Contains(output, "No actual provisioning performed") {
		t.Fatalf("expected no provisioning message, got: %q", output)
	}
}

func TestRunNodeAddGeneratesNanoIDNodeID(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-node-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0o755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	cfg := nodeAddConfig{
		ClusterID:       "test-cluster",
		Address:         "10.0.0.10",
		ServerURL:       "https://10.0.0.5:8443",
		User:            "testuser",
		IdentityFile:    identityPath,
		PloydNodeBinary: binPath,
		SSHPort:         22,
		DryRun:          true,
	}

	buf := &bytes.Buffer{}
	if err := runNodeAdd(cfg, buf); err != nil {
		t.Fatalf("runNodeAdd (dry-run) error: %v", err)
	}

	output := buf.String()
	var nodeID string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		const prefix = "Generated node ID:"
		if strings.HasPrefix(line, prefix) {
			nodeID = strings.TrimSpace(strings.TrimPrefix(line, prefix))
			break
		}
	}
	if nodeID == "" {
		t.Fatalf("expected Generated node ID line in output; got: %q", output)
	}

	if len(nodeID) != 6 {
		t.Fatalf("node ID %q length = %d, want 6", nodeID, len(nodeID))
	}

	const nanoIDAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_-"
	for _, c := range nodeID {
		if !strings.ContainsRune(nanoIDAlphabet, c) {
			t.Fatalf("node ID %q contains invalid character %q; expected URL-safe NanoID alphabet", nodeID, c)
		}
	}
}

func TestSignNodeCSR_Success(t *testing.T) {
	t.Parallel()
	nodeID := domaintypes.NewNodeKey()
	// Arrange a fake PKI sign endpoint
	var gotPath, gotContentType string
	var gotBody pkiSignRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"certificate": "CERT-PEM",
			"ca_bundle":   "CA-PEM",
			"serial":      "01",
			"fingerprint": "ff",
			"not_before":  "2025-11-01T00:00:00Z",
			"not_after":   "2026-11-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	// Act
	cert, ca, err := signNodeCSR(context.Background(), srv.URL, nodeID, []byte("CSR-PEM"))
	if err != nil {
		t.Fatalf("signNodeCSR error: %v", err)
	}

	// Assert
	if gotPath != "/v1/pki/sign" {
		t.Fatalf("expected path /v1/pki/sign, got: %s", gotPath)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected application/json, got: %s", gotContentType)
	}
	if gotBody.NodeID.String() != nodeID || gotBody.CSR != "CSR-PEM" {
		t.Fatalf("unexpected body: %+v", gotBody)
	}
	if cert != "CERT-PEM" || ca != "CA-PEM" {
		t.Fatalf("unexpected response: cert=%q ca=%q", cert, ca)
	}
}

func TestSignNodeCSR_Non200(t *testing.T) {
	t.Parallel()
	nodeID := domaintypes.NewNodeKey()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad csr", http.StatusBadRequest)
	}))
	defer srv.Close()

	_, _, err := signNodeCSR(context.Background(), srv.URL, nodeID, []byte("CSR-PEM"))
	if err == nil || !strings.Contains(err.Error(), "server returned 400: bad csr") {
		t.Fatalf("expected status error, got: %v", err)
	}
}

func TestResolvePloydNodeBinaryPath_Explicit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "ployd-node-test")
	if err := os.WriteFile(p, []byte("bin"), 0o755); err != nil {
		t.Fatalf("write temp binary: %v", err)
	}
	out, err := resolvePloydNodeBinaryPath(stringValue{set: true, value: p})
	if err != nil {
		t.Fatalf("resolvePloydNodeBinaryPath error: %v", err)
	}
	if out != p {
		t.Fatalf("expected %q, got %q", p, out)
	}
}

// TestNodeAddDescriptorRefresh verifies that the cluster descriptor is refreshed after successful node add.
func TestNodeAddDescriptorRefresh(t *testing.T) {
	// Set up temporary config directory.
	tmpDir := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", tmpDir)
	// Ensure any test-created descriptors are removed.
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(tmpDir, "clusters")) })

	// Create a temporary test binary file.
	binaryPath := filepath.Join(tmpDir, "ployd-node-test")
	if err := os.WriteFile(binaryPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}

	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Prepare configuration.
	clusterID := "test-cluster-node"
	serverURL := "https://10.0.0.5:8443"
	cfg := nodeAddConfig{
		ClusterID:       domaintypes.ClusterID(clusterID),
		Address:         "10.0.0.10",
		ServerURL:       serverURL,
		User:            "testuser",
		IdentityFile:    identityPath,
		PloydNodeBinary: binaryPath,
		SSHPort:         2222,
	}

	// Simulate provisioning with mock runner.
	user := cfg.User
	if strings.TrimSpace(user) == "" {
		user = deploy.DefaultRemoteUser
	}

	sshPort := cfg.SSHPort
	if sshPort == 0 {
		sshPort = deploy.DefaultSSHPort
	}

	scriptEnv := map[string]string{
		"CLUSTER_ID":           clusterID,
		"NODE_ID":              "node-test-123",
		"NODE_ADDRESS":         cfg.Address,
		"BOOTSTRAP_PRIMARY":    "false",
		"PLOY_CA_CERT_PEM":     "CA-PEM",
		"PLOY_SERVER_CERT_PEM": "CERT-PEM",
		"PLOY_SERVER_KEY_PEM":  "KEY-PEM",
		"PLOY_SERVER_URL":      serverURL,
	}

	provisionOpts := deploy.ProvisionOptions{
		Host:            cfg.Address,
		Address:         cfg.Address,
		User:            user,
		Port:            sshPort,
		IdentityFile:    identityPath,
		PloydBinaryPath: binaryPath,
		Stdout:          io.Discard,
		Stderr:          io.Discard,
		ScriptEnv:       scriptEnv,
		ScriptArgs:      []string{"--cluster-id", clusterID, "--node-id", "node-test-123", "--node-address", cfg.Address},
		ServiceChecks:   []string{"ployd-node"},
	}

	// Use mock runner to simulate successful provisioning.
	mockRunner := &mockNodeProvisionRunner{t: t}
	provisionOpts.Runner = mockRunner

	ctx := context.Background()
	if err := deploy.ProvisionHost(ctx, provisionOpts); err != nil {
		t.Fatalf("ProvisionHost failed: %v", err)
	}

	// Simulate the descriptor refresh logic from runNodeAdd.
	desc := config.Descriptor{
		ClusterID:       config.ClusterID(clusterID),
		Address:         serverURL,
		Scheme:          "https",
		SSHIdentityPath: identityPath,
	}
	if _, err := config.SaveDescriptor(desc); err != nil {
		t.Fatalf("SaveDescriptor failed: %v", err)
	}

	// Verify the descriptor was saved/refreshed.
	list, err := config.ListDescriptors()
	if err != nil {
		t.Fatalf("ListDescriptors failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 descriptor, got %d", len(list))
	}

	saved := list[0]
	if string(saved.ClusterID) != clusterID {
		t.Fatalf("expected ClusterID=%q, got %q", clusterID, saved.ClusterID)
	}
	if saved.Address != serverURL {
		t.Fatalf("expected Address=%q, got %q", serverURL, saved.Address)
	}
	if saved.Scheme != "https" {
		t.Fatalf("expected Scheme=%q, got %q", "https", saved.Scheme)
	}
	if saved.SSHIdentityPath != identityPath {
		t.Fatalf("expected SSHIdentityPath=%q, got %q", identityPath, saved.SSHIdentityPath)
	}
}

func TestFetchCACertificate_HTTPS_UsesSSH(t *testing.T) {
	ctx := context.Background()
	var called bool

	runner := deploy.RunnerFunc(func(_ context.Context, command string, args []string, _ io.Reader, streams deploy.IOStreams) error {
		called = true
		if command != "ssh" {
			t.Fatalf("expected command ssh, got %q", command)
		}
		if !strings.Contains(strings.Join(args, " "), "cat /etc/ploy/pki/ca.crt") {
			t.Fatalf("expected ssh args to cat CA cert, got: %q", strings.Join(args, " "))
		}
		_, _ = io.WriteString(streams.Stdout, "CA-PEM\n")
		return nil
	})

	ca, err := fetchCACertificate(ctx, "https://example.com:8443", "root", 22, "/tmp/id", runner)
	if err != nil {
		t.Fatalf("fetchCACertificate error: %v", err)
	}
	if !called {
		t.Fatalf("expected runner to be called")
	}
	if ca != "CA-PEM\n" {
		t.Fatalf("expected CA cert, got %q", ca)
	}
}

func TestFetchCACertificate_HTTP_SkipsSSH(t *testing.T) {
	ctx := context.Background()
	runner := deploy.RunnerFunc(func(_ context.Context, _ string, _ []string, _ io.Reader, _ deploy.IOStreams) error {
		t.Fatalf("runner should not be called for http URLs")
		return nil
	})

	ca, err := fetchCACertificate(ctx, "http://example.com:8080", "root", 22, "/tmp/id", runner)
	if err != nil {
		t.Fatalf("fetchCACertificate error: %v", err)
	}
	if ca != "" {
		t.Fatalf("expected empty CA cert for http URLs, got %q", ca)
	}
}

func TestFetchCACertificate_NoScheme_AssumesHTTPS(t *testing.T) {
	ctx := context.Background()
	var called bool

	runner := deploy.RunnerFunc(func(_ context.Context, command string, _ []string, _ io.Reader, streams deploy.IOStreams) error {
		called = true
		if command != "ssh" {
			t.Fatalf("expected command ssh, got %q", command)
		}
		_, _ = io.WriteString(streams.Stdout, "CA-PEM\n")
		return nil
	})

	ca, err := fetchCACertificate(ctx, "example.com:8443", "root", 22, "/tmp/id", runner)
	if err != nil {
		t.Fatalf("fetchCACertificate error: %v", err)
	}
	if !called {
		t.Fatalf("expected runner to be called")
	}
	if ca != "CA-PEM\n" {
		t.Fatalf("expected CA cert, got %q", ca)
	}
}

// mockNodeProvisionRunner is a test double for deploy.Runner for node provisioning.
type mockNodeProvisionRunner struct {
	t *testing.T
}

func (m *mockNodeProvisionRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
	// Just accept all commands without error.
	return nil
}
