package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/internal/pki"
)

func TestServerDeployDescriptorPersistence(t *testing.T) {
	IsolatePloyConfigHomeAllowDefault(t)
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binaryPath, []byte("fake binary"), 0o755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("create identity: %v", err)
	}
	cfg := serverDeployConfig{Address: "10.0.0.5", User: "root", IdentityFile: identityPath, PloydBinary: binaryPath, SSHPort: 22}
	clusterID := "cluster-descriptor-xyz"
	now := time.Now()
	ca, err := pki.GenerateCA(clusterID, now)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}
	adminCert, err := pki.IssueClientCert(ca, clusterID, now)
	if err != nil {
		t.Fatalf("IssueClientCert failed: %v", err)
	}
	if _, _, _, err := writeLocalAdminBundle(clusterID, ca.CertPEM, adminCert.CertPEM, adminCert.KeyPEM); err != nil {
		t.Fatalf("writeLocalAdminBundle: %v", err)
	}
	// Build ProvisionOptions and run with mock
	user := cfg.User
	if user == "" {
		user = deploy.DefaultRemoteUser
	}
	sshPort := cfg.SSHPort
	if sshPort == 0 {
		sshPort = deploy.DefaultSSHPort
	}
	provisionOpts := deploy.ProvisionOptions{Host: cfg.Address, Address: cfg.Address, User: user, Port: sshPort, IdentityFile: identityPath, PloydBinaryPath: binaryPath, Stdout: io.Discard, Stderr: io.Discard, ScriptEnv: map[string]string{}, ScriptArgs: []string{"--cluster-id", clusterID, "--node-id", "control", "--node-address", cfg.Address, "--primary"}, ServiceChecks: []string{"ployd"}}
	mockRunner := &mockProvisionRunner{t: t}
	provisionOpts.Runner = mockRunner
	if err := deploy.ProvisionHost(context.Background(), provisionOpts); err != nil {
		t.Fatalf("ProvisionHost failed: %v", err)
	}
	serverAddress := "https://10.0.0.5:8443"
	desc := config.Descriptor{ClusterID: config.ClusterID(clusterID), Address: serverAddress, Scheme: "https", SSHIdentityPath: identityPath}
	if _, err := config.SaveDescriptor(desc); err != nil {
		t.Fatalf("SaveDescriptor failed: %v", err)
	}
	if err := config.SetDefault(config.ClusterID(clusterID)); err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}
	list, err := config.ListDescriptors()
	if err != nil {
		t.Fatalf("ListDescriptors failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 descriptor, got %d", len(list))
	}
	saved := list[0]
	if string(saved.ClusterID) != clusterID || saved.Address != serverAddress || saved.Scheme != "https" || saved.SSHIdentityPath != identityPath || !saved.Default {
		t.Fatal("descriptor fields do not match expected values")
	}
}
