package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/deploy"
)

func TestHandleServerDeployValidatesSSHPort(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0o755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	tests := []struct {
		name      string
		sshPort   int
		expectErr bool
	}{
		{"valid port 22", 22, false},
		{"valid port 2222", 2222, false},
		{"default port 0", 0, false},
		{"invalid port -1", -1, true},
		{"invalid port 99999", 99999, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			} else if err != nil && strings.Contains(err.Error(), "invalid SSH port") {
				t.Fatalf("unexpected SSH port validation error for valid port %d: %v", tt.sshPort, err)
			}
		})
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
		{"User provides DSN", "postgres://user:pass@dbhost:5432/ploy", false, true, "false"},
		{"No DSN provided - install PostgreSQL", "", true, false, "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pgDSN := strings.TrimSpace(tt.postgresqlDSN)
			installPostgres := pgDSN == ""
			if installPostgres != tt.expectInstallPostgres {
				t.Fatalf("expected installPostgres=%v, got: %v", tt.expectInstallPostgres, installPostgres)
			}
			scriptEnv := map[string]string{
				"CLUSTER_ID":              "test-cluster-123",
				"NODE_ID":                 "control",
				"NODE_ADDRESS":            "192.168.1.10",
				"BOOTSTRAP_PRIMARY":       "true",
				"PLOY_INSTALL_POSTGRESQL": boolToString(installPostgres),
			}
			if pgDSN != "" {
				scriptEnv["PLOY_POSTGRES_DSN"] = pgDSN
			}
			if got := scriptEnv["PLOY_INSTALL_POSTGRESQL"]; got != tt.expectedInstallFlagVal {
				t.Fatalf("expected PLOY_INSTALL_POSTGRESQL=%q, got: %q", tt.expectedInstallFlagVal, got)
			}
			_, hasDSN := scriptEnv["PLOY_POSTGRES_DSN"]
			if hasDSN != tt.expectDSNInEnvironment {
				t.Fatalf("expected PLOY_POSTGRES_DSN present=%v, got: %v", tt.expectDSNInEnvironment, hasDSN)
			}
			if tt.expectDSNInEnvironment && scriptEnv["PLOY_POSTGRES_DSN"] != tt.postgresqlDSN {
				t.Fatalf("expected DSN %q, got: %q", tt.postgresqlDSN, scriptEnv["PLOY_POSTGRES_DSN"])
			}
		})
	}
}
