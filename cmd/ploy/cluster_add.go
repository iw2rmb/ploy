package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/deploy"
)

const defaultSSHPort = 22
const defaultSSHUser = "root"
const remotePKIDir = "/etc/ploy/pki"
const remoteControlPlaneCAPath = remotePKIDir + "/control-plane-ca.pem"
const remoteNodeCertPath = remotePKIDir + "/node.pem"
const remoteNodeKeyPath = remotePKIDir + "/node-key.pem"
const remoteConfigPath = "/etc/ploy/ployd.yaml"

var (
	clusterBootstrapRunner                               = deploy.RunBootstrap
	clusterProvisionHost                                 = deploy.ProvisionHost
	clusterWorkerRegister                                = registerWorker
	clusterHTTPClientFactory descriptorHTTPClientFactory = newDescriptorHTTPClient
	remoteCommandExecutor                                = runRemoteCommand
	remoteFileWriter                                     = writeRemoteFile
)

func handleClusterAdd(args []string, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("cluster add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		address   stringValue
		clusterID stringValue
		identity  stringValue
		userFlag  stringValue
		control   stringValue
		ploydBin  stringValue
		sshPort   intValue
	)
	labels := make(labelMap)
	probes := make(probeList, 0)
	dryRun := fs.Bool("dry-run", false, "Preview worker onboarding without registering the node")

	fs.Var(&address, "address", "Target host or IP address")
	fs.Var(&clusterID, "cluster-id", "Existing cluster identifier to join as a worker")
	fs.Var(&identity, "identity", "SSH private key used for provisioning (default: ~/.ssh/id_rsa)")
	fs.Var(&userFlag, "user", "SSH username used for provisioning (default: root)")
	fs.Var(&control, "control-plane-url", "Control-plane endpoint recorded during bootstrap (default: http://127.0.0.1:9094)")
	fs.Var(&ploydBin, "ployd-binary", "Path to the ployd binary uploaded during provisioning (default: alongside the CLI)")
	fs.Var(&sshPort, "ssh-port", "SSH port for worker provisioning (default: 22)")
	fs.Var(&labels, "label", "Apply a worker label key=value (worker mode only). May be repeated.")
	fs.Var(&probes, "health-probe", "Register a worker health probe as name=url (worker mode only). May be repeated.")

	if err := fs.Parse(args); err != nil {
		printClusterAddUsage(stderr)
		return err
	}
	if fs.NArg() > 0 {
		printClusterAddUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !address.set || strings.TrimSpace(address.value) == "" {
		printClusterAddUsage(stderr)
		return errors.New("address is required")
	}
	isWorker := clusterID.set && strings.TrimSpace(clusterID.value) != ""
	if !isWorker {
		return runClusterBootstrap(address.value, userFlag, identity, control, ploydBin, sshPort, stderr)
	}
	if control.set {
		printClusterAddUsage(stderr)
		return errors.New("--control-plane-url is only valid when bootstrapping the first node")
	}
	workerIdentity, err := resolveIdentityPath(identity)
	if err != nil {
		return err
	}
	workerPloyd, err := resolvePloydBinaryPath(ploydBin)
	if err != nil {
		return err
	}
	workerCfg := workerProvisionConfig{
		ClusterID:     clusterID.value,
		WorkerAddress: address.value,
		User:          userFlag.value,
		IdentityFile:  workerIdentity,
		PloydBinary:   workerPloyd,
		SSHPort:       sshPort.value,
		Labels:        cloneLabelMap(labels),
		Probes:        append(make(probeList, 0, len(probes)), probes...),
		DryRun:        *dryRun,
	}
	return runClusterWorkerAdd(workerCfg, stderr)
}

func printClusterAddUsage(w io.Writer) {
	printCommandUsage(w, "cluster", "add")
}
