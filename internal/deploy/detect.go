package deploy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// DetectionResult holds information about an existing cluster detected on a target host.
type DetectionResult struct {
	Found     bool
	ClusterID domaintypes.ClusterID
}

// DetectExisting probes the target host for an existing ploy cluster installation.
// It checks for the presence of /etc/ploy/pki/ca.crt and /etc/ploy/ployd.yaml,
// and attempts to extract the cluster ID from the server certificate CN.
//
// The server certificate CN follows the pattern "ployd-<clusterID>".
func DetectExisting(ctx context.Context, runner Runner, opts ProvisionOptions) (DetectionResult, error) {
	if runner == nil {
		runner = SystemRunner{}
	}

	user := strings.TrimSpace(opts.User)
	if user == "" {
		user = DefaultRemoteUser
	}
	port := opts.Port
	if port == 0 {
		port = DefaultSSHPort
	}

	connectHost := strings.TrimSpace(opts.Address)
	if connectHost == "" {
		connectHost = strings.TrimSpace(opts.Host)
	}
	if connectHost == "" {
		return DetectionResult{}, fmt.Errorf("detect: host or address required")
	}

	target := connectHost
	if user != "" {
		target = fmt.Sprintf("%s@%s", user, connectHost)
	}

	sshArgs := BuildSSHArgs(opts.IdentityFile, port)

	// Check if CA certificate exists
	checkCACertArgs := append(append([]string(nil), sshArgs...), target, "test -f /etc/ploy/pki/ca.crt")
	var discardOut bytes.Buffer
	streams := IOStreams{Stdout: &discardOut, Stderr: &discardOut}
	if err := runner.Run(ctx, "ssh", checkCACertArgs, nil, streams); err != nil {
		// CA cert does not exist, no cluster present
		return DetectionResult{Found: false}, nil
	}

	// Check if server config exists
	checkConfigArgs := append(append([]string(nil), sshArgs...), target, "test -f /etc/ploy/ployd.yaml")
	discardOut.Reset()
	if err := runner.Run(ctx, "ssh", checkConfigArgs, nil, streams); err != nil {
		// Config does not exist, no cluster present
		return DetectionResult{Found: false}, nil
	}

	// Ensure the server certificate exists; if missing, this might be a node or incomplete installation.
	checkServerArgs := append(append([]string(nil), sshArgs...), target, "test -f /etc/ploy/pki/server.crt")
	if err := runner.Run(ctx, "ssh", checkServerArgs, nil, streams); err != nil {
		return DetectionResult{Found: true, ClusterID: ""}, nil
	}

	// Extract CN from certificate using openssl
	extractCNArgs := append(append([]string(nil), sshArgs...), target,
		"openssl x509 -in /etc/ploy/pki/server.crt -noout -subject -nameopt multiline | grep 'commonName' | sed 's/.*= //'")
	var cnOut bytes.Buffer
	cnStreams := IOStreams{Stdout: &cnOut, Stderr: io.Discard}
	if err := runner.Run(ctx, "ssh", extractCNArgs, nil, cnStreams); err != nil {
		// Could not extract CN
		return DetectionResult{Found: true, ClusterID: ""}, nil
	}

	cn := strings.TrimSpace(cnOut.String())
	clusterID := extractClusterIDFromCN(cn)

	return DetectionResult{Found: true, ClusterID: domaintypes.ClusterID(clusterID)}, nil
}

// extractClusterIDFromCN parses the CN to extract the cluster ID.
// Expected format: "ployd-<clusterID>"
func extractClusterIDFromCN(cn string) string {
	// Match pattern: ployd-<clusterID>
	re := regexp.MustCompile(`^ployd-([a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*)$`)
	matches := re.FindStringSubmatch(cn)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}
