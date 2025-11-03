package main

import (
	"context"
	"errors"
	"flag"
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

// provisionHost indirection allows tests to stub remote provisioning to avoid
// real scp/ssh timeouts. Default is deploy.ProvisionHost.
var provisionHost = deploy.ProvisionHost

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
		address       stringValue
		postgresqlDSN stringValue
		identity      stringValue
		userFlag      stringValue
		ploydBin      stringValue
		sshPort       intValue
	)

	fs.Var(&address, "address", "Target host or IP address for server deployment")
	fs.Var(&postgresqlDSN, "postgresql-dsn", "PostgreSQL connection string (if not provided, PostgreSQL will be installed locally)")
	fs.Var(&identity, "identity", "SSH private key used for provisioning (default: ~/.ssh/id_rsa)")
	fs.Var(&userFlag, "user", "SSH username used for provisioning (default: root)")
	fs.Var(&ploydBin, "ployd-binary", "Path to the ployd server binary uploaded during provisioning (default: alongside the CLI)")
	fs.Var(&sshPort, "ssh-port", "SSH port for server provisioning (default: 22)")

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

	serverCfg := serverDeployConfig{
		Address:       address.value,
		PostgreSQLDSN: postgresqlDSN.value,
		User:          userFlag.value,
		IdentityFile:  identity.value,
		PloydBinary:   ploydBin.value,
		SSHPort:       sshPort.value,
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
	Address       string
	PostgreSQLDSN string
	User          string
	IdentityFile  string
	PloydBinary   string
	SSHPort       int
}

func runServerDeploy(cfg serverDeployConfig, stderr io.Writer) error {
	if stderr == nil {
		stderr = os.Stderr
	}
	ctx := context.Background()

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

	_, _ = fmt.Fprintf(stderr, "Deploying Ploy server to %s\n", cfg.Address)
	_, _ = fmt.Fprintf(stderr, "  SSH User: %s\n", user)
	_, _ = fmt.Fprintf(stderr, "  SSH Port: %d\n", sshPort)
	_, _ = fmt.Fprintf(stderr, "  Identity: %s\n", identityPath)
	_, _ = fmt.Fprintf(stderr, "  Binary: %s\n", ploydBinaryPath)

	// Generate cluster ID
	clusterID, err := deploy.GenerateClusterID()
	if err != nil {
		return fmt.Errorf("server deploy: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "Generated cluster ID: %s\n", clusterID)

	// Generate cluster CA and server certificate
	_, _ = fmt.Fprintln(stderr, "Generating cluster CA and server certificate...")
	now := time.Now()
	ca, err := pki.GenerateCA(clusterID, now)
	if err != nil {
		return fmt.Errorf("server deploy: generate CA: %w", err)
	}

	serverCert, err := pki.IssueServerCert(ca, clusterID, cfg.Address, now)
	if err != nil {
		return fmt.Errorf("server deploy: issue server cert: %w", err)
	}
	_, _ = fmt.Fprintln(stderr, "CA and server certificate generated")

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
		"PLOY_CA_CERT_PEM":        ca.CertPEM,
		"PLOY_CA_KEY_PEM":         ca.KeyPEM,
		"PLOY_SERVER_CERT_PEM":    serverCert.CertPEM,
		"PLOY_SERVER_KEY_PEM":     serverCert.KeyPEM,
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
