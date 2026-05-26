package cluster

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

	"github.com/iw2rmb/ploy/internal/cli/common"
	cliconfig "github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/deploy"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/assertx"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

// newTestNodeAddConfig returns a nodeAddConfig wired up with stub identity and
// ployd-node binary files under t.TempDir(). Callers set SSHPort/DryRun as needed.
func newTestNodeAddConfig(t *testing.T) nodeAddConfig {
	t.Helper()
	dir := t.TempDir()
	binPath := filepath.Join(dir, "ployd-node-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0o755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	idPath := filepath.Join(dir, "id_test")
	if err := os.WriteFile(idPath, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}
	return nodeAddConfig{
		ClusterID:       "test-cluster",
		Address:         "10.0.0.10",
		ServerURL:       "https://10.0.0.5:8443",
		User:            "testuser",
		IdentityFile:    idPath,
		PloydNodeBinary: binPath,
	}
}

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

func TestHandleNodeActionsRequiresNodeID(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "actions", args: []string{"actions"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := handleNode(tt.args, buf)
			if err == nil {
				t.Fatal("expected missing node-id error")
			}
			if !strings.Contains(err.Error(), "node-id is required") {
				t.Fatalf("error = %v, want node-id is required", err)
			}
		})
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
	idPath := filepath.Join(t.TempDir(), "id_test")
	if err := os.WriteFile(idPath, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}
	clienv.RunExpectError(t, handleNodeAdd, []string{
		"--cluster-id", "c1",
		"--address", "10.0.0.5",
		"--identity", idPath,
		"--ployd-node-binary", "/dev/null",
	}, "server-url is required")
}

func TestHandleNodeAddValidatesSSHPort(t *testing.T) {
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
			cfg := newTestNodeAddConfig(t)
			cfg.SSHPort = tt.sshPort
			cfg.DryRun = true

			err := runNodeAdd(cfg, io.Discard)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error for SSH port %d", tt.sshPort)
				}
				if !strings.Contains(err.Error(), "invalid SSH port") {
					t.Fatalf("expected SSH port validation error, got: %v", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error for valid port %d: %v", tt.sshPort, err)
			}
		})
	}
}

func TestHandleNodeAddDryRun(t *testing.T) {
	cfg := newTestNodeAddConfig(t)
	cfg.SSHPort = 22
	cfg.DryRun = true

	buf := &bytes.Buffer{}
	if err := runNodeAdd(cfg, buf); err != nil {
		t.Fatalf("dry-run should not error, got: %v", err)
	}
	out := buf.String()
	assertx.Contains(t, out, "[DRY RUN]")
	assertx.Contains(t, out, "Validation complete")
	assertx.Contains(t, out, "No actual provisioning performed")
}

func TestRunNodeAddGeneratesNanoIDNodeID(t *testing.T) {
	cfg := newTestNodeAddConfig(t)
	cfg.SSHPort = 22
	cfg.DryRun = true

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
	configureDefaultClusterForNodeTest(t, srv.URL)

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
	nodeID := domaintypes.NewNodeKey()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad csr", http.StatusBadRequest)
	}))
	defer srv.Close()
	configureDefaultClusterForNodeTest(t, srv.URL)

	_, _, err := signNodeCSR(context.Background(), srv.URL, nodeID, []byte("CSR-PEM"))
	if err == nil || !strings.Contains(err.Error(), "server returned 400: bad csr") {
		t.Fatalf("expected status error, got: %v", err)
	}
}

func configureDefaultClusterForNodeTest(t *testing.T, serverURL string) {
	t.Helper()
	clienv.IsolateConfigHomeAllowDefault(t)
	clusterID := cliconfig.ClusterID("test")
	if _, err := cliconfig.SaveDescriptor(cliconfig.Descriptor{ClusterID: clusterID, Address: serverURL}); err != nil {
		t.Fatalf("SaveDescriptor: %v", err)
	}
	if err := cliconfig.SetDefault(clusterID); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
}

func TestResolvePloydNodeBinaryPath_Explicit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "ployd-node-test")
	if err := os.WriteFile(p, []byte("bin"), 0o755); err != nil {
		t.Fatalf("write temp binary: %v", err)
	}
	out, err := resolvePloydNodeBinaryPath(common.StringValue{IsSet: true, Value: p})
	if err != nil {
		t.Fatalf("resolvePloydNodeBinaryPath error: %v", err)
	}
	if out != p {
		t.Fatalf("expected %q, got %q", p, out)
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
