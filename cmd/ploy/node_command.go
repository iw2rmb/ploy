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
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/internal/pki"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func handleNode(args []string, stderr io.Writer) error {
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

func handleNodeAdd(args []string, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("node add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		clusterID stringValue
		address   stringValue
		identity  stringValue
		userFlag  stringValue
		ploydBin  stringValue
		sshPort   intValue
	)

	fs.Var(&clusterID, "cluster-id", "Cluster identifier to join")
	fs.Var(&address, "address", "Target host or IP address for node deployment")
	fs.Var(&identity, "identity", "SSH private key used for provisioning (default: ~/.ssh/id_rsa)")
	fs.Var(&userFlag, "user", "SSH username used for provisioning (default: root)")
	fs.Var(&ploydBin, "ployd-binary", "Path to the ployd-node binary uploaded during provisioning (default: alongside the CLI)")
	fs.Var(&sshPort, "ssh-port", "SSH port for node provisioning (default: 22)")

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
		ClusterID:    clusterID.value,
		Address:      address.value,
		User:         userFlag.value,
		IdentityFile: identity.value,
		PloydBinary:  ploydBin.value,
		SSHPort:      sshPort.value,
	}

	return runNodeAdd(nodeCfg, stderr)
}

func printNodeUsage(w io.Writer) {
	printCommandUsage(w, "node")
}

func printNodeAddUsage(w io.Writer) {
	printCommandUsage(w, "node", "add")
}

type nodeAddConfig struct {
	ClusterID    string
	Address      string
	User         string
	IdentityFile string
	PloydBinary  string
	SSHPort      int
}

