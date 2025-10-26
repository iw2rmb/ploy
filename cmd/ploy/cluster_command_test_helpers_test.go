package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const identityBootstrapKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJ076bootTestAdmin deploy-admin"

func ploydFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ployd")
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write temp ployd binary: %v", err)
	}
	return path
}

func identityFixture(t *testing.T, key string) string {
	t.Helper()
	dir := t.TempDir()
	priv := filepath.Join(dir, "id_rsa")
	if err := os.WriteFile(priv, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatalf("write identity private key: %v", err)
	}
	pub := priv + ".pub"
	if err := os.WriteFile(pub, []byte(key+"\n"), 0o644); err != nil {
		t.Fatalf("write identity public key: %v", err)
	}
	return priv
}

func containsAll(s string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(s, needle) {
			return false
		}
	}
	return true
}

type fakeRemoteOps struct {
	commandCalls []remoteCommandCall
	fileCalls    []remoteFileCall
}

type remoteCommandCall struct {
	target  string
	args    []string
	command string
}

type remoteFileCall struct {
	target string
	args   []string
	path   string
	mode   os.FileMode
	data   string
}

func (f *fakeRemoteOps) command(ctx context.Context, target string, sshArgs []string, command string, stdin io.Reader, stdout, stderr io.Writer) error {
	argsCopy := append([]string(nil), sshArgs...)
	f.commandCalls = append(f.commandCalls, remoteCommandCall{
		target:  target,
		args:    argsCopy,
		command: command,
	})
	return nil
}

func (f *fakeRemoteOps) write(ctx context.Context, target string, sshArgs []string, remotePath string, mode os.FileMode, data []byte, stderr io.Writer) error {
	argsCopy := append([]string(nil), sshArgs...)
	dataCopy := string(append([]byte(nil), data...))
	f.fileCalls = append(f.fileCalls, remoteFileCall{
		target: target,
		args:   argsCopy,
		path:   remotePath,
		mode:   mode,
		data:   dataCopy,
	})
	return nil
}

func flagValue(args []string, flag string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
