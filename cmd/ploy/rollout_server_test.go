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

func TestHandleRolloutRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleRollout(nil, buf)
	if err == nil {
		t.Fatalf("expected error for missing rollout subcommand")
	}
	out := buf.String()
	if !strings.Contains(out, "Usage: ploy rollout") {
		t.Fatalf("expected rollout usage output, got: %q", out)
	}
}

func TestHandleRolloutUnknownSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleRollout([]string{"unknown"}, buf)
	if err == nil {
		t.Fatalf("expected error for unknown rollout subcommand")
	}
	if !strings.Contains(err.Error(), "unknown rollout subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleRolloutServerRequiresAddress(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleRolloutServer(nil, buf)
	if err == nil {
		t.Fatalf("expected error when --address is missing")
	}
	if !strings.Contains(err.Error(), "address is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Usage: ploy rollout server") {
		t.Fatalf("expected rollout server usage output, got: %q", buf.String())
	}
}

func TestHandleRolloutServerRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleRolloutServer([]string{"--address", "1.2.3.4", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleRolloutServerValidatesSSHPort(t *testing.T) {
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
			// Stub rollout execution to avoid real scp/ssh attempts during tests.
			old := rolloutServerHost
			rolloutServerHost = func(ctx context.Context, opts rolloutServerOptions) error {
				return errors.New("rollout stubbed: skip remote calls")
			}
			defer func() { rolloutServerHost = old }()

			cfg := rolloutServerConfig{
				Address:      "10.0.0.5",
				User:         "testuser",
				IdentityFile: identityPath,
				BinaryPath:   binPath,
				SSHPort:      tt.sshPort,
			}

			err := runRolloutServer(cfg, io.Discard)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error for SSH port %d", tt.sshPort)
				}
				if !strings.Contains(err.Error(), "invalid SSH port") {
					t.Fatalf("expected SSH port validation error, got: %v", err)
				}
			} else {
				// For valid ports, we expect failure due to stubbed rollout,
				// but NOT due to port validation.
				if err != nil && strings.Contains(err.Error(), "invalid SSH port") {
					t.Fatalf("unexpected SSH port validation error for valid port %d: %v", tt.sshPort, err)
				}
			}
		})
	}
}

func TestHandleRolloutServerValidatesTimeout(t *testing.T) {
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
		timeout   int
		expectErr bool
	}{
		{"valid timeout 60", 60, false},
		{"valid timeout 120", 120, false},
		{"default timeout 0", 0, false}, // Timeout 0 defaults to 60, which is valid.
		{"invalid timeout -1", -1, true},
		{"invalid timeout -100", -100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Stub rollout execution to avoid real scp/ssh attempts during tests.
			old := rolloutServerHost
			rolloutServerHost = func(ctx context.Context, opts rolloutServerOptions) error {
				return errors.New("rollout stubbed: skip remote calls")
			}
			defer func() { rolloutServerHost = old }()

			cfg := rolloutServerConfig{
				Address:      "10.0.0.5",
				User:         "testuser",
				IdentityFile: identityPath,
				BinaryPath:   binPath,
				Timeout:      tt.timeout,
			}

			err := runRolloutServer(cfg, io.Discard)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error for timeout %d", tt.timeout)
				}
				if !strings.Contains(err.Error(), "timeout must be positive") {
					t.Fatalf("expected timeout validation error, got: %v", err)
				}
			} else {
				// For valid timeouts, we expect failure due to stubbed rollout,
				// but NOT due to timeout validation.
				if err != nil && strings.Contains(err.Error(), "timeout must be positive") {
					t.Fatalf("unexpected timeout validation error for valid timeout %d: %v", tt.timeout, err)
				}
			}
		})
	}
}