func runNodeAdd(cfg nodeAddConfig, stderr io.Writer) error {
	if stderr == nil {
		stderr = os.Stderr
	}
	ctx := context.Background()

	// Resolve default paths.
	identityPath, err := resolveIdentityPath(stringValue{set: cfg.IdentityFile != "", value: cfg.IdentityFile})
	if err != nil {
		return fmt.Errorf("node add: %w", err)
	}

	ploydBinaryPath, err := resolvePloydBinaryPath(stringValue{set: cfg.PloydBinary != "", value: cfg.PloydBinary})
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

	_, _ = fmt.Fprintf(stderr, "Adding Ploy node to cluster %s at %s\n", cfg.ClusterID, cfg.Address)
	_, _ = fmt.Fprintf(stderr, "  SSH User: %s\n", user)
	_, _ = fmt.Fprintf(stderr, "  SSH Port: %d\n", sshPort)
	_, _ = fmt.Fprintf(stderr, "  Identity: %s\n", identityPath)
	_, _ = fmt.Fprintf(stderr, "  Binary: %s\n", ploydBinaryPath)

	// Generate node ID.
	nodeID, err := deploy.GenerateNodeID()
	if err != nil {
		return fmt.Errorf("node add: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "Generated node ID: %s\n", nodeID)

	// Generate node private key and CSR.
	_, _ = fmt.Fprintln(stderr, "Generating node private key and CSR...")
	nodeKey, csrPEM, err := pki.GenerateNodeCSR(nodeID, cfg.ClusterID, cfg.Address)
	if err != nil {
		return fmt.Errorf("node add: generate CSR: %w", err)
	}
	_, _ = fmt.Fprintln(stderr, "Node key and CSR generated")

	// Get server URL and database DSN from environment.
	// The server URL is needed to call the PKI sign endpoint.
	// The database DSN is needed to record the node.
	// For now, we require the user to provide a --server-url flag or PLOY_SERVER_URL env var.
	// In production, this would be derived from the cluster configuration.
	serverURL := os.Getenv("PLOY_SERVER_URL")
	if strings.TrimSpace(serverURL) == "" {
		return fmt.Errorf("node add: PLOY_SERVER_URL environment variable required (e.g., https://<server-ip>:8443)")
	}

	pgDSN := os.Getenv("PLOY_SERVER_PG_DSN")
	if strings.TrimSpace(pgDSN) == "" {
		return fmt.Errorf("node add: PLOY_SERVER_PG_DSN environment variable required")
	}

	// Parse node UUID from the node ID.
	nodeUUID, err := parseNodeUUID(nodeID)
	if err != nil {
		return fmt.Errorf("node add: %w", err)
	}

	// Record node in database.
	_, _ = fmt.Fprintln(stderr, "Recording node in database...")
	if err := recordNode(ctx, pgDSN, nodeUUID, nodeID, cfg.Address); err != nil {
		return fmt.Errorf("node add: record node: %w", err)
	}
	_, _ = fmt.Fprintln(stderr, "Node recorded in database")

	// Submit CSR to server for signing.
	_, _ = fmt.Fprintln(stderr, "Submitting CSR to server for signing...")
	nodeCert, caBundle, err := signNodeCSR(ctx, serverURL, nodeUUID.String(), csrPEM)
	if err != nil {
		return fmt.Errorf("node add: sign CSR: %w", err)
	}
	_, _ = fmt.Fprintln(stderr, "CSR signed successfully")

	// Prepare environment variables for bootstrap script.
	scriptEnv := map[string]string{
		"CLUSTER_ID":            cfg.ClusterID,
		"NODE_ID":               nodeID,
		"NODE_ADDRESS":          cfg.Address,
		"BOOTSTRAP_PRIMARY":     "false",
		"PLOY_NODE_SERVER_URL":  serverURL,
		"PLOY_NODE_CERT_PEM":    nodeCert,
		"PLOY_NODE_KEY_PEM":     nodeKey.KeyPEM,
		"PLOY_CA_CERT_PEM":      caBundle,
		"PLOY_INSTALL_POSTGRES": "false",
	}

	// Provision the node host.
	_, _ = fmt.Fprintln(stderr, "Installing ployd binary and bootstrapping node...")
	provisionOpts := deploy.ProvisionOptions{
		Host:            cfg.Address,
		Address:         cfg.Address,
		User:            user,
		Port:            sshPort,
		IdentityFile:    identityPath,
		PloydBinaryPath: ploydBinaryPath,
		Stdout:          os.Stdout,
		Stderr:          stderr,
		ScriptEnv:       scriptEnv,
		ScriptArgs:      []string{"--cluster-id", cfg.ClusterID, "--node-id", nodeID, "--node-address", cfg.Address},
		ServiceChecks:   []string{"ployd-node"},
	}

	if err := deploy.ProvisionHost(ctx, provisionOpts); err != nil {
		return fmt.Errorf("node add: provision host: %w", err)
	}

	_, _ = fmt.Fprintln(stderr, "\nNode addition complete!")
	_, _ = fmt.Fprintf(stderr, "Node ID: %s\n", nodeID)
	_, _ = fmt.Fprintf(stderr, "Node address: %s\n", cfg.Address)
	_, _ = fmt.Fprintf(stderr, "Server URL: %s\n", serverURL)

	return nil
}

// parseNodeUUID extracts or generates a UUID from a node ID.
// Node IDs are in the format "node-<hex>". We'll generate a UUID from the node ID.
func parseNodeUUID(nodeID string) (uuid.UUID, error) {
	// Generate a new UUID for the node.
	// The node ID string will be stored in the name field.
	nodeUUID := uuid.New()
	return nodeUUID, nil
}

// recordNode creates a node entry in the database.
func recordNode(ctx context.Context, dsn string, nodeUUID uuid.UUID, nodeName, nodeIP string) error {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	queries := store.New(pool)

	// Parse IP address.
	addr, err := netip.ParseAddr(nodeIP)
	if err != nil {
		return fmt.Errorf("parse node IP: %w", err)
	}

	// Create node in database.
	params := store.CreateNodeParams{
		Name:        nodeName,
		IpAddress:   addr,
		Concurrency: 1,
	}

	_, err = queries.CreateNode(ctx, params)
	if err != nil {
		return fmt.Errorf("create node: %w", err)
	}

	return nil
}

// signNodeCSR submits a CSR to the server's PKI sign endpoint and returns the signed certificate and CA bundle.
func signNodeCSR(ctx context.Context, serverURL, nodeID string, csrPEM []byte) (string, string, error) {
	pkiURL := fmt.Sprintf("%s/v1/pki/sign", strings.TrimRight(serverURL, "/"))

	reqBody := httpserver.PKISignRequest{
		NodeID: nodeID,
		CSR:    string(csrPEM),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pkiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("PKI sign failed with status %d", resp.StatusCode)
	}

	var respBody httpserver.PKISignResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return "", "", fmt.Errorf("decode response: %w", err)
	}

	return respBody.Certificate, respBody.CABundle, nil
}
