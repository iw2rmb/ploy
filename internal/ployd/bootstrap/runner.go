package bootstrap

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/iw2rmb/ploy/internal/bootstrap"
	"github.com/iw2rmb/ploy/internal/ployd/config"
)

// CommandFunc constructs an exec.Cmd for the provided command invocation.
type CommandFunc func(ctx context.Context, name string, args ...string) *exec.Cmd

// Options configure the script runner.
type Options struct {
	Shell   string
	Stdout  io.Writer
	Stderr  io.Writer
	Command CommandFunc
}

// Runner executes the embedded bootstrap shell script when ployd starts in bootstrap mode.
type Runner struct {
	shell   string
	stdout  io.Writer
	stderr  io.Writer
	command CommandFunc
}

// NewRunner constructs a Runner using the supplied options.
func NewRunner(opts Options) *Runner {
	cmd := opts.Command
	if cmd == nil {
		cmd = exec.CommandContext
	}
	return &Runner{
		shell:   strings.TrimSpace(opts.Shell),
		stdout:  opts.Stdout,
		stderr:  opts.Stderr,
		command: cmd,
	}
}

// Run implements daemon.BootstrapRunner by piping the embedded shell script into the configured shell.
func (r *Runner) Run(ctx context.Context, cfg config.Config) error {
	shell := r.shell
	if shell == "" {
		shell = "bash"
	}

	cmd := r.command(ctx, shell, "-s", "--")
	if cmd == nil {
		return fmt.Errorf("bootstrap: command factory returned nil")
	}

	exports := bootstrap.DefaultExports()
	if listen := strings.TrimSpace(cfg.HTTP.Listen); listen != "" {
		exports["PLOYD_HTTP_LISTEN"] = listen
	}
	if listen := strings.TrimSpace(cfg.Metrics.Listen); listen != "" {
		exports["PLOYD_METRICS_LISTEN"] = listen
	}
	if endpoint := strings.TrimSpace(cfg.ControlPlane.Endpoint); endpoint != "" {
		exports["PLOY_CONTROL_PLANE_ENDPOINT"] = endpoint
	}

	script := bootstrap.PrefixedScript(exports)

	cmd.Stdin = strings.NewReader(script)
	if r.stdout != nil {
		cmd.Stdout = r.stdout
	} else {
		cmd.Stdout = os.Stdout
	}
	if r.stderr != nil {
		cmd.Stderr = r.stderr
	} else {
		cmd.Stderr = os.Stderr
	}
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bootstrap: execute script: %w", err)
	}
	return nil
}
