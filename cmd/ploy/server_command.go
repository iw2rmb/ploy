package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/cli/controlplane"
	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/internal/pki"
)

// provisionHost indirection allows tests to stub remote provisioning to avoid
// real scp/ssh timeouts. Default is deploy.ProvisionHost.
var provisionHost = deploy.ProvisionHost

// detectRunner allows tests to inject a mock runner for cluster detection.
// Default is nil, which causes DetectExisting to use systemRunner.
var detectRunner deploy.Runner

func handleServer(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printServerUsage(stderr)
		return errors.New("server subcommand required")
	}
	switch args[0] {
	case "deploy":
		return handleServerDeploy(args[1:], stderr)
	default:
		printServerUsage(stderr)
		return fmt.Errorf("unknown server subcommand %q", args[0])
	}
}

func handleServerDeploy(args []string, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("server deploy", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		address          stringValue
		postgresqlDSN    stringValue
		identity         stringValue
		userFlag         stringValue
		ploydBin         stringValue
		sshPort          intValue
		reuse            boolValue
		forceNewCA       boolValue
		refreshAdminCert boolValue
		dryRun           boolValue
	)

	fs.Var(&address, "address", "Target host or IP address for server deployment")
	fs.Var(&postgresqlDSN, "postgresql-dsn", "PostgreSQL connection string (if not provided, PostgreSQL will be installed locally)")
	fs.Var(&identity, "identity", "SSH private key used for provisioning (default: ~/.ssh/id_rsa)")
	fs.Var(&userFlag, "user", "SSH username used for provisioning (default: root)")
	fs.Var(&ploydBin, "ployd-binary", "Path to the ployd server binary uploaded during provisioning (default: alongside the CLI)")
	fs.Var(&sshPort, "ssh-port", "SSH port for server provisioning (default: 22)")
	fs.Var(&reuse, "reuse", "Reuse existing cluster CA and server certificate if present (default: true)")
	fs.Var(&forceNewCA, "force-new-ca", "Force generation of new CA and server certificate even if cluster exists")
	fs.Var(&refreshAdminCert, "refresh-admin-cert", "Refresh admin client certificate via server PKI endpoint")
	fs.Var(&dryRun, "dry-run", "Print detected cluster and planned actions without making changes")

	if err := fs.Parse(args); err != nil {
		printServerDeployUsage(stderr)
		return err
	}
	if fs.NArg() > 0 {
		printServerDeployUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !address.set || strings.TrimSpace(address.value) == "" {
		printServerDeployUsage(stderr)
		return errors.New("address is required")
	}

	// Work around Go flag behavior with custom boolean values: when a Value
	// implements IsBoolFlag, "--reuse=false" may still be parsed as present=true
	// depending on the stdlib behavior. Detect an explicit "=false" token to
	// ensure we honor it.
	var reuseExplicitFalse bool
	for _, a := range args {
		if a == "--reuse=false" || a == "-reuse=false" {
			reuseExplicitFalse = true
			break
		}
	}

	// Default --reuse to true unless explicit disable or --force-new-ca.
	reuseCA := true
	if reuse.set {
		reuseCA = reuse.value
	}
	if forceNewCA.set && forceNewCA.value {
		reuseCA = false
	}
	if reuseExplicitFalse {
		reuseCA = false
	}

	serverCfg := serverDeployConfig{
		Address:          address.value,
		PostgreSQLDSN:    postgresqlDSN.value,
		User:             userFlag.value,
		IdentityFile:     identity.value,
		PloydBinary:      ploydBin.value,
		SSHPort:          sshPort.value,
		Reuse:            reuseCA,
		RefreshAdminCert: refreshAdminCert.value,
		DryRun:           dryRun.value,
	}

	return runServerDeploy(serverCfg, stderr)
}

func printServerUsage(w io.Writer) {
	printCommandUsage(w, "server")
}

func printServerDeployUsage(w io.Writer) {
	printCommandUsage(w, "server", "deploy")
}

type serverDeployConfig struct {
	Address          string
	PostgreSQLDSN    string
	User             string
	IdentityFile     string
	PloydBinary      string
	SSHPort          int
	Reuse            bool
	RefreshAdminCert bool
	DryRun           bool
}

func runServerDeploy(cfg serverDeployConfig, stderr io.Writer) error {
	if stderr == nil {
		stderr = os.Stderr
	}
	ctx := context.Background()

	// Handle --refresh-admin-cert flag independently.
	if cfg.RefreshAdminCert {
		return handleRefreshAdminCert(ctx, stderr)
	}

	// Resolve default paths
	identityPath, err := resolveIdentityPath(stringValue{set: cfg.IdentityFile != "", value: cfg.IdentityFile})
	if err != nil {
		return fmt.Errorf("server deploy: %w", err)
	}

	ploydBinaryPath, err := resolvePloydBinaryPath(stringValue{set: cfg.PloydBinary != "", value: cfg.PloydBinary})
	if err != nil {
		return fmt.Errorf("server deploy: %w", err)
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
		return fmt.Errorf("server deploy: %w", err)
	}

	if cfg.DryRun {
		_, _ = fmt.Fprintf(stderr, "DRY RUN: Server deployment to %s\n", cfg.Address)
	} else {
		_, _ = fmt.Fprintf(stderr, "Deploying Ploy server to %s\n", cfg.Address)
	}
	_, _ = fmt.Fprintf(stderr, "  SSH User: %s\n", user)
	_, _ = fmt.Fprintf(stderr, "  SSH Port: %d\n", sshPort)
	_, _ = fmt.Fprintf(stderr, "  Identity: %s\n", identityPath)
	_, _ = fmt.Fprintf(stderr, "  Binary: %s\n", ploydBinaryPath)

	// Detect existing cluster if --reuse is enabled
	var clusterID string
	var ca *pki.CABundle
	var serverCert *pki.IssuedCert
	var adminCert *pki.IssuedCert
	var reusingCluster bool

	if cfg.Reuse {
		_, _ = fmt.Fprintln(stderr, "Checking for existing cluster...")
		detectOpts := deploy.ProvisionOptions{
			Address:      cfg.Address,
			User:         user,
			Port:         sshPort,
			IdentityFile: identityPath,
		}
		detection, err := deploy.DetectExisting(ctx, detectRunner, detectOpts)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "Warning: failed to detect existing cluster: %v\n", err)
		} else if detection.Found {
			if detection.ClusterID != "" {
				clusterID = detection.ClusterID
				reusingCluster = true
				_, _ = fmt.Fprintf(stderr, "Found existing cluster: %s (reusing CA and server certificate)\n", clusterID)
			} else {
				_, _ = fmt.Fprintln(stderr, "Found existing PKI files but could not extract cluster ID; will generate new CA")
			}
		} else {
			_, _ = fmt.Fprintln(stderr, "No existing cluster found; will generate new CA")
		}
	}

	// In dry-run mode, print planned actions and exit before making changes.
	if cfg.DryRun {
		_, _ = fmt.Fprintln(stderr, "\nPlanned actions:")
		if reusingCluster {
			_, _ = fmt.Fprintf(stderr, "  - Reuse existing cluster ID: %s\n", clusterID)
			_, _ = fmt.Fprintln(stderr, "  - Reuse existing CA and server certificate")
			_, _ = fmt.Fprintln(stderr, "  - Skip PKI generation")
		} else {
			_, _ = fmt.Fprintln(stderr, "  - Generate new cluster ID")
			_, _ = fmt.Fprintln(stderr, "  - Generate new CA certificate")
			_, _ = fmt.Fprintln(stderr, "  - Issue server certificate")
			_, _ = fmt.Fprintf(stderr, "    Subject: CN=ployd-<cluster-id>, O=Ploy\n")
			_, _ = fmt.Fprintln(stderr, "  - Issue admin client certificate")
			_, _ = fmt.Fprintf(stderr, "    Subject: CN=cli-admin-<cluster-id>, OU=Ploy role=cli-admin, O=Ploy\n")
		}
		if cfg.PostgreSQLDSN == "" {
			_, _ = fmt.Fprintln(stderr, "  - Install PostgreSQL on target host")
		} else {
			_, _ = fmt.Fprintln(stderr, "  - Use provided PostgreSQL DSN")
		}
		_, _ = fmt.Fprintln(stderr, "  - Upload ployd binary to target host")
		_, _ = fmt.Fprintln(stderr, "  - Bootstrap server (install systemd service, configure firewall)")
		_, _ = fmt.Fprintln(stderr, "  - Start ployd service")
		if !reusingCluster {
			_, _ = fmt.Fprintln(stderr, "  - Save cluster descriptor and admin certificates locally")
		}
		_, _ = fmt.Fprintln(stderr, "\nDry run complete. No changes have been made.")
		return nil
	}

	// Generate cluster ID and PKI if not reusing
	if !reusingCluster {
		var err error
		clusterID, err = deploy.GenerateClusterID()
		if err != nil {
			return fmt.Errorf("server deploy: %w", err)
		}
		_, _ = fmt.Fprintf(stderr, "Generated cluster ID: %s\n", clusterID)

		// Generate cluster CA and server certificate
		_, _ = fmt.Fprintln(stderr, "Generating cluster CA and server certificate...")
		now := time.Now()
		ca, err = pki.GenerateCA(clusterID, now)
		if err != nil {
			return fmt.Errorf("server deploy: generate CA: %w", err)
		}

		serverCert, err = pki.IssueServerCert(ca, clusterID, cfg.Address, now)
		if err != nil {
			return fmt.Errorf("server deploy: issue server cert: %w", err)
		}
		_, _ = fmt.Fprintln(stderr, "CA and server certificate generated")

		// Generate a CLI-admin client certificate for local descriptor/mTLS
		adminCert, err = pki.IssueClientCert(ca, clusterID, now)
		if err != nil {
			return fmt.Errorf("server deploy: issue cli-admin cert: %w", err)
		}
	}

	// Determine PostgreSQL DSN
	pgDSN := strings.TrimSpace(cfg.PostgreSQLDSN)
	installPostgres := pgDSN == ""

	if installPostgres {
		_, _ = fmt.Fprintln(stderr, "No PostgreSQL DSN provided; will install PostgreSQL on target host")
		// The DSN will be derived by the bootstrap script after installing PostgreSQL
		// and validating connectivity. We pass an empty value here, and the bootstrap
		// script will export PLOY_SERVER_PG_DSN once ready.
		pgDSN = ""
	} else {
		_, _ = fmt.Fprintf(stderr, "Using provided PostgreSQL DSN\n")
	}

	// Prepare environment variables for bootstrap script
	scriptEnv := map[string]string{
		"CLUSTER_ID":              clusterID,
		"NODE_ID":                 "control",
		"NODE_ADDRESS":            cfg.Address,
		"BOOTSTRAP_PRIMARY":       "true",
		"PLOY_INSTALL_POSTGRESQL": boolToString(installPostgres),
	}

	// Only include PKI environment variables when NOT reusing
	if !reusingCluster {
		scriptEnv["PLOY_CA_CERT_PEM"] = ca.CertPEM
		scriptEnv["PLOY_CA_KEY_PEM"] = ca.KeyPEM
		scriptEnv["PLOY_SERVER_CERT_PEM"] = serverCert.CertPEM
		scriptEnv["PLOY_SERVER_KEY_PEM"] = serverCert.KeyPEM
	}

	// Only set PLOY_SERVER_PG_DSN if the user provided one.
	// When installing PostgreSQL, the bootstrap script will derive and export the DSN.
	if pgDSN != "" {
		scriptEnv["PLOY_SERVER_PG_DSN"] = pgDSN
	}

	// Provision the server host
	_, _ = fmt.Fprintln(stderr, "Installing ployd binary and bootstrapping server...")
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
		ScriptArgs:      []string{"--cluster-id", clusterID, "--node-id", "control", "--node-address", cfg.Address, "--primary"},
		ServiceChecks:   []string{"ployd"},
	}

	if err := provisionHost(ctx, provisionOpts); err != nil {
		return fmt.Errorf("server deploy: provision host: %w", err)
	}

	// Save cluster descriptor locally. Use the same normalization as the CLI control-plane helper
	// to correctly handle IPv6 addresses and explicit ports.
	serverAddress, _ := controlplane.BaseURLFromDescriptor(config.Descriptor{Address: cfg.Address, Scheme: "https"})
	desc := config.Descriptor{
		ClusterID:       clusterID,
		Address:         serverAddress,
		Scheme:          "https",
		SSHIdentityPath: identityPath,
	}
	// Write local mTLS bundle for the default descriptor (only if not reusing, as we don't have admin cert)
	if !reusingCluster && adminCert != nil {
		caPath, certPath, keyPath, err := writeLocalAdminBundle(clusterID, ca.CertPEM, adminCert.CertPEM, adminCert.KeyPEM)
		if err == nil {
			desc.CAPath = caPath
			desc.CertPath = certPath
			desc.KeyPath = keyPath
		} else {
			_, _ = fmt.Fprintf(stderr, "Warning: failed to write local admin mTLS bundle: %v\n", err)
		}
	} else if reusingCluster {
		_, _ = fmt.Fprintln(stderr, "Skipped admin certificate generation (reusing existing cluster)")
		_, _ = fmt.Fprintln(stderr, "Note: Use --refresh-admin-cert to obtain a new admin certificate from the server")
	}
	if _, err := config.SaveDescriptor(desc); err != nil {
		_, _ = fmt.Fprintf(stderr, "Warning: failed to save cluster descriptor: %v\n", err)
	} else {
		// Set this cluster as the default
		if err := config.SetDefault(clusterID); err != nil {
			_, _ = fmt.Fprintf(stderr, "Warning: failed to set default cluster: %v\n", err)
		} else {
			_, _ = fmt.Fprintf(stderr, "Cluster descriptor saved to ~/.config/ploy/clusters/%s.json\n", clusterID)
		}
	}

	_, _ = fmt.Fprintln(stderr, "\nServer deployment complete!")
	_, _ = fmt.Fprintf(stderr, "Cluster ID: %s\n", clusterID)
	_, _ = fmt.Fprintf(stderr, "Server address: %s\n", serverAddress)
	_, _ = fmt.Fprintln(stderr, "\nNext steps:")
	_, _ = fmt.Fprintf(stderr, "  1. Add worker nodes with: ploy node add --cluster-id %s --address <node-address>\n", clusterID)
	_, _ = fmt.Fprintln(stderr, "  2. Configure your local environment to point to this server")

	return nil
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// writeLocalAdminBundle writes CA and admin cert/key under the config home.
func writeLocalAdminBundle(clusterID, caPEM, certPEM, keyPEM string) (caPath, certPath, keyPath string, err error) {
	base, err := resolveConfigBaseDir()
	if err != nil {
		return "", "", "", err
	}
	dir := filepath.Join(base, "certs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", "", err
	}
	caPath = filepath.Join(dir, fmt.Sprintf("%s-ca.crt", clusterID))
	certPath = filepath.Join(dir, fmt.Sprintf("%s-admin.crt", clusterID))
	keyPath = filepath.Join(dir, fmt.Sprintf("%s-admin.key", clusterID))
	if err := os.WriteFile(caPath, []byte(strings.TrimSpace(caPEM)+"\n"), 0o644); err != nil {
		return "", "", "", err
	}
	if err := os.WriteFile(certPath, []byte(strings.TrimSpace(certPEM)+"\n"), 0o644); err != nil {
		return "", "", "", err
	}
	// Ensure 0600 for the private key regardless of umask
	if err := writeFile0600(keyPath, []byte(strings.TrimSpace(keyPEM)+"\n")); err != nil {
		return "", "", "", err
	}
	return caPath, certPath, keyPath, nil
}

