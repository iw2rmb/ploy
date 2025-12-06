package deploy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"unicode"

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
		"PLOY_BOOTSTRAP_VERSION": Version,
	}
	envVars["PLOYD_METRICS_LISTEN"] = "127.0.0.1:9101"
	if sanitized := sanitizeNodeID(nodeID); sanitized != "" {
		envVars["PLOYD_NODE_ID"] = sanitized
	} else {
		envVars["PLOYD_NODE_ID"] = nodeID
	}
	envVars["PLOYD_HOME_DIR"] = "/root"
	envVars["PLOYD_CACHE_HOME"] = "/var/cache/ploy"

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
		ClusterID:       config.ClusterID(descriptorID),
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

func formatHost(address string) string {
	addr := strings.TrimSpace(address)
	if host, _, err := net.SplitHostPort(addr); err == nil && host != "" {
		addr = host
	}
	if addr == "" {
		return "127.0.0.1"
	}
	if strings.HasPrefix(addr, "[") {
		return addr
	}
	if strings.Contains(addr, ":") {
		return "[" + addr + "]"
	}
	return addr
}

func sanitizeNodeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune('-')
		default:
			if unicode.IsSpace(r) {
				builder.WriteRune('-')
			}
		}
	}
	out := strings.Trim(builder.String(), "-")
	if out == "" {
		return ""
	}
	return out
}
