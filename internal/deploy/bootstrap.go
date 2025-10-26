package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/bootstrap"
	"github.com/iw2rmb/ploy/internal/cli/config"
)

// RunBootstrap orchestrates remote installation via SSH and finalises PKI metadata locally.
func RunBootstrap(ctx context.Context, opts Options) error {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	stdin := opts.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}

	address := strings.TrimSpace(opts.Address)
	if address == "" {
		return errors.New("bootstrap: address required")
	}
	opts.Address = address

	clusterID := strings.TrimSpace(opts.ClusterID)
	if clusterID == "" {
		clusterID = strings.TrimSpace(opts.DescriptorID)
	}
	opts.ClusterID = clusterID

	nodeID := strings.TrimSpace(opts.NodeID)
	if nodeID == "" {
		nodeID = "control"
	}

	nodeAddress := strings.TrimSpace(opts.NodeAddress)
	if nodeAddress == "" {
		nodeAddress = address
	}

	user := strings.TrimSpace(opts.User)
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

	displayTarget := address
	ploydBinary := strings.TrimSpace(opts.PloydBinaryPath)
	if ploydBinary == "" {
		return errors.New("bootstrap: ployd binary path required")
	}

	envVars := map[string]string{
		"PLOY_BOOTSTRAP_VERSION": bootstrap.Version,
	}

	scriptArgs := make([]string, 0, 8)
	if clusterID != "" {
		scriptArgs = append(scriptArgs, "--cluster-id", clusterID)
	}
	if nodeID != "" {
		scriptArgs = append(scriptArgs, "--node-id", nodeID)
	}
	if nodeAddress != "" {
		scriptArgs = append(scriptArgs, "--node-address", nodeAddress)
	}
	if opts.Primary {
		scriptArgs = append(scriptArgs, "--primary")
	}

	provisionOpts := ProvisionOptions{
		Host:            address,
		Address:         address,
		User:            user,
		Port:            port,
		IdentityFile:    opts.IdentityFile,
		PloydBinaryPath: ploydBinary,
		Runner:          runner,
		Stdout:          stdout,
		Stderr:          stderr,
		ScriptEnv:       envVars,
		ScriptArgs:      scriptArgs,
		ServiceChecks:   []string{"ployd"},
	}

	if err := ProvisionHost(ctx, provisionOpts); err != nil {
		return err
	}

	descriptorID := strings.TrimSpace(opts.DescriptorID)
	if descriptorID == "" {
		descriptorID = address
	}
	desc := config.Descriptor{
		ClusterID:       descriptorID,
		Address:         strings.TrimSpace(opts.DescriptorAddress),
		SSHIdentityPath: strings.TrimSpace(opts.DescriptorIdentityPath),
	}
	if desc.Address == "" {
		desc.Address = address
	}
	if desc.SSHIdentityPath == "" {
		desc.SSHIdentityPath = opts.IdentityFile
	}
	saved, err := config.SaveDescriptor(desc)
	if err != nil {
		return fmt.Errorf("bootstrap: save descriptor: %w", err)
	}
	if err := config.SetDefault(saved.ClusterID); err != nil {
		return fmt.Errorf("bootstrap: set default descriptor: %w", err)
	}

	if _, err := fmt.Fprintf(stderr, "Bootstrap completed for %s.\n", displayTarget); err != nil {
		return fmt.Errorf("bootstrap: write completion message: %w", err)
	}
	return nil
}
