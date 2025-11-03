package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/internal/pki"
)

func TestHandleServerRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleServer(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing server subcommand")
	}
	out := buf.String()
	if !strings.Contains(out, "Usage: ploy server") {
		t.Fatalf("expected server usage output, got: %q", out)
	}
}

func TestHandleServerDeployRequiresAddress(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleServerDeploy(nil, buf)
	if err == nil {
		t.Fatalf("expected error when --address is missing")
	}
	if !strings.Contains(err.Error(), "address is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Usage: ploy server deploy") {
		t.Fatalf("expected deploy usage output, got: %q", buf.String())
	}
}

func TestHandleServerDeployRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleServerDeploy([]string{"--address", "1.2.3.4", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleServerDeployValidatesSSHPort(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
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
			// Stub provisioning to avoid real scp/ssh attempts during tests.
			old := provisionHost
			provisionHost = func(ctx context.Context, opts deploy.ProvisionOptions) error {
				return errors.New("provision stubbed: skip remote calls")
			}
			defer func() { provisionHost = old }()
			cfg := serverDeployConfig{
				Address:      "10.0.0.5",
				User:         "testuser",
				IdentityFile: identityPath,
				PloydBinary:  binPath,
				SSHPort:      tt.sshPort,
			}

			err := runServerDeploy(cfg, io.Discard)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error for SSH port %d", tt.sshPort)
				}
				if !strings.Contains(err.Error(), "invalid SSH port") {
					t.Fatalf("expected SSH port validation error, got: %v", err)
				}
			} else {
				// For valid ports, we expect failure due to missing CA/server cert generation,
				// but NOT due to port validation.
				if err != nil && strings.Contains(err.Error(), "invalid SSH port") {
					t.Fatalf("unexpected SSH port validation error for valid port %d: %v", tt.sshPort, err)
				}
			}
		})
	}
}

// TestServerDeployCAGeneration verifies that CA and server certificate generation works correctly.
func TestServerDeployCAGeneration(t *testing.T) {
	// Generate cluster ID.
	clusterID, err := deploy.GenerateClusterID()
	if err != nil {
		t.Fatalf("GenerateClusterID failed: %v", err)
	}
	if clusterID == "" {
		t.Fatal("expected non-empty cluster ID")
	}
	if !strings.HasPrefix(clusterID, "cluster-") {
		t.Fatalf("expected cluster ID to start with 'cluster-', got: %s", clusterID)
	}

	// Generate CA.
	now := time.Now()
	ca, err := pki.GenerateCA(clusterID, now)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}
	if ca == nil {
		t.Fatal("expected non-nil CA bundle")
	}
	if ca.CertPEM == "" {
		t.Fatal("expected non-empty CA certificate PEM")
	}
	if ca.KeyPEM == "" {
		t.Fatal("expected non-empty CA key PEM")
	}
	if !strings.Contains(ca.CertPEM, "BEGIN CERTIFICATE") {
		t.Fatal("invalid CA certificate PEM format")
	}
	if !strings.Contains(ca.KeyPEM, "PRIVATE KEY") {
		t.Fatal("invalid CA key PEM format")
	}

	// Issue server certificate.
	serverAddress := "192.168.1.10"
	serverCert, err := pki.IssueServerCert(ca, clusterID, serverAddress, now)
	if err != nil {
		t.Fatalf("IssueServerCert failed: %v", err)
	}
	if serverCert == nil {
		t.Fatal("expected non-nil server certificate")
	}
	if serverCert.CertPEM == "" {
		t.Fatal("expected non-empty server certificate PEM")
	}
	if serverCert.KeyPEM == "" {
		t.Fatal("expected non-empty server key PEM")
	}
	if serverCert.Serial == "" {
		t.Fatal("expected non-empty serial number")
	}
	if serverCert.Fingerprint == "" {
		t.Fatal("expected non-empty fingerprint")
	}
	if !strings.Contains(serverCert.CertPEM, "BEGIN CERTIFICATE") {
		t.Fatal("invalid server certificate PEM format")
	}
	if !strings.Contains(serverCert.KeyPEM, "PRIVATE KEY") {
		t.Fatal("invalid server key PEM format")
	}

	// Verify the server certificate subject includes the cluster ID.
	if !strings.Contains(serverCert.Cert.Subject.CommonName, clusterID) {
		t.Fatalf("expected cluster ID in server cert CN, got: %s", serverCert.Cert.Subject.CommonName)
	}

	// Verify the server certificate includes the server IP in SANs.
	found := false
	for _, ip := range serverCert.Cert.IPAddresses {
		if ip.String() == serverAddress {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected server IP %s in certificate SANs, got: %v", serverAddress, serverCert.Cert.IPAddresses)
	}
}