func writeFile0600(path string, data []byte) error {
	// Atomic write with proper mode
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	// os.Rename preserves mode bits
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// Ensure mode is exactly 0600
	return os.Chmod(path, fs.FileMode(0o600))
}

// resolveConfigBaseDir mirrors internal/cli/config clusters dir resolution to find the base.
func resolveConfigBaseDir() (string, error) {
	base := strings.TrimSpace(os.Getenv("PLOY_CONFIG_HOME"))
	if base == "" {
		xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
		if xdg != "" {
			base = filepath.Join(xdg, "ploy")
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".config", "ploy")
		}
	}
	return base, nil
}

// handleRefreshAdminCert refreshes the admin certificate from the server.
func handleRefreshAdminCert(ctx context.Context, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}

	// Load the default cluster descriptor to get cluster ID and server address.
	desc, err := config.LoadDefault()
	if err != nil {
		return fmt.Errorf("refresh admin cert: load default cluster descriptor: %w", err)
	}
	if desc.ClusterID == "" {
		return errors.New("refresh admin cert: cluster ID not found in descriptor")
	}

	_, _ = fmt.Fprintf(stderr, "Refreshing admin certificate for cluster: %s\n", desc.ClusterID)

	// Call server to sign new admin cert.
	caPEM, certPEM, keyPEM, err := refreshAdminCertFromServer(ctx, desc.ClusterID, stderr)
	if err != nil {
		return fmt.Errorf("refresh admin cert: %w", err)
	}

	// Write local admin bundle.
	caPath, certPath, keyPath, err := writeLocalAdminBundle(desc.ClusterID, caPEM, certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("refresh admin cert: write local bundle: %w", err)
	}

	_, _ = fmt.Fprintf(stderr, "Admin certificate bundle written:\n")
	_, _ = fmt.Fprintf(stderr, "  CA: %s\n", caPath)
	_, _ = fmt.Fprintf(stderr, "  Cert: %s\n", certPath)
	_, _ = fmt.Fprintf(stderr, "  Key: %s\n", keyPath)

	// Update descriptor with new paths.
	desc.CAPath = caPath
	desc.CertPath = certPath
	desc.KeyPath = keyPath

	if _, err := config.SaveDescriptor(desc); err != nil {
		return fmt.Errorf("refresh admin cert: save descriptor: %w", err)
	}

	_, _ = fmt.Fprintf(stderr, "\nAdmin certificate refreshed successfully!\n")
	_, _ = fmt.Fprintf(stderr, "Cluster descriptor updated: ~/.config/ploy/clusters/%s.json\n", desc.ClusterID)

	return nil
}

