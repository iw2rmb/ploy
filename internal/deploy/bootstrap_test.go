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
	if command == "ssh" {
		for i := 0; i+2 < len(args); i++ {
			if args[i] == "bash" && args[i+1] == "-s" && args[i+2] == "--" {
				scriptBody = entry.stdin
				break
			}
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
		Primary:       true,
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
		for i := 0; i+2 < len(c.args); i++ {
			if c.args[i] == "bash" && c.args[i+1] == "-s" && c.args[i+2] == "--" {
				ranScript = true
				break
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
	if scriptBody == "" || !strings.Contains(scriptBody, "PLOY_BOOTSTRAP_VERSION") {
		t.Fatalf("expected bootstrap version export in script: %q", scriptBody)
	}
	var scriptArgs []string
	for _, c := range calls {
		if c.command != "ssh" {
			continue
		}
		for i := 0; i < len(c.args); i++ {
			if c.args[i] == "bash" && i+2 < len(c.args) && c.args[i+1] == "-s" {
				if i+3 <= len(c.args) {
					scriptArgs = append([]string(nil), c.args[i+3:]...)
				}
				break
			}
		}
	}
	if len(scriptArgs) == 0 {
		t.Fatalf("expected script args captured, got none")
	}
	expectPair := func(flag, value string) {
		for i := 0; i < len(scriptArgs)-1; i++ {
			if scriptArgs[i] == flag && scriptArgs[i+1] == value {
				return
			}
		}
		t.Fatalf("expected %s %s in script args %v", flag, value, scriptArgs)
	}
	expectPair("--cluster-id", "cluster-alpha")
	expectPair("--node-id", "control")
	primaryFlag := false
	for _, arg := range scriptArgs {
		if arg == "--primary" {
			primaryFlag = true
			break
		}
	}
	if !primaryFlag {
		t.Fatalf("expected --primary flag in script args %v", scriptArgs)
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
