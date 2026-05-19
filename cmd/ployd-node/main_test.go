package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"os"
	"testing"

	"github.com/iw2rmb/ploy/internal/nodeagent"
)

// withFreshFlags runs fn with a fresh flag.CommandLine and os.Args,
// so tests can invoke run() multiple times safely.
func withFreshFlags(t *testing.T, args []string, fn func()) {
	t.Helper()

	oldCommandLine := flag.CommandLine
	oldArgs := os.Args

	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	os.Args = append([]string{"ployd-node"}, args...)

	defer func() {
		flag.CommandLine = oldCommandLine
		os.Args = oldArgs
	}()

	fn()
}

// stubAgent implements the minimal Run method expected by newAgent.
type stubAgent struct {
	runErr error
}

func (a *stubAgent) Run(ctx context.Context) error {
	return a.runErr
}

func TestRun_VersionFlagExitsZeroAndSkipsAgent(t *testing.T) {
	var stdout, stderr bytes.Buffer
	withDaemonLogWriters(t, &stdout, &stderr)
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	origLoadConfig := loadConfig
	origNewAgent := newAgent
	defer func() {
		loadConfig = origLoadConfig
		newAgent = origNewAgent
	}()

	var loadCalled, newCalled bool
	loadConfig = func(path string) (nodeagent.Config, error) {
		loadCalled = true
		return nodeagent.Config{}, nil
	}
	newAgent = func(cfg nodeagent.Config) (interface{ Run(context.Context) error }, error) {
		newCalled = true
		return &stubAgent{}, nil
	}

	withFreshFlags(t, []string{"-version"}, func() {
		code := run()
		if code != 0 {
			t.Fatalf("run() exit code = %d, want 0", code)
		}
		if loadCalled {
			t.Fatalf("loadConfig should not be called when -version is set")
		}
		if newCalled {
			t.Fatalf("newAgent should not be called when -version is set")
		}
	})

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	frame := decodeDaemonLogFrame(t, stdout.String())
	if frame["level"] != "INFO" || frame["msg"] != "ployd-node" {
		t.Fatalf("version frame = %#v", frame)
	}
}

func TestRun_LoadConfigErrorReturnsNonZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	withDaemonLogWriters(t, &stdout, &stderr)
	origLoadConfig := loadConfig
	origNewAgent := newAgent
	defer func() {
		loadConfig = origLoadConfig
		newAgent = origNewAgent
	}()

	loadConfig = func(path string) (nodeagent.Config, error) {
		return nodeagent.Config{}, errors.New("load failed")
	}
	newAgent = func(cfg nodeagent.Config) (interface{ Run(context.Context) error }, error) {
		t.Fatalf("newAgent should not be called when loadConfig fails")
		return nil, nil
	}

	withFreshFlags(t, nil, func() {
		code := run()
		if code != 1 {
			t.Fatalf("run() exit code = %d, want 1 on load error", code)
		}
	})

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	frame := decodeDaemonLogFrame(t, stderr.String())
	if frame["level"] != "ERROR" || frame["msg"] != "load config" || frame["err"] != "load failed" {
		t.Fatalf("load error frame = %#v", frame)
	}
}

func TestRun_InvalidFlagEmitsDaemonJSONError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	withDaemonLogWriters(t, &stdout, &stderr)
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	withFreshFlags(t, []string{"-unknown"}, func() {
		code := run()
		if code != 2 {
			t.Fatalf("run() exit code = %d, want 2", code)
		}
	})

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	frame := decodeDaemonLogFrame(t, stderr.String())
	if frame["level"] != "ERROR" || frame["msg"] != "parse flags" {
		t.Fatalf("invalid flag frame = %#v", frame)
	}
}

func TestRun_NewAgentErrorReturnsNonZero(t *testing.T) {
	origLoadConfig := loadConfig
	origNewAgent := newAgent
	defer func() {
		loadConfig = origLoadConfig
		newAgent = origNewAgent
	}()

	loadConfig = func(path string) (nodeagent.Config, error) {
		return nodeagent.Config{}, nil
	}
	newAgent = func(cfg nodeagent.Config) (interface{ Run(context.Context) error }, error) {
		return nil, errors.New("construct failed")
	}

	withFreshFlags(t, nil, func() {
		code := run()
		if code != 1 {
			t.Fatalf("run() exit code = %d, want 1 on newAgent error", code)
		}
	})
}

func TestRun_AgentRunCanceledIsSuccess(t *testing.T) {
	origLoadConfig := loadConfig
	origNewAgent := newAgent
	defer func() {
		loadConfig = origLoadConfig
		newAgent = origNewAgent
	}()

	loadConfig = func(path string) (nodeagent.Config, error) {
		return nodeagent.Config{}, nil
	}
	newAgent = func(cfg nodeagent.Config) (interface{ Run(context.Context) error }, error) {
		return &stubAgent{runErr: context.Canceled}, nil
	}

	withFreshFlags(t, nil, func() {
		code := run()
		if code != 0 {
			t.Fatalf("run() exit code = %d, want 0 when agent.Run returns context.Canceled", code)
		}
	})
}

func TestRun_AgentRunErrorReturnsNonZero(t *testing.T) {
	origLoadConfig := loadConfig
	origNewAgent := newAgent
	defer func() {
		loadConfig = origLoadConfig
		newAgent = origNewAgent
	}()

	loadConfig = func(path string) (nodeagent.Config, error) {
		return nodeagent.Config{}, nil
	}
	newAgent = func(cfg nodeagent.Config) (interface{ Run(context.Context) error }, error) {
		return &stubAgent{runErr: errors.New("run failed")}, nil
	}

	withFreshFlags(t, nil, func() {
		code := run()
		if code != 1 {
			t.Fatalf("run() exit code = %d, want 1 when agent.Run returns error", code)
		}
	})
}

func withDaemonLogWriters(t *testing.T, stdout, stderr *bytes.Buffer) {
	t.Helper()
	oldStdout := stdoutWriter
	oldStderr := stderrWriter
	stdoutWriter = stdout
	stderrWriter = stderr
	t.Cleanup(func() {
		stdoutWriter = oldStdout
		stderrWriter = oldStderr
	})
}

func decodeDaemonLogFrame(t *testing.T, line string) map[string]any {
	t.Helper()
	var frame map[string]any
	if err := json.Unmarshal([]byte(line), &frame); err != nil {
		t.Fatalf("decode daemon log frame %q: %v", line, err)
	}
	return frame
}