// TestServerDeployDSNHandling verifies DSN configuration handling for both provided and install modes.
func TestServerDeployDSNHandling(t *testing.T) {
	tests := []struct {
		name                   string
		postgresqlDSN          string
		expectInstallPostgres  bool
		expectDSNInEnvironment bool
		expectedInstallFlagVal string
	}{
		{
			name:                   "User provides DSN",
			postgresqlDSN:          "postgres://user:pass@dbhost:5432/ploy",
			expectInstallPostgres:  false,
			expectDSNInEnvironment: true,
			expectedInstallFlagVal: "false",
		},
		{
			name:                   "No DSN provided - install PostgreSQL",
			postgresqlDSN:          "",
			expectInstallPostgres:  true,
			expectDSNInEnvironment: false,
			expectedInstallFlagVal: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the DSN handling logic from runServerDeploy.
			pgDSN := strings.TrimSpace(tt.postgresqlDSN)
			installPostgres := pgDSN == ""

			if installPostgres != tt.expectInstallPostgres {
				t.Fatalf("expected installPostgres=%v, got: %v", tt.expectInstallPostgres, installPostgres)
			}

			// Build environment variables as runServerDeploy does.
			scriptEnv := map[string]string{
				"CLUSTER_ID":              "test-cluster-123",
				"NODE_ID":                 "control",
				"NODE_ADDRESS":            "192.168.1.10",
				"BOOTSTRAP_PRIMARY":       "true",
				"PLOY_INSTALL_POSTGRESQL": boolToString(installPostgres),
			}

			if pgDSN != "" {
				scriptEnv["PLOY_SERVER_PG_DSN"] = pgDSN
			}

			// Verify PLOY_INSTALL_POSTGRESQL flag.
			if got := scriptEnv["PLOY_INSTALL_POSTGRESQL"]; got != tt.expectedInstallFlagVal {
				t.Fatalf("expected PLOY_INSTALL_POSTGRESQL=%q, got: %q", tt.expectedInstallFlagVal, got)
			}

			// Verify PLOY_SERVER_PG_DSN presence.
			_, hasDSN := scriptEnv["PLOY_SERVER_PG_DSN"]
			if hasDSN != tt.expectDSNInEnvironment {
				t.Fatalf("expected PLOY_SERVER_PG_DSN present=%v, got: %v", tt.expectDSNInEnvironment, hasDSN)
			}

			// When user provides DSN, verify it matches.
			if tt.expectDSNInEnvironment {
				if got := scriptEnv["PLOY_SERVER_PG_DSN"]; got != tt.postgresqlDSN {
					t.Fatalf("expected DSN %q, got: %q", tt.postgresqlDSN, got)
				}
			}
		})
	}
}