// generateAdminCSR generates a CSR for cli-admin with proper OU and ExtKeyUsage.
func generateAdminCSR(clusterID string) (csrPEM, keyPEM []byte, err error) {
	// Generate ECDSA private key.
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate admin key: %w", err)
	}

	// Create CSR template with proper OU and CN.
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         "cli-admin-" + clusterID,
			Organization:       []string{"Ploy"},
			OrganizationalUnit: []string{"Ploy role=cli-admin"},
		},
	}

	// Add ExtKeyUsage extension for ClientAuth.
	// ClientAuth OID is 1.3.6.1.5.5.7.3.2.
	clientAuthOID := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 2}
	ekuValue, err := asn1.Marshal([]asn1.ObjectIdentifier{clientAuthOID})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal EKU: %w", err)
	}
	template.ExtraExtensions = []pkix.Extension{
		{
			Id:    asn1.ObjectIdentifier{2, 5, 29, 37}, // ExtKeyUsage OID
			Value: ekuValue,
		},
	}

	// Create CSR.
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create CSR: %w", err)
	}

	// Encode CSR to PEM.
	csrPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	// Encode private key to PEM.
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal admin private key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return csrPEM, keyPEM, nil
}

// refreshAdminCertFromServer generates a CSR and calls the server PKI endpoint to sign it.
func refreshAdminCertFromServer(ctx context.Context, clusterID string, stderr io.Writer) (caPEM, certPEM, keyPEM string, err error) {
	if stderr == nil {
		stderr = io.Discard
	}

	// Generate CSR and private key.
	_, _ = fmt.Fprintln(stderr, "Generating admin certificate signing request...")
	csrPEMBytes, keyPEMBytes, err := generateAdminCSR(clusterID)
	if err != nil {
		return "", "", "", fmt.Errorf("generate admin CSR: %w", err)
	}

	// Get server URL and HTTP client from descriptor.
	serverURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve control plane URL: %w", err)
	}

	// Build request body.
	reqBody := struct {
		CSR string `json:"csr"`
	}{
		CSR: string(csrPEMBytes),
	}
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", "", fmt.Errorf("marshal request: %w", err)
	}

	// Call server PKI endpoint.
	endpoint := strings.TrimSuffix(serverURL.String(), "/") + "/v1/pki/sign/admin"
	_, _ = fmt.Fprintf(stderr, "Requesting admin certificate from server: %s\n", endpoint)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("call server PKI endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		bodyStr := strings.TrimSpace(string(bodyBytes))
		if bodyStr != "" {
			return "", "", "", fmt.Errorf("server returned status %d: %s", resp.StatusCode, bodyStr)
		}
		return "", "", "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// Decode response.
	var signResp struct {
		Certificate string `json:"certificate"`
		CABundle    string `json:"ca_bundle"`
		Serial      string `json:"serial"`
		Fingerprint string `json:"fingerprint"`
		NotBefore   string `json:"not_before"`
		NotAfter    string `json:"not_after"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&signResp); err != nil {
		return "", "", "", fmt.Errorf("decode response: %w", err)
	}

	_, _ = fmt.Fprintf(stderr, "Admin certificate issued successfully\n")
	_, _ = fmt.Fprintf(stderr, "  Serial: %s\n", signResp.Serial)
	_, _ = fmt.Fprintf(stderr, "  Fingerprint: %s\n", signResp.Fingerprint)
	_, _ = fmt.Fprintf(stderr, "  Valid: %s to %s\n", signResp.NotBefore, signResp.NotAfter)

	return signResp.CABundle, signResp.Certificate, string(keyPEMBytes), nil
}
