package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"github.com/iw2rmb/ploy/internal/deploy"
)

const (
	defaultDomainSuffix = ".ploy"
	defaultIDAlphabet   = "0123456789abcdef"
	defaultIDLength     = 16
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
		hostFlag stringValue
		userFlag stringValue
		identity stringValue
		portFlag intValue
		address  stringValue
		cluster  stringValue
		control  stringValue
		beacon   stringValue
		apiKey   stringValue
		etcd     stringSliceValue
		beacons  stringSliceValue
		workers  stringSliceValue
	)

	fs.Var(&hostFlag, "host", "SSH hostname or IP address")
	fs.Var(&userFlag, "user", "SSH username (default: root)")
	fs.Var(&identity, "identity", "SSH identity file (default: ~/.ssh/id_rsa)")
	fs.Var(&portFlag, "port", "SSH port (default: 22)")
	fs.Var(&address, "address", "Override SSH target address (defaults to host)")
	fs.Var(&cluster, "cluster-id", "Cluster identifier (required)")
	fs.Var(&control, "control-plane-url", "Control plane endpoint recorded in the local descriptor")
	fs.Var(&beacon, "beacon-url", "Beacon URL recorded in the local descriptor (required for execution)")
	fs.Var(&apiKey, "api-key", "API key stored in the local descriptor (required for execution)")
	fs.Var(&etcd, "etcd-endpoints", "Comma-separated etcd endpoints (required for execution)")
	fs.Var(&beacons, "beacon-id", "Initial beacon identifier (repeatable, required for execution)")
	fs.Var(&workers, "worker-id", "Initial worker identifier (repeatable)")
	dryRun := fs.Bool("dry-run", false, "Print bootstrap script without executing")

	if err := fs.Parse(args); err != nil {
		printDeployBootstrapUsage(stderr)
		return err
	}

	if fs.NArg() > 0 {
		printDeployBootstrapUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	if !cluster.set || strings.TrimSpace(cluster.value) == "" {
		printDeployBootstrapUsage(stderr)
		return errors.New("cluster-id is required")
	}

	var opts deploy.Options
	if hostFlag.set {
		opts.Host = strings.TrimSpace(hostFlag.value)
	}
	if userFlag.set {
		opts.User = strings.TrimSpace(userFlag.value)
	}
	if identity.set {
		opts.IdentityFile = expandPath(strings.TrimSpace(identity.value))
	}
	if portFlag.set {
		opts.Port = portFlag.value
	}
	if address.set {
		opts.Address = strings.TrimSpace(address.value)
	}
	opts.ClusterID = strings.TrimSpace(cluster.value)
	opts.BeaconURL = strings.TrimSpace(beacon.value)
	opts.ControlPlaneURL = strings.TrimSpace(control.value)
	opts.APIKey = strings.TrimSpace(apiKey.value)
	opts.InitialBeacons = beacons.Values()
	opts.InitialWorkers = workers.Values()

	var etcdEndpoints []string
	if len(etcd.values) > 0 {
		etcdEndpoints = etcd.Values()
	}

	if strings.TrimSpace(opts.Host) == "" {
		id, err := gonanoid.Generate(defaultIDAlphabet, defaultIDLength)
		if err != nil {
			return fmt.Errorf("generate host identifier: %w", err)
		}
		opts.Host = id + defaultDomainSuffix
	}

	if opts.IdentityFile == "" {
		opts.IdentityFile = defaultIdentityPath()
	} else {
		opts.IdentityFile = expandPath(opts.IdentityFile)
	}

	opts.DryRun = *dryRun
	opts.Stdout = stderr
	opts.Stderr = stderr

	if opts.DryRun {
		opts.EtcdClient = nil
		_, _ = fmt.Fprintln(stderr, "# ploy deploy bootstrap --dry-run (script preview)")
	} else {
		if len(etcdEndpoints) == 0 {
			printDeployBootstrapUsage(stderr)
			return errors.New("etcd-endpoints is required")
		}
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
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := newEtcdClient(ctx, etcdEndpoints)
		if err != nil {
			return err
		}
		defer func() {
			_ = client.Close()
		}()
		opts.EtcdClient = client
	}

	if err := deployBootstrapRunner(context.Background(), opts); err != nil {
		return err
	}

	if opts.DryRun {
		_, _ = fmt.Fprintln(stderr, "# end bootstrap script")
	}
	return nil
}

func printDeployUsage(w io.Writer) {
	printCommandUsage(w, "deploy")
}

func printDeployBootstrapUsage(w io.Writer) {
	printCommandUsage(w, "deploy", "bootstrap")
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

type stringSliceValue struct {
	values []string
}

func (s *stringSliceValue) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			s.values = append(s.values, trimmed)
		}
	}
	return nil
}

func (s *stringSliceValue) String() string {
	return strings.Join(s.values, ",")
}

func (s *stringSliceValue) Values() []string {
	return append([]string(nil), s.values...)
}