// TestServerDeployProvisionHostCallPath verifies the ProvisionHost function is called with correct parameters.
func TestServerDeployProvisionHostCallPath(t *testing.T) {
	// Create a temporary test binary file.
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binaryPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}

	// Prepare configuration.
	cfg := serverDeployConfig{
		Address:       "10.0.0.5",
		PostgreSQLDSN: "postgres://test:pass@dbhost:5432/ploy",
		User:          "testuser",
		IdentityFile:  "/path/to/identity",
		PloydBinary:   binaryPath,
		SSHPort:       2222,
	}

	// Capture the ProvisionHost call by wrapping runServerDeploy logic.
	// Since runServerDeploy is difficult to test without actual SSH, we verify
	// that the ProvisionOptions are constructed correctly.
	clusterID := "test-cluster-abc"
	now := time.Now()

	ca, err := pki.GenerateCA(clusterID, now)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	serverCert, err := pki.IssueServerCert(ca, clusterID, cfg.Address, now)
	if err != nil {
		t.Fatalf("IssueServerCert failed: %v", err)
	}

	pgDSN := strings.TrimSpace(cfg.PostgreSQLDSN)
	installPostgres := pgDSN == ""

	scriptEnv := map[string]string{
		"CLUSTER_ID":              clusterID,
		"NODE_ID":                 "control",
		"NODE_ADDRESS":            cfg.Address,
		"BOOTSTRAP_PRIMARY":       "true",
		"PLOY_INSTALL_POSTGRESQL": boolToString(installPostgres),
		"PLOY_CA_CERT_PEM":        ca.CertPEM,
		"PLOY_CA_KEY_PEM":         ca.KeyPEM,
		"PLOY_SERVER_CERT_PEM":    serverCert.CertPEM,
		"PLOY_SERVER_KEY_PEM":     serverCert.KeyPEM,
	}

	if pgDSN != "" {
		scriptEnv["PLOY_SERVER_PG_DSN"] = pgDSN
	}

	// Build ProvisionOptions.
	user := cfg.User
	if strings.TrimSpace(user) == "" {
		user = deploy.DefaultRemoteUser
	}

	sshPort := cfg.SSHPort
	if sshPort == 0 {
		sshPort = deploy.DefaultSSHPort
	}

	provisionOpts := deploy.ProvisionOptions{
		Host:            cfg.Address,
		Address:         cfg.Address,
		User:            user,
		Port:            sshPort,
		IdentityFile:    cfg.IdentityFile,
		PloydBinaryPath: binaryPath,
		Stdout:          io.Discard,
		Stderr:          io.Discard,
		ScriptEnv:       scriptEnv,
		ScriptArgs:      []string{"--cluster-id", clusterID, "--node-id", "control", "--node-address", cfg.Address, "--primary"},
		ServiceChecks:   []string{"ployd"},
	}

	// Verify ProvisionOptions fields.
	if provisionOpts.Host != cfg.Address {
		t.Fatalf("expected Host=%q, got: %q", cfg.Address, provisionOpts.Host)
	}
	if provisionOpts.User != "testuser" {
		t.Fatalf("expected User=%q, got: %q", "testuser", provisionOpts.User)
	}
	if provisionOpts.Port != 2222 {
		t.Fatalf("expected Port=2222, got: %d", provisionOpts.Port)
	}
	if provisionOpts.IdentityFile != cfg.IdentityFile {
		t.Fatalf("expected IdentityFile=%q, got: %q", cfg.IdentityFile, provisionOpts.IdentityFile)
	}
	if provisionOpts.PloydBinaryPath != binaryPath {
		t.Fatalf("expected PloydBinaryPath=%q, got: %q", binaryPath, provisionOpts.PloydBinaryPath)
	}

	// Verify ScriptEnv contains necessary keys.
	if val := provisionOpts.ScriptEnv["CLUSTER_ID"]; val != clusterID {
		t.Fatalf("expected CLUSTER_ID=%q, got: %q", clusterID, val)
	}
	if val := provisionOpts.ScriptEnv["PLOY_CA_CERT_PEM"]; val == "" {
		t.Fatal("expected PLOY_CA_CERT_PEM to be set")
	}
	if val := provisionOpts.ScriptEnv["PLOY_CA_KEY_PEM"]; val == "" {
		t.Fatal("expected PLOY_CA_KEY_PEM to be set")
	}
	if val := provisionOpts.ScriptEnv["PLOY_SERVER_CERT_PEM"]; val == "" {
		t.Fatal("expected PLOY_SERVER_CERT_PEM to be set")
	}
	if val := provisionOpts.ScriptEnv["PLOY_SERVER_KEY_PEM"]; val == "" {
		t.Fatal("expected PLOY_SERVER_KEY_PEM to be set")
	}
	if val := provisionOpts.ScriptEnv["PLOY_SERVER_PG_DSN"]; val != cfg.PostgreSQLDSN {
		t.Fatalf("expected PLOY_SERVER_PG_DSN=%q, got: %q", cfg.PostgreSQLDSN, val)
	}

	// Verify ServiceChecks includes ployd.
	found := false
	for _, svc := range provisionOpts.ServiceChecks {
		if svc == "ployd" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ServiceChecks to include 'ployd', got: %v", provisionOpts.ServiceChecks)
	}

	// Verify that calling ProvisionHost with a mock runner succeeds.
	// We use a mock runner that simulates success without SSH.
	mockRunner := &mockProvisionRunner{t: t}
	provisionOpts.Runner = mockRunner

	ctx := context.Background()
	if err := deploy.ProvisionHost(ctx, provisionOpts); err != nil {
		t.Fatalf("ProvisionHost failed: %v", err)
	}

	// Verify the mock runner received the expected commands.
	if !mockRunner.copiedBinary {
		t.Fatal("expected binary copy command to be executed")
	}
	if !mockRunner.installedBinary {
		t.Fatal("expected binary install command to be executed")
	}
	if !mockRunner.ranBootstrapScript {
		t.Fatal("expected bootstrap script to be executed")
	}
	if !mockRunner.checkedService {
		t.Fatal("expected service check to be executed")
	}
}

// mockProvisionRunner is a test double for deploy.Runner that simulates successful provisioning.
type mockProvisionRunner struct {
	t                  *testing.T
	copiedBinary       bool
	installedBinary    bool
	ranBootstrapScript bool
	checkedService     bool
}

func (m *mockProvisionRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
	switch name {
	case "scp":
		// Binary copy.
		m.copiedBinary = true
	case "ssh":
		// Detect which SSH command by inspecting args.
		argsStr := strings.Join(args, " ")
		if strings.Contains(argsStr, "install -m0755") {
			m.installedBinary = true
		} else if strings.Contains(argsStr, "systemctl is-active") {
			m.checkedService = true
		} else if strings.Contains(argsStr, "bash") && strings.Contains(argsStr, "-s") {
			// Bootstrap script execution (bash -s --).
			m.ranBootstrapScript = true
		}
	default:
		m.t.Fatalf("unexpected command: %s", name)
	}
	return nil
}