func TestExecuteRolloutServerCommandSequence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rollout command sequence test in short mode")
	}

	// Create a recording runner to track command execution.
	var calls [][]string
	runner := deploy.RunnerFunc(func(ctx context.Context, command string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
		call := append([]string{command}, args...)
		calls = append(calls, call)
		return nil
	})

	// Stub the rollout runner.
	old := rolloutRunner
	rolloutRunner = runner
	defer func() { rolloutRunner = old }()

	opts := rolloutServerOptions{
		Address:         "10.0.0.5",
		User:            "testuser",
		Port:            22,
		IdentityFile:    "/tmp/id_test",
		PloydBinaryPath: "/tmp/ployd-test",
		Stdout:          io.Discard,
		Stderr:          io.Discard,
	}

	ctx := context.Background()
	_ = executeRolloutServer(ctx, opts)

	// Verify expected command sequence:
	// 1. scp (upload binary)
	// 2. ssh (install binary)
	// 3. ssh (restart service)
	// 4. ssh (check service active) - may be called multiple times
	// 5. ssh (verify port) - optional
	if len(calls) < 4 {
		t.Fatalf("expected at least 4 commands (scp, ssh install, ssh restart, ssh check), got %d", len(calls))
	}

	// Check first command is scp.
	if calls[0][0] != "scp" {
		t.Fatalf("expected first command to be scp, got: %v", calls[0])
	}

	// Check second command is ssh with install.
	if calls[1][0] != "ssh" {
		t.Fatalf("expected second command to be ssh, got: %v", calls[1])
	}
	installCmd := strings.Join(calls[1], " ")
	if !strings.Contains(installCmd, "install") {
		t.Fatalf("expected second command to contain 'install', got: %v", calls[1])
	}

	// Check third command is ssh with systemctl restart.
	if calls[2][0] != "ssh" {
		t.Fatalf("expected third command to be ssh, got: %v", calls[2])
	}
	restartCmd := strings.Join(calls[2], " ")
	if !strings.Contains(restartCmd, "systemctl restart") {
		t.Fatalf("expected third command to contain 'systemctl restart', got: %v", calls[2])
	}

	// Check subsequent commands are ssh (health checks).
	for i := 3; i < len(calls); i++ {
		if calls[i][0] != "ssh" {
			t.Fatalf("expected command %d to be ssh, got: %v", i, calls[i])
		}
	}

	// Verify the verify-port check targets 8443.
	var sawPortCheck bool
	for i := 3; i < len(calls); i++ {
		joined := strings.Join(calls[i], " ")
		if strings.Contains(joined, ":8443") && strings.Contains(joined, "grep") {
			sawPortCheck = true
			break
		}
	}
	if !sawPortCheck {
		t.Fatalf("expected a verify-port check for :8443 in SSH calls, got: %v", calls)
	}
}

func TestExecuteRolloutServerWithRetry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rollout retry test in short mode")
	}

	// Create a runner that fails service checks initially then succeeds.
	var callCount int
	runner := deploy.RunnerFunc(func(ctx context.Context, command string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
		callCount++
		// Let upload and install succeed.
		if command == "scp" {
			return nil
		}
		cmdStr := strings.Join(args, " ")
		if strings.Contains(cmdStr, "install") {
			return nil
		}
		if strings.Contains(cmdStr, "systemctl restart") {
			return nil
		}
		// Fail first service check, succeed on second.
		if strings.Contains(cmdStr, "systemctl is-active") {
			if callCount <= 4 {
				return errors.New("service not active yet")
			}
			return nil
		}
		return nil
	})

	old := rolloutRunner
	rolloutRunner = runner
	defer func() { rolloutRunner = old }()

	opts := rolloutServerOptions{
		Address:         "10.0.0.5",
		User:            "testuser",
		Port:            22,
		IdentityFile:    "/tmp/id_test",
		PloydBinaryPath: "/tmp/ployd-test",
		Stdout:          io.Discard,
		Stderr:          io.Discard,
	}

	ctx := context.Background()
	err := executeRolloutServer(ctx, opts)
	if err != nil {
		t.Fatalf("expected rollout to succeed after retry, got: %v", err)
	}

	// Verify retry happened (call count > 4 means at least one retry).
	if callCount <= 4 {
		t.Fatalf("expected retry for service check, got %d calls", callCount)
	}
}

// TestRolloutServerDryRun verifies dry-run output for server rollout.
func TestRolloutServerDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	var stderr bytes.Buffer
	cfg := rolloutServerConfig{
		Address:      "10.0.0.3",
		User:         "root",
		IdentityFile: identityPath,
		BinaryPath:   binPath,
		SSHPort:      22,
		Timeout:      60,
		DryRun:       true,
	}

	err := runRolloutServer(cfg, &stderr)
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}

	out := stderr.String()
	if !strings.Contains(out, "DRY RUN: Rollout Ploy server") {
		t.Errorf("expected 'DRY RUN' header, got: %q", out)
	}
	if !strings.Contains(out, "Planned actions:") {
		t.Errorf("expected planned actions header, got: %q", out)
	}
	if !strings.Contains(out, "Upload new ployd binary") {
		t.Errorf("expected upload message, got: %q", out)
	}
	if !strings.Contains(out, "Install binary to") {
		t.Errorf("expected install message, got: %q", out)
	}
	if !strings.Contains(out, "Restart ployd service") {
		t.Errorf("expected restart message, got: %q", out)
	}
	if !strings.Contains(out, "Wait for service to become active") {
		t.Errorf("expected health check message, got: %q", out)
	}
	if !strings.Contains(out, "Verify service is listening on port 8443") {
		t.Errorf("expected port verification message, got: %q", out)
	}
	if !strings.Contains(out, "Dry run complete. No changes have been made.") {
		t.Errorf("expected completion message, got: %q", out)
	}
}
