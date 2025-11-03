package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/deploy"
)

// rolloutServerHost is an indirection to allow tests to stub remote commands.
var rolloutServerHost = executeRolloutServer

// rolloutRunner allows tests to inject a mock runner. Default is nil (uses system runner).
var rolloutRunner deploy.Runner

func handleRollout(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printRolloutUsage(stderr)
		return errors.New("rollout subcommand required")
	}
	switch args[0] {
	case "server":
		return handleRolloutServer(args[1:], stderr)
	case "nodes":
		return handleRolloutNodes(args[1:], stderr)
	default:
		printRolloutUsage(stderr)
		return fmt.Errorf("unknown rollout subcommand %q", args[0])
	}
}

func handleRolloutServer(args []string, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("rollout server", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		address  stringValue
		binary   stringValue
		identity stringValue
		userFlag stringValue
		sshPort  intValue
		timeout  intValue
	)

	fs.Var(&address, "address", "Target server host or IP address")
	fs.Var(&binary, "binary", "Path to the ployd binary for upload (default: alongside the CLI)")
	fs.Var(&identity, "identity", "SSH private key used for server connection (default: ~/.ssh/id_rsa)")
	fs.Var(&userFlag, "user", "SSH username for server connection (default: root)")
	fs.Var(&sshPort, "ssh-port", "SSH port for server connection (default: 22)")
	fs.Var(&timeout, "timeout", "Timeout in seconds for the rollout operation (default: 60)")

	if err := fs.Parse(args); err != nil {
		printRolloutServerUsage(stderr)
		return err
	}
	if fs.NArg() > 0 {
		printRolloutServerUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !address.set || strings.TrimSpace(address.value) == "" {
		printRolloutServerUsage(stderr)
		return errors.New("address is required")
	}

	cfg := rolloutServerConfig{
		Address:      address.value,
		BinaryPath:   binary.value,
		User:         userFlag.value,
		IdentityFile: identity.value,
		SSHPort:      sshPort.value,
		Timeout:      timeout.value,
	}

	return runRolloutServer(cfg, stderr)
}

func printRolloutUsage(w io.Writer) {
	printCommandUsage(w, "rollout")
}

func printRolloutServerUsage(w io.Writer) {
	printCommandUsage(w, "rollout", "server")
}

type rolloutServerConfig struct {
	Address      string
	BinaryPath   string
	User         string
	IdentityFile string
	SSHPort      int
	Timeout      int
}

func runRolloutServer(cfg rolloutServerConfig, stderr io.Writer) error {
	if stderr == nil {
		stderr = os.Stderr
	}

	// Resolve default paths.
	identityPath, err := resolveIdentityPath(stringValue{set: cfg.IdentityFile != "", value: cfg.IdentityFile})
	if err != nil {
		return fmt.Errorf("rollout server: %w", err)
	}

	ploydBinaryPath, err := resolvePloydBinaryPath(stringValue{set: cfg.BinaryPath != "", value: cfg.BinaryPath})
	if err != nil {
		return fmt.Errorf("rollout server: %w", err)
	}

	user := cfg.User
	if strings.TrimSpace(user) == "" {
		user = deploy.DefaultRemoteUser
	}

	sshPort := cfg.SSHPort
	if sshPort == 0 {
		sshPort = deploy.DefaultSSHPort
	}
	if err := validateSSHPort(sshPort); err != nil {
		return fmt.Errorf("rollout server: %w", err)
	}

	timeoutSecs := cfg.Timeout
	if timeoutSecs == 0 {
		timeoutSecs = 60
	}
	if timeoutSecs < 1 {
		return fmt.Errorf("rollout server: timeout must be positive, got %d", timeoutSecs)
	}

	_, _ = fmt.Fprintf(stderr, "Rolling out Ploy server to %s\n", cfg.Address)
	_, _ = fmt.Fprintf(stderr, "  SSH User: %s\n", user)
	_, _ = fmt.Fprintf(stderr, "  SSH Port: %d\n", sshPort)
	_, _ = fmt.Fprintf(stderr, "  Identity: %s\n", identityPath)
	_, _ = fmt.Fprintf(stderr, "  Binary: %s\n", ploydBinaryPath)
	_, _ = fmt.Fprintf(stderr, "  Timeout: %ds\n", timeoutSecs)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	return rolloutServerHost(ctx, rolloutServerOptions{
		Address:         cfg.Address,
		User:            user,
		Port:            sshPort,
		IdentityFile:    identityPath,
		PloydBinaryPath: ploydBinaryPath,
		Stdout:          os.Stdout,
		Stderr:          stderr,
	})
}

type rolloutServerOptions struct {
	Address         string
	User            string
	Port            int
	IdentityFile    string
	PloydBinaryPath string
	Stdout          io.Writer
	Stderr          io.Writer
}

func executeRolloutServer(ctx context.Context, opts rolloutServerOptions) error {
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}

	runner := rolloutRunner
	if runner == nil {
		runner = deploy.NewSystemRunner()
	}
	target := opts.Address
	if opts.User != "" {
		target = fmt.Sprintf("%s@%s", opts.User, opts.Address)
	}

	sshArgs := deploy.BuildSSHArgs(opts.IdentityFile, opts.Port)
	scpArgs := deploy.BuildScpArgs(opts.IdentityFile, opts.Port)
	streams := deploy.IOStreams{Stdout: stdout, Stderr: stderr}

	// Generate a random suffix for the uploaded binary.
	binarySuffix, err := deploy.RandomHexString(8)
	if err != nil {
		return fmt.Errorf("rollout server: generate binary suffix: %w", err)
	}
	remoteBinaryPath := fmt.Sprintf("/tmp/ployd-%s", binarySuffix)

	// Step 1: Upload the new binary via scp.
	_, _ = fmt.Fprintln(stderr, "Uploading new ployd binary...")
	copyBinaryArgs := append(append([]string(nil), scpArgs...), opts.PloydBinaryPath, fmt.Sprintf("%s:%s", target, remoteBinaryPath))
	if err := runner.Run(ctx, "scp", copyBinaryArgs, nil, streams); err != nil {
		return fmt.Errorf("rollout server: upload binary: %w", err)
	}

	// Step 2: Install the binary.
	_, _ = fmt.Fprintln(stderr, "Installing ployd binary...")
	installCmd := fmt.Sprintf("install -m0755 %s /usr/local/bin/ployd && rm -f %s", remoteBinaryPath, remoteBinaryPath)
	installArgs := append(append([]string(nil), sshArgs...), target, installCmd)
	if err := runner.Run(ctx, "ssh", installArgs, nil, streams); err != nil {
		return fmt.Errorf("rollout server: install binary: %w", err)
	}

	// Step 3: Restart the ployd service.
	_, _ = fmt.Fprintln(stderr, "Restarting ployd service...")
	restartCmd := "systemctl restart ployd"
	restartArgs := append(append([]string(nil), sshArgs...), target, restartCmd)
	if err := runner.Run(ctx, "ssh", restartArgs, nil, streams); err != nil {
		return fmt.Errorf("rollout server: restart service: %w", err)
	}

	// Step 4: Poll for service to become active.
	_, _ = fmt.Fprintln(stderr, "Waiting for ployd service to become active...")
	if err := pollServiceActive(ctx, runner, sshArgs, target, "ployd", streams); err != nil {
		return fmt.Errorf("rollout server: service health check: %w", err)
	}

	// Step 5: Verify the service is listening on the expected API port (8443).
	_, _ = fmt.Fprintln(stderr, "Verifying service is listening on port 8443...")
	verifyCmd := "ss -tlnp | grep :8443 || netstat -tlnp | grep :8443"
	verifyArgs := append(append([]string(nil), sshArgs...), target, verifyCmd)
	if err := runner.Run(ctx, "ssh", verifyArgs, nil, streams); err != nil {
		_, _ = fmt.Fprintf(stderr, "Warning: could not verify port 8443 is listening: %v\n", err)
		_, _ = fmt.Fprintln(stderr, "Continuing anyway; service may still be initializing...")
	}

	_, _ = fmt.Fprintln(stderr, "\nRollout complete!")
	_, _ = fmt.Fprintf(stderr, "Server %s has been updated successfully.\n", opts.Address)

	return nil
}

// pollServiceActive polls systemctl is-active until the service is active or context expires.
func pollServiceActive(ctx context.Context, runner deploy.Runner, sshArgs []string, target, service string, streams deploy.IOStreams) error {
	checkCmd := fmt.Sprintf("systemctl is-active --quiet %s", service)
	checkArgs := append(append([]string(nil), sshArgs...), target, checkCmd)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Try immediately first.
	if err := runner.Run(ctx, "ssh", checkArgs, nil, streams); err == nil {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			// Dump service status for debugging.
			statusCmd := fmt.Sprintf("systemctl status %s --no-pager", service)
			statusArgs := append(append([]string(nil), sshArgs...), target, statusCmd)
			_ = runner.Run(context.Background(), "ssh", statusArgs, nil, streams)
			return fmt.Errorf("service %s did not become active within timeout", service)
		case <-ticker.C:
			if err := runner.Run(ctx, "ssh", checkArgs, nil, streams); err == nil {
				return nil
			}
		}
	}
}
