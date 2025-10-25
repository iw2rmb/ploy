package deploy

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/config"
)

func TestRunBootstrapRequiresAddress(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		ClusterID: "cluster",
		Runner:    RunnerFunc(func(context.Context, string, []string, io.Reader, IOStreams) error { return nil }),
	}
	opts.PloydBinaryPath = tempPloydBinary(t)

	if err := RunBootstrap(ctx, opts); err == nil || !strings.Contains(err.Error(), "address required") {
		t.Fatalf("expected address required error, got %v", err)
	}
}

func TestRunBootstrapInvokesProvisioningSteps(t *testing.T) {
	ctx := context.Background()
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	t.Setenv("PLOY_CONFIG_HOME", "")

	type call struct {
		command string
		args    []string
		stdin   string
	}
	var calls []call
	var scriptBody string
	runner := RunnerFunc(func(_ context.Context, command string, args []string, stdin io.Reader, _ IOStreams) error {
		entry := call{command: command, args: append([]string(nil), args...)}
		if stdin != nil {
			data, _ := io.ReadAll(stdin)
			entry.stdin = string(data)
		}
		if command == "ssh" && len(args) >= 3 {
			last := args[len(args)-3:]
			if last[0] == "bash" && last[1] == "-s" && last[2] == "--" {
				scriptBody = entry.stdin
			}
		}
		calls = append(calls, entry)
		return nil
	})

	opts := Options{
		Address:       "203.0.113.7",
		User:          "root",
		Runner:        runner,
		Stdout:        io.Discard,
		Stderr:        io.Discard,
		ClusterID:     "cluster-alpha",
		WorkstationOS: "linux",
	}
	opts.PloydBinaryPath = tempPloydBinary(t)

	if err := RunBootstrap(ctx, opts); err != nil {
		t.Fatalf("RunBootstrap returned error: %v", err)
	}

	var copiedBinary, ranScript bool
	for _, c := range calls {
		switch c.command {
		case "scp":
			for _, arg := range c.args {
				if strings.Contains(arg, "/tmp/ployd-") {
					copiedBinary = true
					break
				}
			}
		case "ssh":
			if len(c.args) >= 3 {
				last := c.args[len(c.args)-3:]
				if last[0] == "bash" && last[1] == "-s" && last[2] == "--" {
					ranScript = true
				}
			}
		}
	}
	if !copiedBinary {
		t.Fatalf("expected ployd binary copy via scp; calls=%v", calls)
	}
	if !ranScript {
		t.Fatalf("expected bootstrap script execution; calls=%v", calls)
	}
	if scriptBody == "" || !strings.Contains(scriptBody, "PLOY_CONTROL_PLANE_ENDPOINT") {
		t.Fatalf("expected control plane endpoint export in script: %q", scriptBody)
	}
}

func TestRunBootstrapSavesDescriptorAndSetsDefault(t *testing.T) {
	ctx := context.Background()
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	t.Setenv("PLOY_CONFIG_HOME", "")

	runner := RunnerFunc(func(context.Context, string, []string, io.Reader, IOStreams) error { return nil })
	opts := Options{
		Address:   "203.0.113.10",
		Runner:    runner,
		Stdout:    io.Discard,
		Stderr:    io.Discard,
		ClusterID: "cluster-alpha",
	}
	opts.PloydBinaryPath = tempPloydBinary(t)

	if err := RunBootstrap(ctx, opts); err != nil {
		t.Fatalf("RunBootstrap returned error: %v", err)
	}

	descs, err := config.ListDescriptors()
	if err != nil {
		t.Fatalf("ListDescriptors: %v", err)
	}
	if len(descs) != 1 {
		t.Fatalf("expected 1 descriptor, got %d", len(descs))
	}
	if descs[0].Address != "203.0.113.10" {
		t.Fatalf("expected descriptor address 203.0.113.10, got %s", descs[0].Address)
	}
	if !descs[0].Default {
		t.Fatalf("expected descriptor to be set default")
	}
}

func tempPloydBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ployd")
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write temp ployd binary: %v", err)
	}
	return path
}
