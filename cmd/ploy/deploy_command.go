package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"github.com/iw2rmb/ploy/internal/deploy"
)

const (
	defaultDomainSuffix      = ".ploy"
	defaultClusterIDAlphabet = "0123456789abcdef"
	defaultClusterIDLength   = 16
	defaultWorkerIDAlphabet  = "0123456789abcdef"
	defaultWorkerIDLength    = 4
	defaultAPIKeyAlphabet    = "0123456789abcdef"
	defaultAPIKeyLength      = 64
)

var deployBootstrapRunner = deploy.RunBootstrap

// handleDeploy routes deploy subcommands.
func handleDeploy(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printDeployUsage(stderr)
		return errors.New("deploy subcommand required")
	}

	switch args[0] {
	case "bootstrap":
		return handleDeployBootstrap(args[1:], stderr)
	default:
		printDeployUsage(stderr)
		return fmt.Errorf("unknown deploy subcommand %q", args[0])
	}
}

func handleDeployBootstrap(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("deploy bootstrap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		userFlag stringValue
		identity stringValue
		address  stringValue
		control  stringValue
		beacon   stringValue
		ploydBin stringValue
	)

	fs.Var(&userFlag, "user", "SSH username (default: root)")
	fs.Var(&identity, "identity", "SSH identity file (default: ~/.ssh/id_rsa)")
	fs.Var(&address, "address", "Override SSH target address (defaults to host)")
	fs.Var(&control, "control-plane-url", "Control plane endpoint recorded in the local descriptor")
	fs.Var(&beacon, "beacon-url", "Beacon URL recorded in the local descriptor (default: https://<node-id>.<cluster-id>.ploy)")
	fs.Var(&ploydBin, "ployd-binary", "Path to the ployd binary uploaded during bootstrap (default: alongside the CLI)")

	if err := fs.Parse(args); err != nil {
		printDeployBootstrapUsage(stderr)
		return err
	}

	if fs.NArg() > 0 {
		printDeployBootstrapUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	var opts deploy.Options
	if userFlag.set {
		opts.User = strings.TrimSpace(userFlag.value)
	}
	if identity.set {
		opts.IdentityFile = expandPath(strings.TrimSpace(identity.value))
	}
	if address.set {
		opts.Address = strings.TrimSpace(address.value)
	}
	clusterID, err := gonanoid.Generate(defaultClusterIDAlphabet, defaultClusterIDLength)
	if err != nil {
		return fmt.Errorf("generate cluster identifier: %w", err)
	}
	opts.ClusterID = clusterID
	manualBeaconURL := strings.TrimSpace(beacon.value)
	opts.ControlPlaneURL = strings.TrimSpace(control.value)

	opts.Host = opts.ClusterID + defaultDomainSuffix

	nodeID, err := gonanoid.Generate(defaultWorkerIDAlphabet, defaultWorkerIDLength)
	if err != nil {
		return fmt.Errorf("generate node identifier: %w", err)
	}
	opts.InitialBeacons = []string{nodeID}
	opts.InitialWorkers = []string{nodeID}

	if manualBeaconURL != "" {
		opts.BeaconURL = manualBeaconURL
	} else {
		opts.BeaconURL = fmt.Sprintf("https://%s.%s%s", nodeID, opts.ClusterID, defaultDomainSuffix)
	}

	apiKey, err := gonanoid.Generate(defaultAPIKeyAlphabet, defaultAPIKeyLength)
	if err != nil {
		return fmt.Errorf("generate api key: %w", err)
	}
	opts.APIKey = apiKey

	connectHost := strings.TrimSpace(opts.Address)
	if connectHost == "" {
		connectHost = strings.TrimSpace(opts.Host)
	}
	if connectHost != "" {
		etcdHost := connectHost
		if strings.Contains(etcdHost, ":") && !strings.Contains(etcdHost, "]") && !strings.Contains(etcdHost, "[") {
			etcdHost = "[" + etcdHost + "]"
		}
		opts.EtcdEndpoints = []string{fmt.Sprintf("http://%s:2379", etcdHost)}
	}

	if opts.IdentityFile == "" {
		opts.IdentityFile = defaultIdentityPath()
	} else {
		opts.IdentityFile = expandPath(opts.IdentityFile)
	}

	if ploydBin.set {
		opts.PloydBinaryPath = expandPath(strings.TrimSpace(ploydBin.value))
	} else {
		path, err := defaultPloydBinaryPath()
		if err != nil {
			return err
		}
		opts.PloydBinaryPath = path
	}

	opts.Stdout = stderr
	opts.Stderr = stderr
	opts.Stdin = os.Stdin
	opts.WorkstationOS = runtime.GOOS
	if opts.BeaconURL == "" {
		printDeployBootstrapUsage(stderr)
		return errors.New("beacon-url is required")
	}
	if opts.APIKey == "" {
		printDeployBootstrapUsage(stderr)
		return errors.New("api-key is required")
	}
	if len(opts.InitialBeacons) == 0 {
		printDeployBootstrapUsage(stderr)
		return errors.New("at least one beacon-id is required")
	}

	if err := deployBootstrapRunner(context.Background(), opts); err != nil {
		return err
	}

	return nil
}

func printDeployUsage(w io.Writer) {
	printCommandUsage(w, "deploy")
}

func printDeployBootstrapUsage(w io.Writer) {
	printCommandUsage(w, "deploy", "bootstrap")
}

func defaultPloydBinaryPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate ploy executable: %w", err)
	}
	dir := filepath.Dir(execPath)
	candidates := []string{
		filepath.Join(dir, "ployd"),
	}
	if runtime.GOOS == "windows" {
		candidates = append([]string{filepath.Join(dir, "ployd.exe")}, candidates...)
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}
		return candidate, nil
	}
	return "", errors.New("ploy deploy bootstrap: ployd binary not found alongside CLI; provide --ployd-binary")
}

type stringValue struct {
	value string
	set   bool
}

func (s *stringValue) Set(value string) error {
	s.value = value
	s.set = true
	return nil
}

func (s *stringValue) String() string {
	return s.value
}

type intValue struct {
	value int
	set   bool
}

func (i *intValue) Set(value string) error {
	v, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("parse int flag: %w", err)
	}
	i.value = v
	i.set = true
	return nil
}

func (i *intValue) String() string {
	if !i.set {
		return ""
	}
	return strconv.Itoa(i.value)
}

func defaultIdentityPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "id_rsa")
}

func expandPath(path string) string {
	if path == "" {
		return ""
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
