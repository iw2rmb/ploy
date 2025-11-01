package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

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
		address      stringValue
		postgresqlDSN stringValue
		identity     stringValue
		userFlag     stringValue
		ploydBin     stringValue
		sshPort      intValue
	)

	fs.Var(&address, "address", "Target host or IP address for server deployment")
	fs.Var(&postgresqlDSN, "postgresql-dsn", "PostgreSQL connection string (if not provided, PostgreSQL will be installed locally)")
	fs.Var(&identity, "identity", "SSH private key used for provisioning (default: ~/.ssh/id_rsa)")
	fs.Var(&userFlag, "user", "SSH username used for provisioning (default: root)")
	fs.Var(&ploydBin, "ployd-binary", "Path to the ployd-server binary uploaded during provisioning (default: alongside the CLI)")
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
		return errors.New("--address is required")
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
	// TODO: Implement server deployment logic.
	// This should:
	// 1. Install ployd-server binary on target host via SSH
	// 2. Generate cluster CA and issue server TLS certificate
	// 3. Create cluster_id and persist it in Postgres and on disk
	// 4. If PostgreSQLDSN is empty, install PostgreSQL locally and create 'ploy' database
	// 5. Bootstrap ployd-server systemd unit with PLOY_SERVER_PG_DSN
	// 6. Store cluster descriptor locally for CLI access

	_, _ = fmt.Fprintf(stderr, "server deploy: address=%s postgresql-dsn=%s\n", cfg.Address, cfg.PostgreSQLDSN)
	_, _ = fmt.Fprintln(stderr, "Server deployment is not yet implemented.")
	_, _ = fmt.Fprintln(stderr, "TODO: Install ployd-server, generate CA, create cluster, configure PostgreSQL.")
	return errors.New("server deploy not yet implemented")
}
