package deploycli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/deploy"
)

func TestExpandPath(t *testing.T) {
	// Empty is empty
	if ExpandPath("") != "" {
		t.Fatalf("ExpandPath empty not empty")
	}
	// ~ expands to home (best-effort)
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := ExpandPath("~"); got != home {
		t.Fatalf("ExpandPath(~)=%q want %q", got, home)
	}
	// ~/x joins under home
	if got := ExpandPath("~/x"); !strings.HasSuffix(got, filepath.Join(home, "x")) {
		t.Fatalf("ExpandPath(~/x) suffix mismatch: %q", got)
	}
	// absolute untouched
	if got := ExpandPath("/abs"); got != "/abs" {
		t.Fatalf("ExpandPath=/abs -> %q", got)
	}
}

func TestBootstrapCommand_Run_WiresDefaults(t *testing.T) {
	var invoked bool
	cmd := BootstrapCommand{
		RunBootstrap: func(ctx context.Context, opts deploy.Options) error {
			invoked = true
			if opts.Address != "host" {
				t.Fatalf("opts.Address=%q", opts.Address)
			}
			if opts.IdentityFile == "" {
				t.Fatalf("expected non-empty IdentityFile")
			}
			if opts.PloydBinaryPath != "/tmp/ployd" {
				t.Fatalf("opts.PloydBinaryPath=%q", opts.PloydBinaryPath)
			}
			if opts.ControlPlaneURL == "" {
				t.Fatalf("expected default ControlPlaneURL to be set")
			}
			return nil
		},
		LocatePloydBinary: func(os string) (string, error) { return "/tmp/ployd", nil },
		DefaultIdentity:   func() string { return filepath.Join(t.TempDir(), "id_rsa") },
	}
	buf := &bytes.Buffer{}
	err := cmd.Run(context.Background(), BootstrapConfig{
		Address:       "host",
		WorkstationOS: runtime.GOOS,
		Stdout:        buf,
		Stderr:        buf,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !invoked {
		t.Fatalf("RunBootstrap was not invoked")
	}
}

func TestBootstrapCommand_Run_PropagatesRunner(t *testing.T) {
	want := errors.New("runner err")
	cmd := BootstrapCommand{RunBootstrap: func(context.Context, deploy.Options) error { return want }}
	err := cmd.Run(context.Background(), BootstrapConfig{Address: "x", PloydBinaryPath: "/bin/ployd"})
	if !errors.Is(err, want) {
		t.Fatalf("expected runner error propagated, got %v", err)
	}
}

func TestBootstrapCommand_RequiresAddress(t *testing.T) {
	cmd := BootstrapCommand{RunBootstrap: func(context.Context, deploy.Options) error { return nil }, LocatePloydBinary: func(string) (string, error) { return "/tmp/ployd", nil }, DefaultIdentity: func() string { return "/id" }}
	err := cmd.Run(context.Background(), BootstrapConfig{Address: "", PloydBinaryPath: "/tmp/ployd"})
	if err == nil {
		t.Fatalf("expected address required error")
	}
}
