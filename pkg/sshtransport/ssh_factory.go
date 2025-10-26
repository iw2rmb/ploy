package sshtransport

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type sshFactory struct {
	controlSocketDir string
	localBind        string
	binary           string
	persist          time.Duration
}

func (f *sshFactory) Activate(ctx context.Context, node Node, localAddr string) (TunnelHandle, error) {
	controlPath := f.controlPath(node)
	if err := os.MkdirAll(filepath.Dir(controlPath), 0o755); err != nil {
		return nil, fmt.Errorf("sshtransport: ensure control socket dir: %w", err)
	}

	localHost, localPort, err := net.SplitHostPort(localAddr)
	if err != nil {
		return nil, fmt.Errorf("sshtransport: parse local address %q: %w", localAddr, err)
	}
	if strings.TrimSpace(localHost) == "" {
		localHost = f.localBind
	}

	target := node.Address
	if strings.TrimSpace(node.User) != "" {
		target = fmt.Sprintf("%s@%s", node.User, target)
	}

	bin := f.binary
	if bin == "" {
		bin = "ssh"
	}
	persist := f.persist
	if persist <= 0 {
		persist = 2 * time.Minute
	}

	args := []string{
		"-M",
		"-S", controlPath,
		"-o", fmt.Sprintf("ControlPersist=%s", durationString(persist)),
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-N",
		"-L", fmt.Sprintf("%s:%s:localhost:%d", localHost, localPort, node.APIPort),
	}
	if node.IdentityFile != "" {
		args = append(args, "-i", node.IdentityFile)
	}
	if node.SSHPort > 0 && node.SSHPort != 22 {
		args = append(args, "-p", strconv.Itoa(node.SSHPort))
	}
	args = append(args, target)

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("sshtransport: start ssh tunnel: %w", err)
	}

	handle := &sshHandle{
		cmd:         cmd,
		controlPath: controlPath,
		localAddr:   net.JoinHostPort(localHost, localPort),
		target:      target,
		port:        node.SSHPort,
	}

	if err := waitForTunnel(ctx, handle.localAddr, 3*time.Second); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("sshtransport: tunnel not ready: %w", err)
	}

	go handle.monitor()
	return handle, nil
}

func (f *sshFactory) controlPath(node Node) string {
	slug := sanitize(node.ID)
	if slug == "" {
		slug = "node"
	}
	key := fmt.Sprintf("%s-%d-%s", slug, node.APIPort, strings.TrimSpace(node.Address))
	if len(key) > 40 {
		sum := sha1.Sum([]byte(key))
		key = fmt.Sprintf("%s-%s", slug, hex.EncodeToString(sum[:6]))
	}
	return filepath.Join(f.controlSocketDir, key+".sock")
}

type sshHandle struct {
	cmd         *exec.Cmd
	controlPath string
	localAddr   string
	target      string
	port        int

	once sync.Once
	done chan error
}

func (h *sshHandle) monitor() {
	err := h.cmd.Wait()
	h.finish(err)
	_ = os.Remove(h.controlPath)
}

func (h *sshHandle) finish(err error) {
	h.once.Do(func() {
		if h.done == nil {
			h.done = make(chan error, 1)
		}
		h.done <- err
		close(h.done)
	})
}

func (h *sshHandle) LocalAddress() string {
	return h.localAddr
}

func (h *sshHandle) ControlPath() string {
	return h.controlPath
}

func (h *sshHandle) Wait() <-chan error {
	if h.done == nil {
		h.done = make(chan error, 1)
	}
	return h.done
}

func (h *sshHandle) Close() error {
	h.finish(nil)
	exitArgs := []string{"-S", h.controlPath, h.target, "-O", "exit"}
	if h.port > 0 && h.port != 22 {
		exitArgs = append([]string{"-p", strconv.Itoa(h.port)}, exitArgs...)
	}
	cmd := exec.Command("ssh", exitArgs...)
	_ = cmd.Run()

	waitCh := h.Wait()
	select {
	case <-waitCh:
	case <-time.After(2 * time.Second):
		if h.cmd.Process != nil {
			_ = h.cmd.Process.Kill()
		}
	}
	_ = os.Remove(h.controlPath)
	return nil
}

func waitForTunnel(ctx context.Context, localAddr string, deadline time.Duration) error {
	if deadline <= 0 {
		deadline = 2 * time.Second
	}
	timeout := time.NewTimer(deadline)
	defer timeout.Stop()

	for {
		conn, err := net.DialTimeout("tcp", localAddr, 120*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return err
		case <-time.After(80 * time.Millisecond):
		}
	}
}

func sanitize(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, string(os.PathSeparator), "-")
	value = strings.ReplaceAll(value, " ", "-")
	return value
}

func durationString(d time.Duration) string {
	if d%time.Second == 0 {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return d.String()
}
