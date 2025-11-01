package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
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
		return errors.New("--cluster-id is required")
	}
	if !address.set || strings.TrimSpace(address.value) == "" {
		printNodeAddUsage(stderr)
		return errors.New("--address is required")
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
	// TODO: Implement node addition logic.
	// This should:
	// 1. Install ployd-node binary on target host via SSH
	// 2. Generate private key and CSR on the node
	// 3. Submit CSR to server's /v1/pki/sign endpoint for signing
	// 4. Install issued node certificate and CA bundle on the node
	// 5. Record node IP in the database (via server API)
	// 6. Bootstrap ployd-node systemd unit pointing at PLOY_NODE_SERVER_URL
	// 7. Configure mTLS for node-to-server and server-to-node communication

	_, _ = fmt.Fprintf(stderr, "node add: cluster-id=%s address=%s\n", cfg.ClusterID, cfg.Address)
	_, _ = fmt.Fprintln(stderr, "Node addition is not yet implemented.")
	_, _ = fmt.Fprintln(stderr, "TODO: Install ployd-node, generate CSR, sign via /v1/pki/sign, configure mTLS.")
	return errors.New("node add not yet implemented")
}
