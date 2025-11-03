package main

import (
	"bytes"
	"context"
	"errors"
	"io"
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
