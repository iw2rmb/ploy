package deploy

import (
	"context"
	"io"
	"os"
	"os/exec"
	"time"
)

// This file defines bootstrap constants plus helper types shared across splits.

const (
	// DefaultRemoteUser is applied when no remote user is provided.
	DefaultRemoteUser = "root"
	// DefaultSSHPort is used when no SSH port is specified.
	DefaultSSHPort = 22
	// remotePloydBinaryPath is where the ployd binary is installed on the target host.
	remotePloydBinaryPath = "/usr/local/bin/ployd"
)

// Options configure bootstrap execution.
type Options struct {
	Host                   string
	Address                string
	User                   string
	Port                   int
	IdentityFile           string
	Stdout                 io.Writer
	Stderr                 io.Writer
	Runner                 Runner
	PloydBinaryPath        string
	ControlPlaneURL        string
	Clock                  func() time.Time
	Stdin                  io.Reader
	WorkstationOS          string
	DescriptorID           string
	DescriptorAddress      string
	DescriptorIdentityPath string
	ClusterID              string
	InitialWorkers         []string
	Primary                bool
	NodeID                 string
	NodeAddress            string
}

// IOStreams represents command IO endpoints.
type IOStreams struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Runner executes commands with the rendered script.
type Runner interface {
	Run(ctx context.Context, command string, args []string, stdin io.Reader, streams IOStreams) error
}

// RunnerFunc adapts a function to the Runner interface.
type RunnerFunc func(ctx context.Context, command string, args []string, stdin io.Reader, streams IOStreams) error

// Run executes the underlying function.
func (fn RunnerFunc) Run(ctx context.Context, command string, args []string, stdin io.Reader, streams IOStreams) error {
	return fn(ctx, command, args, stdin, streams)
}

// systemRunner executes shell commands using the host environment streams.
type systemRunner struct{}

// Run invokes the command with inherited stdio defaults when streams are nil.
func (systemRunner) Run(ctx context.Context, command string, args []string, stdin io.Reader, streams IOStreams) error {
	cmd := exec.CommandContext(ctx, command, args...)
	if streams.Stdout != nil {
		cmd.Stdout = streams.Stdout
	}
	if streams.Stderr != nil {
		cmd.Stderr = streams.Stderr
	}
	if stdin != nil {
		cmd.Stdin = stdin
	} else {
		cmd.Stdin = os.Stdin
	}
	return cmd.Run()
}
