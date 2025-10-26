package sshtransport

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CopyToOptions contains parameters for uploading a local file to a node via SSH.
type CopyToOptions struct {
	LocalPath  string
	RemotePath string
}

// CopyFromOptions contains parameters for downloading a remote file to the local filesystem.
type CopyFromOptions struct {
	RemotePath string
	LocalPath  string
}

// CopyTo copies a local file to the specified node using scp over the existing SSH transport.
func (m *Manager) CopyTo(ctx context.Context, nodeID string, opts CopyToOptions) error {
	if m == nil {
		return errors.New("sshtransport: manager not configured")
	}
	if strings.TrimSpace(opts.LocalPath) == "" {
		return errors.New("sshtransport: local path required")
	}
	if strings.TrimSpace(opts.RemotePath) == "" {
		return errors.New("sshtransport: remote path required")
	}
	if _, err := os.Stat(opts.LocalPath); err != nil {
		return fmt.Errorf("sshtransport: stat local path: %w", err)
	}
	node, controlPath, err := m.prepareCopy(ctx, nodeID)
	if err != nil {
		return err
	}
	target := buildRemoteTarget(node, opts.RemotePath)
	args := buildScpArgs(node, controlPath)
	args = append(args, opts.LocalPath, target)
	return m.runner.Run(contextOrBackground(ctx), "scp", args, nil, nil, nil)
}

// CopyFrom downloads a remote file from the specified node to the supplied local path.
func (m *Manager) CopyFrom(ctx context.Context, nodeID string, opts CopyFromOptions) error {
	if m == nil {
		return errors.New("sshtransport: manager not configured")
	}
	if strings.TrimSpace(opts.RemotePath) == "" {
		return errors.New("sshtransport: remote path required")
	}
	if strings.TrimSpace(opts.LocalPath) == "" {
		return errors.New("sshtransport: local path required")
	}
	if err := os.MkdirAll(filepath.Dir(opts.LocalPath), 0o755); err != nil {
		return fmt.Errorf("sshtransport: ensure local dir: %w", err)
	}
	node, controlPath, err := m.prepareCopy(ctx, nodeID)
	if err != nil {
		return err
	}
	source := buildRemoteTarget(node, opts.RemotePath)
	args := buildScpArgs(node, controlPath)
	args = append(args, source, opts.LocalPath)
	return m.runner.Run(contextOrBackground(ctx), "scp", args, nil, nil, nil)
}

func (m *Manager) prepareCopy(ctx context.Context, nodeID string) (Node, string, error) {
	if strings.TrimSpace(nodeID) == "" {
		return Node{}, "", errors.New("sshtransport: node id required")
	}
	state, err := m.ensureTunnel(contextOrBackground(ctx), nodeID)
	if err != nil {
		return Node{}, "", err
	}
	controlPath := ""
	if state != nil && state.handle != nil {
		if provider, ok := state.handle.(interface{ ControlPath() string }); ok {
			controlPath = strings.TrimSpace(provider.ControlPath())
		}
	}
	return state.node, controlPath, nil
}

func buildScpArgs(node Node, controlPath string) []string {
	args := []string{"-q"}
	if cp := strings.TrimSpace(controlPath); cp != "" {
		args = append(args, "-o", fmt.Sprintf("ControlPath=%s", cp))
		args = append(args, "-o", "ControlMaster=no", "-o", "ControlPersist=no")
	}
	if strings.TrimSpace(node.IdentityFile) != "" {
		args = append(args, "-i", node.IdentityFile)
	}
	if node.SSHPort > 0 && node.SSHPort != 22 {
		args = append(args, "-P", strconv.Itoa(node.SSHPort))
	}
	return args
}

func buildRemoteTarget(node Node, remotePath string) string {
	user := strings.TrimSpace(node.User)
	if user == "" {
		user = "root"
	}
	host := strings.TrimSpace(node.Address)
	if host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("%s@%s:%s", user, host, remotePath)
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}
