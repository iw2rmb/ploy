package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/iw2rmb/ploy/internal/deploy"
)

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
		configPath stringValue
		hostFlag   stringValue
		userFlag   stringValue
		identity   stringValue
		portFlag   intValue
		minDisk    intValue
		portList   multiIntValue
	)

	fs.Var(&configPath, "config", "Path to bootstrap configuration (YAML)")
	fs.Var(&hostFlag, "host", "SSH hostname or IP address")
	fs.Var(&userFlag, "user", "SSH username (default: root)")
	fs.Var(&identity, "identity", "SSH identity file (optional)")
	fs.Var(&portFlag, "port", "SSH port (default: 22)")
	fs.Var(&minDisk, "min-disk", "Minimum free disk in GiB (default aligns with docs)")
	fs.Var(&portList, "require-port", "Additional port to verify availability (repeatable)")
	dryRun := fs.Bool("dry-run", false, "Print bootstrap script without executing")

	if err := fs.Parse(args); err != nil {
		printDeployBootstrapUsage(stderr)
		return err
	}

	if fs.NArg() > 0 {
		printDeployBootstrapUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	var opts deploy.Options
	if configPath.set {
		cfg, err := deploy.LoadBootstrapConfig(strings.TrimSpace(configPath.value))
		if err != nil {
			return err
		}
		opts = cfg.ToOptions()
	}

	if hostFlag.set {
		opts.Host = strings.TrimSpace(hostFlag.value)
	}
	if userFlag.set {
		opts.User = strings.TrimSpace(userFlag.value)
	}
	if identity.set {
		opts.IdentityFile = strings.TrimSpace(identity.value)
	}
	if portFlag.set {
		opts.Port = portFlag.value
	}
	if minDisk.set {
		opts.MinDiskGB = minDisk.value
	}
	if len(portList.values) > 0 {
		opts.RequiredPorts = append([]int(nil), portList.values...)
	}

	opts.DryRun = *dryRun
	opts.Stdout = stderr
	opts.Stderr = stderr

	if !opts.DryRun && strings.TrimSpace(opts.Host) == "" {
		printDeployBootstrapUsage(stderr)
		return errors.New("host required (use --host or specify in --config)")
	}

	if opts.DryRun {
		_, _ = fmt.Fprintln(stderr, "# ploy deploy bootstrap --dry-run (script preview)")
	}

	if err := deploy.RunBootstrap(context.Background(), opts); err != nil {
		return err
	}

	if opts.DryRun {
		_, _ = fmt.Fprintln(stderr, "# end bootstrap script")
	} else {
		_, _ = fmt.Fprintf(stderr, "Bootstrap completed for %s.\n", opts.Host)
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

type multiIntValue struct {
	values []int
}

func (m *multiIntValue) Set(value string) error {
	for _, segment := range strings.Split(value, ",") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		v, err := strconv.Atoi(segment)
		if err != nil {
			return fmt.Errorf("parse port %q: %w", segment, err)
		}
		if v <= 0 || v > 65535 {
			return fmt.Errorf("port %d out of range", v)
		}
		m.values = append(m.values, v)
	}
	return nil
}

func (m *multiIntValue) String() string {
	if len(m.values) == 0 {
		return ""
	}
	parts := make([]string, len(m.values))
	for i, v := range m.values {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ",")
}
