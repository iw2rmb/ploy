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
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/internal/pki"
)

// handleNode routes node subcommands.
func handleNode(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "Usage: ploy node <command>")
		return errors.New("node subcommand required")
	}
	switch args[0] {
	case "add":
		return handleNodeAdd(args[1:], stderr)
	default:
		_, _ = fmt.Fprintln(stderr, "Usage: ploy node <command>")
		return fmt.Errorf("unknown node subcommand %q", args[0])
	}
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
		_, _ = fmt.Fprintln(stderr, "Usage: ploy node add --cluster-id <id> --address <ip> --server-url <url>")
		return err
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintln(stderr, "Usage: ploy node add --cluster-id <id> --address <ip> --server-url <url>")
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !clusterID.set || strings.TrimSpace(clusterID.value) == "" {
		_, _ = fmt.Fprintln(stderr, "Usage: ploy node add --cluster-id <id> --address <ip> --server-url <url>")
		return errors.New("cluster-id is required")
	}
	if !address.set || strings.TrimSpace(address.value) == "" {
		_, _ = fmt.Fprintln(stderr, "Usage: ploy node add --cluster-id <id> --address <ip> --server-url <url>")
		return errors.New("address is required")
	}

	nodeCfg := nodeAddConfig{
		ClusterID:       clusterID.value,
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
	ClusterID       string
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

	_, _ = fmt.Fprintf(stderr, "Adding node to cluster %s\n", cfg.ClusterID)
	_, _ = fmt.Fprintf(stderr, "  Node Address: %s\n", cfg.Address)
	_, _ = fmt.Fprintf(stderr, "  SSH User: %s\n", user)
	_, _ = fmt.Fprintf(stderr, "  SSH Port: %d\n", sshPort)
	_, _ = fmt.Fprintf(stderr, "  Identity: %s\n", identityPath)
	_, _ = fmt.Fprintf(stderr, "  Binary: %s\n", ploydNodeBinaryPath)

	// Generate node ID
	nodeID := uuid.New().String()
	_, _ = fmt.Fprintf(stderr, "Generated node ID: %s\n", nodeID)

	// Generate node CSR and private key
	_, _ = fmt.Fprintln(stderr, "Generating node private key and CSR...")
	keyBundle, csrPEM, err := pki.GenerateNodeCSR(nodeID, cfg.ClusterID, cfg.Address)
	if err != nil {
		return fmt.Errorf("node add: generate CSR: %w", err)
	}
	_, _ = fmt.Fprintln(stderr, "CSR generated")

	// Call server to sign the CSR
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

	_, _ = fmt.Fprintf(stderr, "Requesting certificate signing from %s\n", serverURL)

	signedCert, caCert, err := signNodeCSR(ctx, serverURL, nodeID, csrPEM)
	if err != nil {
		return fmt.Errorf("node add: sign CSR: %w", err)
	}
	_, _ = fmt.Fprintln(stderr, "Certificate signed successfully")

	// Prepare environment variables for bootstrap script
	scriptEnv := map[string]string{
		"CLUSTER_ID":        cfg.ClusterID,
		"NODE_ID":           nodeID,
		"NODE_ADDRESS":      cfg.Address,
		"BOOTSTRAP_PRIMARY": "false",
		"PLOY_CA_CERT_PEM":  caCert,
		// Despite the name, the bootstrap uses PLOY_SERVER_* for both server and node flows.
		"PLOY_SERVER_CERT_PEM": signedCert,
		"PLOY_SERVER_KEY_PEM":  keyBundle.KeyPEM,
		"PLOY_SERVER_URL":      serverURL,
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
		ScriptArgs:      []string{"--cluster-id", cfg.ClusterID, "--node-id", nodeID, "--node-address", cfg.Address},
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
		Scheme:          "https",
		SSHIdentityPath: identityPath,
	}
	if loadErr != nil || strings.TrimSpace(existing.ClusterID) == "" {
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
	return "", errors.New("ployd-node binary not found alongside CLI; provide --ployd-node-binary")
}

// pkiSignRequest is the JSON request body for POST /v1/pki/sign.
type pkiSignRequest struct {
	NodeID string `json:"node_id"`
	CSR    string `json:"csr"`
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
func signNodeCSR(ctx context.Context, serverURL, nodeID string, csrPEM []byte) (certPEM, caCertPEM string, err error) {
	reqBody := pkiSignRequest{
		NodeID: nodeID,
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
	defer resp.Body.Close()

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
