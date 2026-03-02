package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/deploy"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleNode routes node subcommands.
func handleNode(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	if wantsHelp(args) {
		printNodeUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printNodeUsage(stderr)
		return errors.New("node subcommand required")
	}
	switch args[0] {
	case "add":
		return handleNodeAdd(args[1:], stderr)
	default:
		printNodeUsage(stderr)
		return fmt.Errorf("unknown node subcommand %q", args[0])
	}
}

// printNodeUsage prints the node command usage information.
// This provides a single, consistent usage output for --help, error paths,
// and unknown subcommand handling.
//
// NOTE: Node commands are now accessed via `ploy cluster node` instead of the
// former `ploy node`. This reflects the restructuring of cluster management
// operations under a unified `ploy cluster` namespace.
func printNodeUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy cluster node <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  add       Add a worker node to the cluster")
}

// printNodeAddUsage prints usage information for the node add command.
// NOTE: Node add is now accessed via `ploy cluster node add`.
func printNodeAddUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy cluster node add --cluster-id <id> --address <ip> --server-url <url>")
}

// handleNodeAdd validates required flags for adding a worker node.
func handleNodeAdd(args []string, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("node add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		clusterID    stringValue
		address      stringValue
		serverURL    stringValue
		identity     stringValue
		userFlag     stringValue
		ploydNodeBin stringValue
		sshPort      intValue
		dryRun       bool
	)
	fs.Var(&clusterID, "cluster-id", "Cluster identifier to join")
	fs.Var(&address, "address", "Node IP or hostname")
	fs.Var(&serverURL, "server-url", "Ploy server URL (required; e.g. https://<server-host>:8443)")
	fs.Var(&identity, "identity", "SSH private key used for provisioning (default: ~/.ssh/id_rsa)")
	fs.Var(&userFlag, "user", "SSH username used for provisioning (default: root)")
	fs.Var(&ploydNodeBin, "ployd-node-binary", "Path to the ployd-node binary uploaded during provisioning (default: alongside the CLI)")
	fs.Var(&sshPort, "ssh-port", "SSH port for node provisioning (default: 22)")
	fs.BoolVar(&dryRun, "dry-run", false, "Validate inputs without performing provisioning")

	if err := fs.Parse(args); err != nil {
		printNodeAddUsage(stderr)
		return err
	}
	if fs.NArg() > 0 {
		printNodeAddUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !clusterID.set || strings.TrimSpace(clusterID.value) == "" {
		printNodeAddUsage(stderr)
		return errors.New("cluster-id is required")
	}
	if !address.set || strings.TrimSpace(address.value) == "" {
		printNodeAddUsage(stderr)
		return errors.New("address is required")
	}

	nodeCfg := nodeAddConfig{
		ClusterID:       domaintypes.ClusterID(strings.TrimSpace(clusterID.value)),
		Address:         address.value,
		ServerURL:       serverURL.value,
		User:            userFlag.value,
		IdentityFile:    identity.value,
		PloydNodeBinary: ploydNodeBin.value,
		SSHPort:         sshPort.value,
		DryRun:          dryRun,
	}

	return runNodeAdd(nodeCfg, stderr)
}

type nodeAddConfig struct {
	ClusterID       domaintypes.ClusterID
	Address         string
	ServerURL       string
	User            string
	IdentityFile    string
	PloydNodeBinary string
	SSHPort         int
	DryRun          bool
}

func runNodeAdd(cfg nodeAddConfig, stderr io.Writer) error {
	if stderr == nil {
		stderr = os.Stderr
	}
	ctx := context.Background()

	// Resolve default paths
	identityPath, err := resolveIdentityPath(stringValue{set: cfg.IdentityFile != "", value: cfg.IdentityFile})
	if err != nil {
		return fmt.Errorf("node add: %w", err)
	}

	ploydNodeBinaryPath, err := resolvePloydNodeBinaryPath(stringValue{set: cfg.PloydNodeBinary != "", value: cfg.PloydNodeBinary})
	if err != nil {
		return fmt.Errorf("node add: %w", err)
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
		return fmt.Errorf("node add: %w", err)
	}

	if cfg.DryRun {
		_, _ = fmt.Fprintln(stderr, "[DRY RUN] Validating node add configuration...")
	}

	_, _ = fmt.Fprintf(stderr, "Adding node to cluster %s\n", cfg.ClusterID.String())
	_, _ = fmt.Fprintf(stderr, "  Node Address: %s\n", cfg.Address)
	_, _ = fmt.Fprintf(stderr, "  SSH User: %s\n", user)
	_, _ = fmt.Fprintf(stderr, "  SSH Port: %d\n", sshPort)
	_, _ = fmt.Fprintf(stderr, "  Identity: %s\n", identityPath)
	_, _ = fmt.Fprintf(stderr, "  Binary: %s\n", ploydNodeBinaryPath)

	// Generate node ID
	nodeID := deploy.GenerateNodeID()
	_, _ = fmt.Fprintf(stderr, "Generated node ID: %s\n", nodeID)

	serverURL := strings.TrimSpace(cfg.ServerURL)
	if serverURL == "" {
		return errors.New("node add: server-url is required")
	}

	// Early exit for dry-run mode after validation.
	if cfg.DryRun {
		_, _ = fmt.Fprintln(stderr, "\n[DRY RUN] Validation complete. Provisioning would proceed with:")
		_, _ = fmt.Fprintf(stderr, "  Server URL: %s\n", serverURL)
		_, _ = fmt.Fprintf(stderr, "  Generated Node ID: %s\n", nodeID)
		_, _ = fmt.Fprintln(stderr, "\nAll validations passed. No actual provisioning performed.")
		return nil
	}

	// Request bootstrap token from server
	_, _ = fmt.Fprintln(stderr, "Requesting bootstrap token from server...")
	bootstrapToken, expiresAt, err := requestBootstrapToken(ctx, serverURL, nodeID)
	if err != nil {
		return fmt.Errorf("node add: request bootstrap token: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "Bootstrap token received (expires: %s)\n", expiresAt.Format(time.RFC3339))

	// Get CA certificate for TLS verification (node will need this to verify server)
	_, _ = fmt.Fprintln(stderr, "Fetching CA certificate for TLS verification...")
	caCert, err := fetchCACertificate(ctx, serverURL, user, sshPort, identityPath, nil)
	if err != nil {
		// For now, we'll allow continuing without CA cert if it fails
		// The node will use system trust store
		_, _ = fmt.Fprintf(stderr, "Warning: could not fetch CA certificate: %v\n", err)
		_, _ = fmt.Fprintln(stderr, "Node will use system trust store for TLS verification")
		caCert = ""
	}

	// Prepare environment variables for bootstrap script
	scriptEnv := map[string]string{
		"CLUSTER_ID":           cfg.ClusterID.String(),
		"NODE_ID":              nodeID,
		"NODE_ADDRESS":         cfg.Address,
		"BOOTSTRAP_PRIMARY":    "false",
		"PLOY_BOOTSTRAP_TOKEN": bootstrapToken,
		"PLOY_SERVER_URL":      serverURL,
	}
	if caCert != "" {
		scriptEnv["PLOY_CA_CERT_PEM"] = caCert
	}

	// Provision the node host
	_, _ = fmt.Fprintln(stderr, "Installing ployd-node binary and bootstrapping node...")
	provisionOpts := deploy.ProvisionOptions{
		Host:            cfg.Address,
		Address:         cfg.Address,
		User:            user,
		Port:            sshPort,
		IdentityFile:    identityPath,
		PloydBinaryPath: ploydNodeBinaryPath,
		Stdout:          os.Stdout,
		Stderr:          stderr,
		ScriptEnv:       scriptEnv,
		ScriptArgs:      []string{"--cluster-id", cfg.ClusterID.String(), "--node-id", nodeID, "--node-address", cfg.Address},
		ServiceChecks:   []string{"ployd-node"},
	}

	if err := deploy.ProvisionHost(ctx, provisionOpts); err != nil {
		return fmt.Errorf("node add: provision host: %w", err)
	}

	// Refresh cluster descriptor without clobbering TLS fields if present.
	// Prefer existing descriptor CA/client cert for future mTLS operations.
	existing, loadErr := config.LoadDefault()
	desc := config.Descriptor{
		ClusterID:       cfg.ClusterID,
		Address:         serverURL,
		SSHIdentityPath: identityPath,
	}
	if loadErr != nil || strings.TrimSpace(string(existing.ClusterID)) == "" {
		if _, err := config.SaveDescriptor(desc); err != nil {
			_, _ = fmt.Fprintf(stderr, "Warning: failed to save/refresh cluster descriptor: %v\n", err)
		}
	}

	_, _ = fmt.Fprintln(stderr, "\nNode provisioning complete!")
	_, _ = fmt.Fprintf(stderr, "Node ID: %s\n", nodeID)
	_, _ = fmt.Fprintf(stderr, "Node address: %s\n", cfg.Address)
	_, _ = fmt.Fprintln(stderr, "\nThe node is now connected to the cluster and ready to accept runs.")

	return nil
}

// resolvePloydNodeBinaryPath locates the ployd-node binary adjacent to the CLI.
func resolvePloydNodeBinaryPath(v stringValue) (string, error) {
	if v.set {
		path := expandPath(v.value)
		if err := validateFileReadable(path); err != nil {
			return "", fmt.Errorf("ployd-node binary: %w", err)
		}
		return path, nil
	}
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate ploy executable: %w", err)
	}
	dir := filepath.Dir(execPath)
	osName := runtime.GOOS
	candidates := make([]string, 0, 3)
	if osName != "linux" {
		candidates = append(candidates, filepath.Join(dir, "ployd-node-linux"))
	}
	if osName == "windows" {
		candidates = append(candidates, filepath.Join(dir, "ployd-node.exe"))
	}
	candidates = append(candidates, filepath.Join(dir, "ployd-node"))
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}
	// Used by both `ploy cluster node add` (flag: --ployd-node-binary) and
	// `ploy cluster rollout nodes` (flag: --binary). Keep guidance helpful for both.
	return "", errors.New("ployd-node binary not found alongside CLI; provide --binary (cluster rollout nodes) or --ployd-node-binary (cluster node add)")
}

// pkiSignRequest is the JSON request body for POST /v1/pki/sign.
// Uses domain type (NodeID) for type-safe identification.
type pkiSignRequest struct {
	NodeID domaintypes.NodeID `json:"node_id"` // Node ID (NanoID-backed)
	CSR    string             `json:"csr"`
}

// pkiSignResponse is the JSON response body for POST /v1/pki/sign.
type pkiSignResponse struct {
	Certificate string `json:"certificate"`
	CABundle    string `json:"ca_bundle"`
	Serial      string `json:"serial"`
	Fingerprint string `json:"fingerprint"`
	NotBefore   string `json:"not_before"`
	NotAfter    string `json:"not_after"`
}

// signNodeCSR calls the server's /v1/pki/sign endpoint to sign the CSR.
// nodeID parameter is a string that gets converted to domain type for the request.
func signNodeCSR(ctx context.Context, serverURL, nodeID string, csrPEM []byte) (certPEM, caCertPEM string, err error) {
	reqBody := pkiSignRequest{
		NodeID: domaintypes.NodeID(nodeID), // Convert to domain type
		CSR:    string(csrPEM),
	}
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := strings.TrimSuffix(serverURL, "/") + "/v1/pki/sign"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Use descriptor-backed mTLS client when available
	_, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return "", "", fmt.Errorf("resolve control-plane client: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("send request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var signResp pkiSignResponse
	if err := json.NewDecoder(resp.Body).Decode(&signResp); err != nil {
		return "", "", fmt.Errorf("decode response: %w", err)
	}

	return signResp.Certificate, signResp.CABundle, nil
}

// requestBootstrapToken requests a short-lived bootstrap token from the server for node provisioning.
func requestBootstrapToken(ctx context.Context, serverURL, nodeID string) (token string, expiresAt time.Time, err error) {
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("resolve control plane: %w", err)
	}

	reqBody := map[string]interface{}{
		"node_id":            nodeID,
		"expires_in_minutes": 15, // 15 minute window for provisioning
	}
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/bootstrap/tokens"
	req, err := makeAuthenticatedRequest(ctx, "POST", endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("POST %s: %w", endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", time.Time{}, fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Token     string             `json:"token"`
		NodeID    domaintypes.NodeID `json:"node_id"`
		ExpiresAt time.Time          `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", time.Time{}, fmt.Errorf("decode response: %w", err)
	}

	return result.Token, result.ExpiresAt, nil
}

// fetchCACertificate reads the CA certificate from the server host over SSH so
// the node can verify the server during bootstrap (before it has mTLS credentials).
//
// The CA cert is expected at /etc/ploy/pki/ca.crt (written during server deploy).
// If serverURL is not https, an empty string is returned.
func fetchCACertificate(ctx context.Context, serverURL, sshUser string, sshPort int, identityPath string, runner deploy.Runner) (string, error) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(serverURL)), "http://") {
		return "", nil
	}

	u, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || u.Hostname() == "" {
		// url.Parse treats URLs without a scheme as paths. Retry with an https scheme.
		u, err = url.Parse("https://" + strings.TrimSpace(serverURL))
	}
	if err != nil {
		return "", fmt.Errorf("parse server URL: %w", err)
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("parse server URL: missing host")
	}
	if strings.TrimSpace(sshUser) == "" {
		sshUser = deploy.DefaultRemoteUser
	}
	if sshPort == 0 {
		sshPort = deploy.DefaultSSHPort
	}
	if runner == nil {
		runner = deploy.NewSystemRunner()
	}

	sshArgs := deploy.BuildSSHArgs(identityPath, sshPort)
	target := fmt.Sprintf("%s@%s", sshUser, host)
	out := &strings.Builder{}
	args := append(append([]string(nil), sshArgs...), target, "cat /etc/ploy/pki/ca.crt")
	if err := runner.Run(ctx, "ssh", args, nil, deploy.IOStreams{Stdout: out, Stderr: io.Discard}); err != nil {
		return "", fmt.Errorf("read CA cert from %s: %w", host, err)
	}
	return out.String(), nil
}
