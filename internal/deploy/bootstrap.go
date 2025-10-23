package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/cmd/ploy/config"
)

type clusterPKIState struct {
	CurrentCA  CABundle
	Descriptor config.Descriptor
}

const (
	// DefaultRemoteUser is applied when no remote user is provided.
	DefaultRemoteUser = "root"
	// DefaultSSHPort is used when no SSH port is specified.
	DefaultSSHPort = 22
	// DefaultMinDiskGB represents the minimum free disk space required for bootstrap.
	DefaultMinDiskGB = 4
)

var (
	defaultRequiredPorts = []int{2379, 2380, 9094, 9095}
	bootstrapVersion     = "2025-10-22"
)

// Options configure bootstrap execution.
type Options struct {
	Host            string
	Address         string
	User            string
	Port            int
	IdentityFile    string
	DryRun          bool
	Stdout          io.Writer
	Stderr          io.Writer
	Runner          Runner
	ClusterID       string
	EtcdClient      *clientv3.Client
	InitialBeacons  []string
	InitialWorkers  []string
	BeaconURL       string
	ControlPlaneURL string
	APIKey          string
	Clock           func() time.Time
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

	requiredPorts := append([]int(nil), defaultRequiredPorts...)

	script := prependEnvironment(DefaultMinDiskGB, requiredPorts) + RenderBootstrapScript()

	clusterID := strings.TrimSpace(opts.ClusterID)
	if clusterID == "" {
		return errors.New("bootstrap: cluster id required")
	}
	opts.ClusterID = clusterID

	opts.BeaconURL = strings.TrimSpace(opts.BeaconURL)
	opts.ControlPlaneURL = strings.TrimSpace(opts.ControlPlaneURL)
	opts.APIKey = strings.TrimSpace(opts.APIKey)

	if !opts.DryRun {
		if opts.EtcdClient == nil {
			return errors.New("bootstrap: etcd client required")
		}
		if opts.BeaconURL == "" {
			return errors.New("bootstrap: beacon url required")
		}
		if opts.APIKey == "" {
			return errors.New("bootstrap: api key required")
		}
		if !hasNonEmpty(opts.InitialBeacons) {
			return errors.New("bootstrap: at least one beacon id required")
		}
	}

	if opts.DryRun {
		if _, err := io.WriteString(stdout, script); err != nil {
			return fmt.Errorf("bootstrap: write dry-run script: %w", err)
		}
		return nil
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

	if opts.Host == "" {
		return errors.New("bootstrap: host required")
	}

	connectHost := strings.TrimSpace(opts.Address)
	if connectHost == "" {
		connectHost = strings.TrimSpace(opts.Host)
	}
	if connectHost == "" {
		return errors.New("bootstrap: address required")
	}

	displayTarget := opts.Host
	if displayTarget == "" {
		displayTarget = connectHost
	} else if opts.Address != "" && opts.Address != opts.Host {
		displayTarget = fmt.Sprintf("%s (%s)", opts.Host, opts.Address)
	}

	target := connectHost
	if user != "" {
		target = fmt.Sprintf("%s@%s", user, connectHost)
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

	state, err := bootstrapClusterPKI(ctx, clusterID, opts)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(stderr, "Bootstrap completed for %s.\n", displayTarget); err != nil {
		return fmt.Errorf("bootstrap: write completion message: %w", err)
	}
	if _, err := fmt.Fprintf(stderr, "Cluster %s PKI bootstrapped (CA version %s).\n", clusterID, state.CurrentCA.Version); err != nil {
		return fmt.Errorf("bootstrap: write PKI completion message: %w", err)
	}
	return nil
}

func bootstrapClusterPKI(ctx context.Context, clusterID string, opts Options) (*clusterPKIState, error) {
	clusterID = strings.TrimSpace(clusterID)
	client := opts.EtcdClient
	if client == nil {
		return nil, errors.New("bootstrap: etcd client required")
	}
	if strings.TrimSpace(opts.BeaconURL) == "" {
		return nil, errors.New("bootstrap: beacon url required")
	}
	if strings.TrimSpace(opts.APIKey) == "" {
		return nil, errors.New("bootstrap: api key required")
	}
	beaconIDs := normalizeNodeIDs(opts.InitialBeacons)
	if len(beaconIDs) == 0 {
		return nil, errors.New("bootstrap: at least one beacon id required")
	}
	workerIDs := normalizeNodeIDs(opts.InitialWorkers)

	manager, err := NewCARotationManager(client, clusterID)
	if err != nil {
		return nil, err
	}

	clock := opts.Clock
	var requestedAt time.Time
	if clock != nil {
		requestedAt = clock().UTC()
	} else {
		requestedAt = time.Now().UTC()
	}

	state, err := manager.Bootstrap(ctx, BootstrapOptions{
		BeaconIDs:   beaconIDs,
		WorkerIDs:   workerIDs,
		RequestedAt: requestedAt,
	})
	if err != nil {
		return nil, err
	}

	caPath, err := config.CABundlePath(clusterID)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: resolve ca bundle path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(caPath), 0o755); err != nil {
		return nil, fmt.Errorf("bootstrap: create config directory: %w", err)
	}
	if err := os.WriteFile(caPath, []byte(state.CurrentCA.CertificatePEM), 0o600); err != nil {
		return nil, fmt.Errorf("bootstrap: write ca bundle: %w", err)
	}

	beaconURL := strings.TrimSpace(opts.BeaconURL)
	controlPlaneURL := strings.TrimSpace(opts.ControlPlaneURL)
	apiKey := strings.TrimSpace(opts.APIKey)

	descriptor := config.Descriptor{
		ID:              clusterID,
		BeaconURL:       beaconURL,
		ControlPlaneURL: controlPlaneURL,
		APIKey:          apiKey,
		CABundlePath:    caPath,
		TrustBundlePath: caPath,
		Version:         state.CurrentCA.Version,
	}
	saved, err := config.SaveDescriptor(descriptor)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: save descriptor: %w", err)
	}

	return &clusterPKIState{
		CurrentCA:  state.CurrentCA,
		Descriptor: saved,
	}, nil
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

func hasNonEmpty(values []string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// RenderBootstrapScript exposes the embedded script.
func RenderBootstrapScript() string {
	return scriptTemplate
}
