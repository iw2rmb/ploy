package deploy

import (
	"context"
	"io"
	"os"
	"os/exec"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
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
// Uses domain types (ClusterID, NodeID) for type-safe identification.
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
	ClusterID              domaintypes.ClusterID `yaml:"cluster_id"` // Cluster ID
	InitialWorkers         []string
	Primary                bool
	NodeID                 domaintypes.NodeID `yaml:"node_id"` // Node ID (NanoID-backed)
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

// SystemRunner executes shell commands using the host environment streams.
type SystemRunner struct{}

// NewSystemRunner creates a new system runner that executes commands via exec.
func NewSystemRunner() Runner {
	return SystemRunner{}
}

// Run invokes the command with inherited stdio defaults when streams are nil.
func (SystemRunner) Run(ctx context.Context, command string, args []string, stdin io.Reader, streams IOStreams) error {
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

// systemRunner is an internal alias for backward compatibility.
type systemRunner = SystemRunner