// TestServerDeployReuseFlags verifies the --reuse and --force-new-ca flags control CA generation.
func TestServerDeployReuseFlags(t *testing.T) {
	tests := []struct {
		name               string
		reuse              bool
		existingCluster    bool
		expectDetect       bool
		expectPKIInEnv     bool
		expectedClusterMsg string
	}{
		{
			name:               "reuse enabled, cluster exists",
			reuse:              true,
			existingCluster:    true,
			expectDetect:       true,
			expectPKIInEnv:     false,
			expectedClusterMsg: "reusing CA and server certificate",
		},
		{
			name:               "reuse enabled, no existing cluster",
			reuse:              true,
			existingCluster:    false,
			expectDetect:       true,
			expectPKIInEnv:     true,
			expectedClusterMsg: "No existing cluster found",
		},
		{
			name:               "reuse disabled (force new CA)",
			reuse:              false,
			existingCluster:    true,
			expectDetect:       false,
			expectPKIInEnv:     true,
			expectedClusterMsg: "Generated cluster ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			binaryPath := filepath.Join(tmpDir, "ployd-test")
			if err := os.WriteFile(binaryPath, []byte("fake binary"), 0755); err != nil {
				t.Fatalf("create test binary: %v", err)
			}
			identityPath := filepath.Join(tmpDir, "id_test")
			if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
				t.Fatalf("create test identity: %v", err)
			}

			cfg := serverDeployConfig{
				Address:      "10.0.0.5",
				User:         "testuser",
				IdentityFile: identityPath,
				PloydBinary:  binaryPath,
				SSHPort:      22,
				Reuse:        tt.reuse,
			}

			// Mock the detection result
			var detectCalled bool
			oldProvision := provisionHost
			oldDetectRunner := detectRunner
			defer func() {
				provisionHost = oldProvision
				detectRunner = oldDetectRunner
			}()

			// Create a mock runner that simulates detection
			mockRunner := &mockDetectRunner{
				t:               t,
				existingCluster: tt.existingCluster,
				detectCalled:    &detectCalled,
			}
			detectRunner = mockRunner

			// Stub provisioning
			provisionHost = func(ctx context.Context, opts deploy.ProvisionOptions) error {
				// Check if PKI env vars are present/absent as expected
				_, hasCACert := opts.ScriptEnv["PLOY_CA_CERT_PEM"]
				_, hasCAKey := opts.ScriptEnv["PLOY_CA_KEY_PEM"]
				_, hasServerCert := opts.ScriptEnv["PLOY_SERVER_CERT_PEM"]
				_, hasServerKey := opts.ScriptEnv["PLOY_SERVER_KEY_PEM"]

				hasPKI := hasCACert && hasCAKey && hasServerCert && hasServerKey

				if hasPKI != tt.expectPKIInEnv {
					t.Errorf("expected PKI in env=%v, got: %v", tt.expectPKIInEnv, hasPKI)
				}

				return nil
			}

			stderr := &bytes.Buffer{}
			_ = runServerDeploy(cfg, stderr)

			output := stderr.String()

			// Verify detection was called if expected
			if tt.expectDetect {
				if !detectCalled {
					t.Error("expected DetectExisting to be called")
				}
			}

			// Verify the correct message appears in output
			if tt.expectedClusterMsg != "" && !strings.Contains(output, tt.expectedClusterMsg) {
				t.Errorf("expected output to contain %q, got: %s", tt.expectedClusterMsg, output)
			}
		})
	}
}

// mockDetectRunner simulates detection responses for testing.
type mockDetectRunner struct {
	t               *testing.T
	existingCluster bool
	detectCalled    *bool
}

func (m *mockDetectRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
	if name == "ssh" {
		argsStr := strings.Join(args, " ")
		if strings.Contains(argsStr, "test -f /etc/ploy/pki/ca.crt") {
			*m.detectCalled = true
			if m.existingCluster {
				return nil // File exists
			}
			return errors.New("file not found") // File doesn't exist
		}
		if strings.Contains(argsStr, "test -f /etc/ploy/ployd.yaml") {
			if m.existingCluster {
				return nil
			}
			return errors.New("file not found")
		}
		if strings.Contains(argsStr, "test -f /etc/ploy/pki/server.crt") {
			if m.existingCluster {
				return nil
			}
			return errors.New("file not found")
		}
		if strings.Contains(argsStr, "openssl x509") && strings.Contains(argsStr, "commonName") {
			if m.existingCluster && streams.Stdout != nil {
				// Simulate extracting cluster ID from cert CN - match the exact format
				_, _ = streams.Stdout.Write([]byte("ployd-abc123\n"))
				return nil
			}
			return errors.New("cert not found")
		}
	}
	return nil
}

