package sshtransport_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/pkg/sshtransport"
)

type recordingRunner struct {
	name string
	args []string
}

func (r *recordingRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	r.name = name
	r.args = append([]string(nil), args...)
	return nil
}

type fakeFactory struct{}

type fakeHandle struct{}

func (fakeFactory) Activate(ctx context.Context, node sshtransport.Node, localAddr string) (sshtransport.TunnelHandle, error) {
	return fakeHandle{}, nil
}

func (fakeHandle) LocalAddress() string { return "127.0.0.1:0" }

func (fakeHandle) Wait() <-chan error {
	ch := make(chan error)
	close(ch)
	return ch
}

func (fakeHandle) Close() error { return nil }

func (fakeHandle) ControlPath() string { return "/tmp/ctrl" }

func TestCopyToBuildsArgs(t *testing.T) {
	runner := &recordingRunner{}
	manager, err := sshtransport.NewManager(sshtransport.Config{
		Factory:       fakeFactory{},
		CommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	node := sshtransport.Node{
		ID:           "node-a",
		Address:      "host.example",
		User:         "ploy",
		IdentityFile: "/tmp/key",
		SSHPort:      2222,
	}
	if err := manager.SetNodes([]sshtransport.Node{node}); err != nil {
		t.Fatalf("SetNodes: %v", err)
	}
	dir := t.TempDir()
	local := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(local, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	if err := manager.CopyTo(context.Background(), "node-a", sshtransport.CopyToOptions{
		LocalPath:  local,
		RemotePath: "/var/lib/ploy/test.bin",
	}); err != nil {
		t.Fatalf("CopyTo: %v", err)
	}
	if runner.name != "scp" {
		t.Fatalf("expected scp invocation, got %s", runner.name)
	}
	joined := strings.Join(runner.args, " ")
	if !strings.Contains(joined, "-i /tmp/key") {
		t.Fatalf("expected identity flag in args: %s", joined)
	}
	if !strings.Contains(joined, "-P 2222") {
		t.Fatalf("expected ssh port flag in args: %s", joined)
	}
	if !strings.Contains(joined, "ploy@host.example:/var/lib/ploy/test.bin") {
		t.Fatalf("expected remote target in args: %s", joined)
	}
	if !strings.Contains(joined, "ControlPath=/tmp/ctrl") {
		t.Fatalf("expected control path option in args: %s", joined)
	}
}
