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

	"github.com/iw2rmb/ploy/internal/deploy"
)

func TestServerDeployReuseFlags(t *testing.T) {
	tests := []struct {
		name               string
		reuse              bool
		existingCluster    bool
		expectDetect       bool
		expectPKIInEnv     bool
		expectedClusterMsg string
	}{
		{"reuse enabled, cluster exists", true, true, true, false, "reusing CA and server certificate"},
		{"reuse enabled, no existing cluster", true, false, true, true, "No existing cluster found"},
		{"reuse disabled (force new CA)", false, true, false, true, "Generated cluster ID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			binaryPath := filepath.Join(tmpDir, "ployd-test")
			if err := os.WriteFile(binaryPath, []byte("fake binary"), 0o755); err != nil {
				t.Fatalf("create test binary: %v", err)
			}
			identityPath := filepath.Join(tmpDir, "id_test")
			if err := os.WriteFile(identityPath, []byte("fake key"), 0o600); err != nil {
				t.Fatalf("create test identity: %v", err)
			}
			cfg := serverDeployConfig{Address: "10.0.0.5", User: "testuser", IdentityFile: identityPath, PloydBinary: binaryPath, SSHPort: 22, Reuse: tt.reuse}
			var detectCalled bool
			oldProvision := provisionHost
			oldDetectRunner := detectRunner
			defer func() { provisionHost = oldProvision; detectRunner = oldDetectRunner }()
			mockDet := &mockDetectRunner{t: t, existingCluster: tt.existingCluster, detectCalled: &detectCalled}
			detectRunner = mockDet
			provisionHost = func(ctx context.Context, opts deploy.ProvisionOptions) error { return nil }
			stderr := &bytes.Buffer{}
			_ = runServerDeploy(cfg, stderr)
			if tt.expectDetect && !detectCalled {
				t.Error("expected detection to run")
			}
			if !tt.expectDetect && detectCalled {
				t.Error("did not expect detection to run")
			}
			output := stderr.String()
			if tt.expectedClusterMsg != "" && !strings.Contains(output, tt.expectedClusterMsg) {
				t.Errorf("expected output to contain %q, got: %s", tt.expectedClusterMsg, output)
			}
		})
	}
}

func TestServerDeployDryRunNewCluster(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0o755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}
	oldRunner := detectRunner
	detectRunner = &mockRunner{runFunc: func(ctx context.Context, cmd string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
		return errors.New("no cluster found")
	}}
	defer func() { detectRunner = oldRunner }()
	var stderr bytes.Buffer
	cfg := serverDeployConfig{Address: "10.0.0.1", User: "root", IdentityFile: identityPath, PloydBinary: binPath, SSHPort: 22, Reuse: true, DryRun: true}
	if err := runServerDeploy(cfg, &stderr); err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
	out := stderr.String()
	for _, s := range []string{"DRY RUN: Server deployment", "No existing cluster found", "Planned actions:", "Generate new cluster ID", "Generate new CA certificate", "Issue server certificate", "CN=ployd-<cluster-id>", "Issue admin client certificate", "OU=Ploy role=cli-admin", "Upload ployd binary", "Bootstrap server", "Dry run complete. No changes have been made."} {
		if !strings.Contains(out, s) {
			t.Errorf("expected %q in output", s)
		}
	}
}

func TestServerDeployDryRunReuseCluster(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0o755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}
	oldRunner := detectRunner
	detectRunner = &mockRunner{runFunc: func(ctx context.Context, cmd string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
		// Simulate existing cluster detection: all SSH commands succeed, and CN extraction returns a cluster ID.
		if len(args) > 0 && strings.Contains(strings.Join(args, " "), "commonName") {
			// Return the cluster ID in CN format (ployd-<clusterID>).
			if streams.Stdout != nil {
				_, _ = io.WriteString(streams.Stdout, "ployd-testcluster123")
			}
		}
		return nil
	}}
	defer func() { detectRunner = oldRunner }()
	var stderr bytes.Buffer
	cfg := serverDeployConfig{Address: "10.0.0.1", User: "root", IdentityFile: identityPath, PloydBinary: binPath, SSHPort: 22, Reuse: true, DryRun: true}
	if err := runServerDeploy(cfg, &stderr); err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
	out := stderr.String()
	for _, s := range []string{"DRY RUN: Server deployment", "Found existing cluster", "Planned actions:", "Reuse existing cluster ID", "Reuse existing CA and server certificate", "Skip PKI generation", "Dry run complete. No changes have been made."} {
		if !strings.Contains(out, s) {
			t.Errorf("expected %q in output", s)
		}
	}
}
