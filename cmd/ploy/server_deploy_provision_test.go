package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/internal/pki"
)

// TestServerDeployProvisionHostCallPath verifies the ProvisionHost function is called with correct parameters.
func TestServerDeployProvisionHostCallPath(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binaryPath, []byte("fake binary"), 0o755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	cfg := serverDeployConfig{Address: "10.0.0.5", PostgreSQLDSN: "postgres://test:pass@dbhost:5432/ploy", User: "testuser", IdentityFile: "/path/to/identity", PloydBinary: binaryPath, SSHPort: 2222}
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
		scriptEnv["PLOY_POSTGRES_DSN"] = pgDSN
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
		IdentityFile:    cfg.IdentityFile,
		PloydBinaryPath: binaryPath,
		Stdout:          io.Discard,
		Stderr:          io.Discard,
		ScriptEnv:       scriptEnv,
		ScriptArgs:      []string{"--cluster-id", clusterID, "--node-id", "control", "--node-address", cfg.Address, "--primary"},
		ServiceChecks:   []string{"ployd"},
	}
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
	if provisionOpts.ScriptEnv["CLUSTER_ID"] != clusterID {
		t.Fatalf("expected CLUSTER_ID=%q, got: %q", clusterID, provisionOpts.ScriptEnv["CLUSTER_ID"])
	}
	for _, k := range []string{"PLOY_CA_CERT_PEM", "PLOY_CA_KEY_PEM", "PLOY_SERVER_CERT_PEM", "PLOY_SERVER_KEY_PEM"} {
		if provisionOpts.ScriptEnv[k] == "" {
			t.Fatalf("expected %s to be set", k)
		}
	}
	if provisionOpts.ScriptEnv["PLOY_POSTGRES_DSN"] != cfg.PostgreSQLDSN {
		t.Fatalf("expected PLOY_POSTGRES_DSN=%q, got: %q", cfg.PostgreSQLDSN, provisionOpts.ScriptEnv["PLOY_POSTGRES_DSN"])
	}
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
	mockRunner := &mockProvisionRunner{t: t}
	provisionOpts.Runner = mockRunner
	if err := deploy.ProvisionHost(context.Background(), provisionOpts); err != nil {
		t.Fatalf("ProvisionHost failed: %v", err)
	}
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
