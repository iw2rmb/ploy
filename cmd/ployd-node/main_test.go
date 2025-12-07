package main

import (
	"context"
	"errors"
	"flag"
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
}

func TestRun_LoadConfigErrorReturnsNonZero(t *testing.T) {
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
