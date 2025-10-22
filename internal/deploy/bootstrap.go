package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	// DefaultRemoteUser is applied when no remote user is provided.
	DefaultRemoteUser = "root"
	// DefaultSSHPort is used when no SSH port is specified.
	DefaultSSHPort = 22
	// DefaultMinDiskGB represents the minimum free disk space required for bootstrap.
	DefaultMinDiskGB = 40
)

var (
	defaultRequiredPorts = []int{2379, 2380, 9094, 9095}
	bootstrapVersion     = "2025-10-22"
)

// Options configure bootstrap execution.
type Options struct {
	Host          string
	User          string
	Port          int
	IdentityFile  string
	DryRun        bool
	MinDiskGB     int
	RequiredPorts []int
	Stdout        io.Writer
	Stderr        io.Writer
	Runner        Runner
}

// IOStreams represents command IO endpoints.
type IOStreams struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Runner executes commands with the rendered script.
type Runner interface {
	Run(ctx context.Context, command string, args []string, stdin string, streams IOStreams) error
}

// RunnerFunc adapts a function to the Runner interface.
type RunnerFunc func(ctx context.Context, command string, args []string, stdin string, streams IOStreams) error

// Run executes the underlying function.
func (fn RunnerFunc) Run(ctx context.Context, command string, args []string, stdin string, streams IOStreams) error {
	return fn(ctx, command, args, stdin, streams)
}

type systemRunner struct{}

func (systemRunner) Run(ctx context.Context, command string, args []string, stdin string, streams IOStreams) error {
	cmd := exec.CommandContext(ctx, command, args...)
	if streams.Stdout != nil {
		cmd.Stdout = streams.Stdout
	}
	if streams.Stderr != nil {
		cmd.Stderr = streams.Stderr
	}
	cmd.Stdin = strings.NewReader(stdin)
	return cmd.Run()
}

// RunBootstrap orchestrates remote installation via SSH or prints the script when dry-run is enabled.
func RunBootstrap(ctx context.Context, opts Options) error {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	minDisk := opts.MinDiskGB
	if minDisk == 0 {
		minDisk = DefaultMinDiskGB
	}
	requiredPorts := opts.RequiredPorts
	if len(requiredPorts) == 0 {
		requiredPorts = append([]int(nil), defaultRequiredPorts...)
	}

	script := prependEnvironment(minDisk, requiredPorts) + RenderBootstrapScript()

	if opts.DryRun {
		if _, err := io.WriteString(stdout, script); err != nil {
			return fmt.Errorf("bootstrap: write dry-run script: %w", err)
		}
		return nil
	}

	if opts.Host == "" {
		return errors.New("bootstrap: host required")
	}

	user := opts.User
	if user == "" {
		user = DefaultRemoteUser
	}
	port := opts.Port
	if port == 0 {
		port = DefaultSSHPort
	}

	runner := opts.Runner
	if runner == nil {
		runner = systemRunner{}
	}

	target := opts.Host
	if user != "" {
		target = fmt.Sprintf("%s@%s", user, opts.Host)
	}

	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if opts.IdentityFile != "" {
		args = append(args, "-i", opts.IdentityFile)
	}
	if port != DefaultSSHPort {
		args = append(args, "-p", strconv.Itoa(port))
	}
	args = append(args, target, "bash -s --")

	streams := IOStreams{Stdout: stdout, Stderr: stderr}
	if err := runner.Run(ctx, "ssh", args, script, streams); err != nil {
		return fmt.Errorf("bootstrap: ssh execution failed: %w", err)
	}
	return nil
}

func prependEnvironment(minDisk int, ports []int) string {
	portStrings := make([]string, len(ports))
	for i, port := range ports {
		portStrings[i] = strconv.Itoa(port)
	}
	builder := strings.Builder{}
	builder.WriteString(fmt.Sprintf("export PLOY_BOOTSTRAP_VERSION=\"%s\"\n", bootstrapVersion))
	builder.WriteString(fmt.Sprintf("export PLOY_MIN_DISK_GB=%d\n", minDisk))
	builder.WriteString(fmt.Sprintf("export PLOY_REQUIRED_PORTS=\"%s\"\n", strings.Join(portStrings, " ")))
	builder.WriteString("export PLOY_REQUIRED_PACKAGES=\"ipfs-cluster-service docker etcd go\"\n")
	builder.WriteString("\n")
	return builder.String()
}

// RenderBootstrapScript exposes the embedded script.
func RenderBootstrapScript() string {
	return scriptTemplate
}
