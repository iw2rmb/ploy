package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func normalizeSSHUser(value string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return defaultSSHUser
}

func buildCLISSArgs(identity string, port int) []string {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if trimmed := strings.TrimSpace(identity); trimmed != "" {
		args = append(args, "-i", trimmed)
	}
	if port == 0 {
		port = defaultSSHPort
	}
	if port != defaultSSHPort {
		args = append(args, "-p", strconv.Itoa(port))
	}
	return args
}

func sshTarget(user, host string) string {
	if trimmed := strings.TrimSpace(user); trimmed != "" {
		return fmt.Sprintf("%s@%s", trimmed, strings.TrimSpace(host))
	}
	return strings.TrimSpace(host)
}

func runRemoteCommand(ctx context.Context, target string, sshArgs []string, command string, stdin io.Reader, stdout, stderr io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	args := append(append([]string(nil), sshArgs...), target, command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	if stdout != nil {
		cmd.Stdout = stdout
	}
	if stderr != nil {
		cmd.Stderr = stderr
	}
	return cmd.Run()
}

func writeRemoteFile(ctx context.Context, target string, sshArgs []string, remotePath string, mode os.FileMode, data []byte, stderr io.Writer) error {
	cmd := fmt.Sprintf("install -m%04o /dev/stdin %s", mode, remotePath)
	return runRemoteCommand(ctx, target, sshArgs, cmd, bytes.NewReader(data), nil, stderr)
}

func readRemoteFile(ctx context.Context, target string, sshArgs []string, remotePath string, stderr io.Writer) (string, error) {
	var buf bytes.Buffer
	cmd := fmt.Sprintf("cat %s", shellQuote(remotePath))
	if err := runRemoteCommand(ctx, target, sshArgs, cmd, nil, &buf, stderr); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.Contains(value, "'") {
		return "'" + value + "'"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
