package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/config"
	"github.com/iw2rmb/ploy/internal/cli/controlplane"
	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/internal/pki"
)

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
		// The DSN will be derived by the bootstrap script
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

	// Only set PLOY_POSTGRES_DSN if the user provided one.
	if pgDSN != "" {
		scriptEnv["PLOY_POSTGRES_DSN"] = pgDSN
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

	// Save cluster descriptor locally.
	serverAddress, _ := controlplane.BaseURLFromDescriptor(config.Descriptor{Address: cfg.Address, Scheme: "https"})
	desc := config.Descriptor{
		ClusterID:       clusterID,
		Address:         serverAddress,
		Scheme:          "https",
		SSHIdentityPath: identityPath,
	}
	// Write local mTLS bundle for the default descriptor (only if not reusing)
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