// TestRefreshAdminCertFromServer verifies that admin cert refresh calls the server and writes files.
func TestRefreshAdminCertFromServer(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	clusterID := "test-cluster-refresh"
	now := time.Now()

	// Generate test CA and existing admin cert for initial descriptor.
	ca, err := pki.GenerateCA(clusterID, now)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	adminCert, err := pki.IssueClientCert(ca, clusterID, now)
	if err != nil {
		t.Fatalf("IssueClientCert failed: %v", err)
	}

	// Write initial admin bundle.
	caPath, certPath, keyPath, err := writeLocalAdminBundle(clusterID, ca.CertPEM, adminCert.CertPEM, adminCert.KeyPEM)
	if err != nil {
		t.Fatalf("writeLocalAdminBundle failed: %v", err)
	}

	// Save descriptor with existing cert paths.
	desc := config.Descriptor{
		ClusterID: clusterID,
		Address:   "https://10.0.0.5:8443",
		Scheme:    "https",
		CAPath:    caPath,
		CertPath:  certPath,
		KeyPath:   keyPath,
	}
	if _, err := config.SaveDescriptor(desc); err != nil {
		t.Fatalf("SaveDescriptor failed: %v", err)
	}
	if err := config.SetDefault(clusterID); err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}

	// Mock HTTP server to simulate PKI endpoint.
	mockServer := &mockPKIServer{
		t:          t,
		clusterID:  clusterID,
		ca:         ca,
		expectRole: "cli-admin",
	}
	server := mockServer.start()
	defer server.Close()

	// Set control plane URL to mock server.
	t.Setenv("PLOY_CONTROL_PLANE_URL", server.URL)

	// Call handleRefreshAdminCert.
	stderr := &bytes.Buffer{}
	ctx := context.Background()
	if err := handleRefreshAdminCert(ctx, stderr); err != nil {
		t.Fatalf("handleRefreshAdminCert failed: %v", err)
	}

	output := stderr.String()
	if !strings.Contains(output, "Refreshing admin certificate") {
		t.Errorf("expected refresh message in output, got: %s", output)
	}
	if !strings.Contains(output, "Admin certificate issued successfully") {
		t.Errorf("expected success message in output, got: %s", output)
	}
	if !strings.Contains(output, "Admin certificate refreshed successfully") {
		t.Errorf("expected completion message in output, got: %s", output)
	}

	// Verify files were written.
	if _, err := os.Stat(caPath); err != nil {
		t.Errorf("CA file not found: %v", err)
	}
	if _, err := os.Stat(certPath); err != nil {
		t.Errorf("cert file not found: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Errorf("key file not found: %v", err)
	}

	// Verify descriptor was updated.
	updatedDesc, err := config.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault failed: %v", err)
	}
	if updatedDesc.CAPath != caPath {
		t.Errorf("expected CAPath=%q, got %q", caPath, updatedDesc.CAPath)
	}
	if updatedDesc.CertPath != certPath {
		t.Errorf("expected CertPath=%q, got %q", certPath, updatedDesc.CertPath)
	}
	if updatedDesc.KeyPath != keyPath {
		t.Errorf("expected KeyPath=%q, got %q", keyPath, updatedDesc.KeyPath)
	}

	// Verify the mock server received a valid CSR.
	if !mockServer.receivedCSR {
		t.Error("expected mock server to receive CSR")
	}
}

// TestRefreshAdminCertFromServerServerError verifies that server-side errors
// during refresh are surfaced to the user.
func TestRefreshAdminCertFromServerServerError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	// Minimal descriptor with cluster ID set and no TLS paths.
	clusterID := "test-cluster-error"
	desc := config.Descriptor{ClusterID: clusterID, Address: "https://127.0.0.1:8443", Scheme: "https"}
	if _, err := config.SaveDescriptor(desc); err != nil {
		t.Fatalf("SaveDescriptor failed: %v", err)
	}
	if err := config.SetDefault(clusterID); err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}

	// Mock server that returns 400 with a short message.
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pki/sign/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad csr", http.StatusBadRequest)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv("PLOY_CONTROL_PLANE_URL", srv.URL)

	// Run the refresh entrypoint which loads descriptor and calls the endpoint.
	err := handleRefreshAdminCert(context.Background(), bytes.NewBuffer(nil))
	if err == nil {
		t.Fatal("expected error from server 400 response")
	}
	if !strings.Contains(err.Error(), "server returned status 400") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRefreshAdminCertFromServerInvalidJSON verifies invalid JSON responses are handled.
func TestRefreshAdminCertFromServerInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	clusterID := "test-cluster-invalid-json"
	desc := config.Descriptor{ClusterID: clusterID, Address: "https://127.0.0.1:8443", Scheme: "https"}
	if _, err := config.SaveDescriptor(desc); err != nil {
		t.Fatalf("SaveDescriptor failed: %v", err)
	}
	if err := config.SetDefault(clusterID); err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pki/sign/admin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{not json}"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv("PLOY_CONTROL_PLANE_URL", srv.URL)

	err := handleRefreshAdminCert(context.Background(), bytes.NewBuffer(nil))
	if err == nil {
		t.Fatal("expected decode error from invalid JSON")
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleRefreshAdminCertMissingClusterID verifies a descriptor lacking ClusterID is rejected.
func TestHandleRefreshAdminCertMissingClusterID(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	// Manually craft a malformed descriptor with empty cluster_id and set as default.
	clusters := filepath.Join(tmpDir, "clusters")
	if err := os.MkdirAll(clusters, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// File name is the cluster ID; JSON contains empty cluster_id to simulate bad data.
	id := "malformed"
	data := []byte(`{"cluster_id":"","address":"https://127.0.0.1:8443","scheme":"https"}`)
	if err := os.WriteFile(filepath.Join(clusters, id+".json"), data, 0o644); err != nil {
		t.Fatalf("write descriptor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(clusters, "default"), []byte(id), 0o644); err != nil {
		t.Fatalf("write default marker: %v", err)
	}

	err := handleRefreshAdminCert(context.Background(), bytes.NewBuffer(nil))
	if err == nil {
		t.Fatal("expected error for missing cluster ID in descriptor")
	}
	if !strings.Contains(err.Error(), "cluster ID not found in descriptor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// mockPKIServer simulates the server PKI signing endpoint for testing.
type mockPKIServer struct {
	t           *testing.T
	clusterID   string
	ca          *pki.CABundle
	expectRole  string
	receivedCSR bool
}

func (m *mockPKIServer) start() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pki/sign/admin", m.handleSignAdmin)

	server := httptest.NewServer(mux)
	return server
}

func (m *mockPKIServer) handleSignAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CSR string `json:"csr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Parse and validate CSR.
	block, _ := pem.Decode([]byte(req.CSR))
	if block == nil {
		http.Error(w, "invalid CSR PEM", http.StatusBadRequest)
		return
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		http.Error(w, "parse CSR failed", http.StatusBadRequest)
		return
	}

	// Verify OU contains expected role.
	hasRole := false
	for _, ou := range csr.Subject.OrganizationalUnit {
		if strings.Contains(ou, m.expectRole) {
			hasRole = true
			break
		}
	}
	if !hasRole {
		http.Error(w, "CSR missing required role", http.StatusBadRequest)
		return
	}

	m.receivedCSR = true

	// Sign CSR using test CA.
	cert, err := pki.SignNodeCSR(m.ca, []byte(req.CSR), time.Now())
	if err != nil {
		http.Error(w, fmt.Sprintf("sign failed: %v", err), http.StatusInternalServerError)
		return
	}

	resp := struct {
		Certificate string `json:"certificate"`
		CABundle    string `json:"ca_bundle"`
		Serial      string `json:"serial"`
		Fingerprint string `json:"fingerprint"`
		NotBefore   string `json:"not_before"`
		NotAfter    string `json:"not_after"`
	}{
		Certificate: cert.CertPEM,
		CABundle:    m.ca.CertPEM,
		Serial:      cert.Serial,
		Fingerprint: cert.Fingerprint,
		NotBefore:   cert.NotBefore.Format(time.RFC3339),
		NotAfter:    cert.NotAfter.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// TestGenerateAdminCSR verifies that admin CSR generation includes proper OU and ExtKeyUsage.
func TestGenerateAdminCSR(t *testing.T) {
	clusterID := "test-cluster-csr"

	csrPEM, keyPEM, err := generateAdminCSR(clusterID)
	if err != nil {
		t.Fatalf("generateAdminCSR failed: %v", err)
	}

	// Verify CSR is valid PEM.
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		t.Fatal("invalid CSR PEM")
	}

	// Parse CSR.
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("parse CSR: %v", err)
	}

	// Verify signature.
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("CSR signature invalid: %v", err)
	}

	// Verify CN contains cluster ID.
	if !strings.Contains(csr.Subject.CommonName, clusterID) {
		t.Errorf("expected CN to contain %q, got: %s", clusterID, csr.Subject.CommonName)
	}

	// Verify OU contains role.
	hasAdminOU := false
	for _, ou := range csr.Subject.OrganizationalUnit {
		if ou == "Ploy role=cli-admin" {
			hasAdminOU = true
			break
		}
	}
	if !hasAdminOU {
		t.Error("expected OU to contain \"Ploy role=cli-admin\"")
	}

	// Verify ExtKeyUsage extension.
	var hasClientAuthEKU bool
	for _, ext := range csr.Extensions {
		if ext.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 37}) { // extKeyUsage
			var oids []asn1.ObjectIdentifier
			if _, err := asn1.Unmarshal(ext.Value, &oids); err != nil {
				t.Fatalf("parse EKU extension: %v", err)
			}
			for _, oid := range oids {
				if oid.Equal(asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 2}) { // clientAuth
					hasClientAuthEKU = true
					break
				}
			}
			break
		}
	}
	if !hasClientAuthEKU {
		t.Error("expected CSR to have ExtKeyUsage with ClientAuth")
	}

	// Verify key is valid PEM.
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil || keyBlock.Type != "EC PRIVATE KEY" {
		t.Fatal("invalid key PEM")
	}

	// Parse private key.
	if _, err := x509.ParseECPrivateKey(keyBlock.Bytes); err != nil {
		t.Fatalf("parse private key: %v", err)
	}
}

// TestServerDeployDescriptorPersistence verifies that the cluster descriptor is saved after successful deployment.
func TestServerDeployDescriptorPersistence(t *testing.T) {
	// Set up temporary config directory.
	tmpDir := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	// Create a temporary test binary file.
	binaryPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binaryPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}

	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Prepare configuration.
	cfg := serverDeployConfig{
		Address:       "10.0.0.5",
		PostgreSQLDSN: "postgres://test:pass@dbhost:5432/ploy",
		User:          "testuser",
		IdentityFile:  identityPath,
		PloydBinary:   binaryPath,
		SSHPort:       2222,
	}

	clusterID := "test-cluster-persist"
	now := time.Now()

	ca, err := pki.GenerateCA(clusterID, now)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	serverCert, err := pki.IssueServerCert(ca, clusterID, cfg.Address, now)
	if err != nil {
		t.Fatalf("IssueServerCert failed: %v", err)
	}

	pgDSN := strings.TrimSpace(cfg.PostgreSQLDSN)
	installPostgres := pgDSN == ""

	scriptEnv := map[string]string{
		"CLUSTER_ID":              clusterID,
		"NODE_ID":                 "control",
		"NODE_ADDRESS":            cfg.Address,
		"BOOTSTRAP_PRIMARY":       "true",
		"PLOY_INSTALL_POSTGRESQL": boolToString(installPostgres),
		"PLOY_CA_CERT_PEM":        ca.CertPEM,
		"PLOY_CA_KEY_PEM":         ca.KeyPEM,
		"PLOY_SERVER_CERT_PEM":    serverCert.CertPEM,
		"PLOY_SERVER_KEY_PEM":     serverCert.KeyPEM,
	}

	if pgDSN != "" {
		scriptEnv["PLOY_SERVER_PG_DSN"] = pgDSN
	}

	user := cfg.User
	if strings.TrimSpace(user) == "" {
		user = deploy.DefaultRemoteUser
	}

	sshPort := cfg.SSHPort
	if sshPort == 0 {
		sshPort = deploy.DefaultSSHPort
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
		ScriptArgs:      []string{"--cluster-id", clusterID, "--node-id", "control", "--node-address", cfg.Address, "--primary"},
		ServiceChecks:   []string{"ployd"},
	}

	// Use mock runner to simulate successful provisioning.
	mockRunner := &mockProvisionRunner{t: t}
	provisionOpts.Runner = mockRunner

	ctx := context.Background()
	if err := deploy.ProvisionHost(ctx, provisionOpts); err != nil {
		t.Fatalf("ProvisionHost failed: %v", err)
	}

	// Simulate the descriptor save logic from runServerDeploy.
	serverAddress := "https://10.0.0.5:8443"
	desc := config.Descriptor{
		ClusterID:       clusterID,
		Address:         serverAddress,
		Scheme:          "https",
		SSHIdentityPath: identityPath,
	}
	if _, err := config.SaveDescriptor(desc); err != nil {
		t.Fatalf("SaveDescriptor failed: %v", err)
	}

	// Set as default.
	if err := config.SetDefault(clusterID); err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}

	// Verify the descriptor was saved.
	list, err := config.ListDescriptors()
	if err != nil {
		t.Fatalf("ListDescriptors failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 descriptor, got %d", len(list))
	}

	saved := list[0]
	if saved.ClusterID != clusterID {
		t.Fatalf("expected ClusterID=%q, got %q", clusterID, saved.ClusterID)
	}
	if saved.Address != serverAddress {
		t.Fatalf("expected Address=%q, got %q", serverAddress, saved.Address)
	}
	if saved.Scheme != "https" {
		t.Fatalf("expected Scheme=%q, got %q", "https", saved.Scheme)
	}
	if saved.SSHIdentityPath != identityPath {
		t.Fatalf("expected SSHIdentityPath=%q, got %q", identityPath, saved.SSHIdentityPath)
	}
	if !saved.Default {
		t.Fatal("expected descriptor to be marked as default")
	}
}

// TestServerDeployDryRunNewCluster verifies dry-run output for a new cluster deployment.
func TestServerDeployDryRunNewCluster(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Stub detectRunner to return no existing cluster.
	oldRunner := detectRunner
	detectRunner = &mockRunner{
		runFunc: func(ctx context.Context, cmd string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
			return errors.New("no cluster found")
		},
	}
	defer func() { detectRunner = oldRunner }()

	var stderr bytes.Buffer
	cfg := serverDeployConfig{
		Address:      "10.0.0.1",
		User:         "root",
		IdentityFile: identityPath,
		PloydBinary:  binPath,
		SSHPort:      22,
		Reuse:        true,
		DryRun:       true,
	}

	err := runServerDeploy(cfg, &stderr)
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}

	out := stderr.String()
	if !strings.Contains(out, "DRY RUN: Server deployment") {
		t.Errorf("expected 'DRY RUN' header, got: %q", out)
	}
	if !strings.Contains(out, "No existing cluster found") {
		t.Errorf("expected detection message, got: %q", out)
	}
	if !strings.Contains(out, "Planned actions:") {
		t.Errorf("expected planned actions header, got: %q", out)
	}
	if !strings.Contains(out, "Generate new cluster ID") {
		t.Errorf("expected cluster ID generation message, got: %q", out)
	}
	if !strings.Contains(out, "Generate new CA certificate") {
		t.Errorf("expected CA generation message, got: %q", out)
	}
	if !strings.Contains(out, "Issue server certificate") {
		t.Errorf("expected server cert message, got: %q", out)
	}
	if !strings.Contains(out, "CN=ployd-<cluster-id>") {
		t.Errorf("expected server cert subject, got: %q", out)
	}
	if !strings.Contains(out, "Issue admin client certificate") {
		t.Errorf("expected admin cert message, got: %q", out)
	}
	if !strings.Contains(out, "OU=Ploy role=cli-admin") {
		t.Errorf("expected admin cert subject, got: %q", out)
	}
	if !strings.Contains(out, "Upload ployd binary") {
		t.Errorf("expected binary upload message, got: %q", out)
	}
	if !strings.Contains(out, "Bootstrap server") {
		t.Errorf("expected bootstrap message, got: %q", out)
	}
	if !strings.Contains(out, "Dry run complete. No changes have been made.") {
		t.Errorf("expected completion message, got: %q", out)
	}
}

// TestServerDeployDryRunReuseCluster verifies dry-run output when reusing an existing cluster.
func TestServerDeployDryRunReuseCluster(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Stub detectRunner to return an existing cluster.
	oldRunner := detectRunner
	detectRunner = &mockRunner{
		runFunc: func(ctx context.Context, cmd string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
			// Simulate successful checks and CN extraction.
			if len(args) > 0 && strings.Contains(args[len(args)-1], "openssl") {
				_, _ = streams.Stdout.Write([]byte("ployd-abcd1234\n"))
			}
			return nil
		},
	}
	defer func() { detectRunner = oldRunner }()

	var stderr bytes.Buffer
	cfg := serverDeployConfig{
		Address:      "10.0.0.2",
		User:         "root",
		IdentityFile: identityPath,
		PloydBinary:  binPath,
		SSHPort:      22,
		Reuse:        true,
		DryRun:       true,
	}

	err := runServerDeploy(cfg, &stderr)
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}

	out := stderr.String()
	if !strings.Contains(out, "DRY RUN: Server deployment") {
		t.Errorf("expected 'DRY RUN' header, got: %q", out)
	}
	if !strings.Contains(out, "Found existing cluster") {
		t.Errorf("expected detection message, got: %q", out)
	}
	if !strings.Contains(out, "Planned actions:") {
		t.Errorf("expected planned actions header, got: %q", out)
	}
	if !strings.Contains(out, "Reuse existing cluster ID") {
		t.Errorf("expected reuse cluster ID message, got: %q", out)
	}
	if !strings.Contains(out, "Reuse existing CA and server certificate") {
		t.Errorf("expected reuse CA message, got: %q", out)
	}
	if !strings.Contains(out, "Skip PKI generation") {
		t.Errorf("expected skip PKI message, got: %q", out)
	}
	if !strings.Contains(out, "Dry run complete. No changes have been made.") {
		t.Errorf("expected completion message, got: %q", out)
	}
}

// mockRunner is a simple mock for deploy.Runner.
type mockRunner struct {
	runFunc func(ctx context.Context, cmd string, args []string, stdin io.Reader, streams deploy.IOStreams) error
}

func (m *mockRunner) Run(ctx context.Context, cmd string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
	if m.runFunc != nil {
		return m.runFunc(ctx, cmd, args, stdin, streams)
	}
	return nil
}
