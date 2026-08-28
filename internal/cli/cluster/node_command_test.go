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
	"github.com/iw2rmb/ploy/internal/deploy"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/iw2rmb/ploy/internal/testutil/assertx"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestHandleNodeListPrintsHostNameAndAddress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "alpha", "name": "node-alpha", "ip_address": "10.0.0.1", "concurrency": 2},
			{"id": "beta", "name": "node-beta", "ip_address": "10.0.0.2", "concurrency": 1},
		})
	}))
	t.Cleanup(server.Close)
	clienv.UseControlPlaneEnv(t, server.URL)

	stdout := &bytes.Buffer{}
	if err := handleNode([]string{"ls"}, stdout, io.Discard); err != nil {
		t.Fatalf("node ls: %v", err)
	}
	for _, want := range []string{"HOST", "NAME", "ADDRESS", "alpha", "node-alpha", "10.0.0.2"} {
		assertx.Contains(t, stdout.String(), want)
	}
}

func TestHandleNodeListPrintsEmptyState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]any{})
	}))
	t.Cleanup(server.Close)
	clienv.UseControlPlaneEnv(t, server.URL)

	stdout := &bytes.Buffer{}
	if err := handleNode([]string{"ls"}, stdout, io.Discard); err != nil {
		t.Fatalf("node ls: %v", err)
	}
	if got := stdout.String(); got != "No nodes found.\n" {
		t.Fatalf("output = %q, want empty state", got)
	}
}

func TestHandleNodeInspectPrintsCapacityResourcesImageAndRunningJobs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id": "alpha", "name": "node-alpha", "ip_address": "10.0.0.1", "concurrency": 1,
				"cpu_total_millis": 16000, "cpu_free_millis": 8000,
				"mem_total_bytes": 8589934592, "mem_free_bytes": 4294967296,
				"disk_total_bytes": 107374182400, "disk_free_bytes": 53687091200,
				"last_heartbeat": "2026-08-28T06:09:52Z",
			}})
		case "/v1/nodes/alpha/diagnostics":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"node_id": "alpha", "component": "node", "status": "ok",
				"image_ref": "registry.example/ploy/node:latest", "details": map[string]any{"concurrency": 3},
				"updated_at": "2026-08-28T06:09:52Z",
			}})
		case "/v1/jobs":
			if got := r.URL.Query().Get("node_id"); got != "alpha" {
				t.Errorf("node_id = %q, want alpha", got)
			}
			if got := r.URL.Query().Get("status"); got != "Running" {
				t.Errorf("status = %q, want Running", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jobs": []map[string]any{{
					"job_id": "3IX3mmup1S4DZadb2ukhr7Rxm3q", "name": "pre-gate", "job_type": "pre_gate",
					"status": "Running", "duration_ms": 0, "job_image": "gate:latest", "node_id": "alpha",
					"mig_name": "upgrade", "run_id": "3IX3mXbI8PEvIfjLeNRkFHuMYSX", "repo_id": "8dj0K11g",
				}},
				"total": 1,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	clienv.UseControlPlaneEnv(t, server.URL)

	stdout := &bytes.Buffer{}
	if err := handleNode([]string{"inspect", "node-alpha"}, stdout, io.Discard); err != nil {
		t.Fatalf("node inspect: %v", err)
	}
	for _, want := range []string{
		"Name:             node-alpha",
		"Host:             alpha",
		"Image:            registry.example/ploy/node:latest",
		"Queue:            1/3 active (2 available)",
		"CPU available:    8.00 / 16.00 cores",
		"Memory available: 4.00 GiB / 8.00 GiB",
		"Disk available:   50.00 GiB / 100.00 GiB",
		"3IX3mmup1S4DZadb2ukhr7Rxm3q",
	} {
		assertx.Contains(t, stdout.String(), want)
	}
}

func TestHandleNodeInspectMissingNameReturnsNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "alpha", "name": "node-alpha", "ip_address": "10.0.0.1"}})
	}))
	t.Cleanup(server.Close)
	clienv.UseControlPlaneEnv(t, server.URL)

	err := handleNode([]string{"inspect", "missing"}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), `node "missing" not found`) {
		t.Fatalf("error = %v, want not found", err)
	}
}

func TestRenderNodeInspectHandlesMissingTelemetryAndNoJobs(t *testing.T) {
	stdout := &bytes.Buffer{}
	renderNodeInspect(stdout, domainapi.Node{
		ID: "alpha", Name: "node-alpha", IPAddress: "0.0.0.0", Concurrency: 2,
	}, nil, nil)

	for _, want := range []string{
		"Image:            unknown",
		"Last heartbeat:   unknown",
		"Queue:            0/2 active (2 available)",
		"CPU available:    unknown",
		"Memory available: unknown",
		"Disk available:   unknown",
		"Active jobs:\n  none",
	} {
		assertx.Contains(t, stdout.String(), want)
	}
}

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
		Address:         "10.0.0.10",
		ServerURL:       "https://10.0.0.5:8443",
		User:            "testuser",
		IdentityFile:    idPath,
		PloydNodeBinary: binPath,
	}
}

func TestHandleNodeAddRequiresAddress(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleNodeAdd(nil, buf)
	if err == nil {
		t.Fatalf("expected error when --address is missing")
	}
	if !strings.Contains(err.Error(), "address is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleNodeAddRequiresServerURL(t *testing.T) {
	idPath := filepath.Join(t.TempDir(), "id_test")
	if err := os.WriteFile(idPath, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}
	clienv.RunExpectError(t, handleNodeAdd, []string{
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
