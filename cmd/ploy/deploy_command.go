package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	deploycli "github.com/iw2rmb/ploy/internal/cli/deploy"
	"github.com/iw2rmb/ploy/internal/deploy"
)

var deployBootstrapRunner = deploy.RunBootstrap

// handleDeploy routes deploy subcommands.
func handleDeploy(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printDeployUsage(stderr)
		return errors.New("deploy requires target address")
	}

	if !strings.HasPrefix(args[0], "-") && args[0] != "" {
		switch args[0] {
		case "bootstrap":
			return handleDeployBootstrap(args[1:], stderr)
		default:
			printDeployUsage(stderr)
			return fmt.Errorf("unknown deploy subcommand %q", args[0])
		}
	}

	return handleDeployBootstrap(args, stderr)
}

func handleDeployBootstrap(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("deploy bootstrap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		userFlag stringValue
		identity stringValue
		address  stringValue
		control  stringValue
		ploydBin stringValue
	)

	fs.Var(&userFlag, "user", "SSH username (default: root)")
	fs.Var(&identity, "identity", "SSH identity file (default: ~/.ssh/id_rsa)")
	fs.Var(&address, "address", "Override SSH target address (defaults to host)")
	fs.Var(&control, "control-plane-url", "Control plane endpoint recorded in the local descriptor")
	fs.Var(&ploydBin, "ployd-binary", "Path to the ployd binary uploaded during bootstrap (default: alongside the CLI)")

	if err := fs.Parse(args); err != nil {
		printDeployBootstrapUsage(stderr)
		return err
	}

	if fs.NArg() > 0 {
		printDeployBootstrapUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	cfg := deploycli.BootstrapConfig{
		Stdout:        stderr,
		Stderr:        stderr,
		Stdin:         os.Stdin,
		WorkstationOS: runtime.GOOS,
	}
	if userFlag.set {
		cfg.User = strings.TrimSpace(userFlag.value)
	}
	if identity.set {
		cfg.IdentityFile = strings.TrimSpace(identity.value)
	}
	if address.set {
		cfg.Address = strings.TrimSpace(address.value)
	}
	cfg.ControlPlaneURL = strings.TrimSpace(control.value)
	if ploydBin.set {
		cfg.PloydBinaryPath = strings.TrimSpace(ploydBin.value)
	}

	cmd := deploycli.BootstrapCommand{
		RunBootstrap: deployBootstrapRunner,
	}
	if err := cmd.Run(context.Background(), cfg); err != nil {
		if errors.Is(err, deploycli.ErrBeaconURLRequired) || errors.Is(err, deploycli.ErrAPIKeyRequired) || errors.Is(err, deploycli.ErrInitialBeaconIDMissing) {
			printDeployBootstrapUsage(stderr)
		}
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
