package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// provisionHost indirection allows tests to stub remote provisioning to avoid
// real scp/ssh timeouts. Default is deploy.ProvisionHost.
// Declared in server_deploy_remote.go.

// detectRunner allows tests to inject a mock runner for cluster detection.
// Declared in server_deploy_remote.go.

func handleServer(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	if wantsHelp(args) {
		printServerUsage(stderr)
		return nil
	}
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
