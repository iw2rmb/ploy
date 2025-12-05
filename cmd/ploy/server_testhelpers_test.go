package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/deploy"
)

type mockProvisionRunner struct {
	t                  *testing.T
	copiedBinary       bool
	installedBinary    bool
	ranBootstrapScript bool
	checkedService     bool
}

func (m *mockProvisionRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
	switch name {
	case "scp":
		m.copiedBinary = true
	case "ssh":
		argsStr := strings.Join(args, " ")
		switch {
		case strings.Contains(argsStr, "install -m0755"):
			m.installedBinary = true
		case strings.Contains(argsStr, "systemctl is-active"):
			m.checkedService = true
		case strings.Contains(argsStr, "bash") && strings.Contains(argsStr, "-s"):
			m.ranBootstrapScript = true
		}
	default:
		m.t.Fatalf("unexpected command: %s", name)
	}
	return nil
}

type mockDetectRunner struct {
	t               *testing.T
	existingCluster bool
	detectCalled    *bool
}

func (m *mockDetectRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
	if name == "ssh" {
		argsStr := strings.Join(args, " ")
		if strings.Contains(argsStr, "test -f /etc/ploy/pki/ca.crt") {
			*m.detectCalled = true
			if m.existingCluster {
				return nil
			}
			return errors.New("file not found")
		}
		if strings.Contains(argsStr, "test -f /etc/ploy/ployd.yaml") {
			if m.existingCluster {
				return nil
			}
			return errors.New("file not found")
		}
		if strings.Contains(argsStr, "test -f /etc/ploy/pki/server.crt") {
			if m.existingCluster {
				return nil
			}
			return errors.New("file not found")
		}
		if strings.Contains(argsStr, "openssl x509") && strings.Contains(argsStr, "commonName") {
			if m.existingCluster && streams.Stdout != nil {
				_, _ = streams.Stdout.Write([]byte("ployd-abc123\n"))
				return nil
			}
			return errors.New("cert not found")
		}
	}
	return nil
}

type mockRunner struct {
	runFunc func(ctx context.Context, cmd string, args []string, stdin io.Reader, streams deploy.IOStreams) error
}

func (m *mockRunner) Run(ctx context.Context, cmd string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
	if m.runFunc != nil {
		return m.runFunc(ctx, cmd, args, stdin, streams)
	}
	return nil
}
