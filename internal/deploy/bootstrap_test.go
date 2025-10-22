package deploy

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderBootstrapScriptVersions(t *testing.T) {
	script := RenderBootstrapScript()
	for _, fragment := range []string{
		`ETCD_VERSION="3.6.`,
		`IPFS_CLUSTER_VERSION="1.1.4"`,
		`DOCKER_CHANNEL="28"`,
		`GO_VERSION="1.25`,
		"check_package_manager",
		"check_required_ports",
		"check_disk_space",
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected bootstrap script to contain %q", fragment)
		}
	}
}

func TestRunBootstrapDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()
	opts := Options{
		DryRun: true,
		Stdout: &buf,
	}
	if err := RunBootstrap(ctx, opts); err != nil {
		t.Fatalf("RunBootstrap(dry-run) returned error: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "PLOY_BOOTSTRAP_VERSION") {
		t.Fatalf("expected dry-run output to include script metadata, got:\n%s", got)
	}
}

func TestRunBootstrapRequiresHost(t *testing.T) {
	ctx := context.Background()
	opts := Options{}
	if err := RunBootstrap(ctx, opts); err == nil {
		t.Fatalf("expected error when host not provided")
	}
}

func TestRunBootstrapInvokesSSH(t *testing.T) {
	ctx := context.Background()
	var invoked struct {
		Command string
		Args    []string
		Stdin   string
	}
	runner := RunnerFunc(func(_ context.Context, command string, args []string, stdin string, _ IOStreams) error {
		invoked.Command = command
		invoked.Args = append([]string(nil), args...)
		invoked.Stdin = stdin
		return nil
	})

	opts := Options{
		Host:   "bootstrap.example.com",
		User:   "ploy",
		Runner: runner,
	}

	if err := RunBootstrap(ctx, opts); err != nil {
		t.Fatalf("RunBootstrap returned error: %v", err)
	}

	if invoked.Command != "ssh" {
		t.Fatalf("expected ssh command, got %q", invoked.Command)
	}
	if len(invoked.Args) == 0 || invoked.Args[len(invoked.Args)-1] != "bash -s --" {
		t.Fatalf("expected trailing stdin exec argument, got %v", invoked.Args)
	}
	if !strings.Contains(invoked.Stdin, "ETCD_VERSION=\"3.6.") {
		t.Fatalf("expected stdin script to contain ETCD version, got:\n%s", invoked.Stdin)
	}
}

func TestLoadBootstrapConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "bootstrap.yaml")
	configData := `
host: node1.example.com
user: root
port: 2222
identity_file: /home/user/.ssh/id_ed25519
min_disk_gb: 80
required_ports: [2379, 2380, 9094]
`
	if err := os.WriteFile(configPath, []byte(configData), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadBootstrapConfig(configPath)
	if err != nil {
		t.Fatalf("LoadBootstrapConfig returned error: %v", err)
	}
	if cfg.Host != "node1.example.com" {
		t.Fatalf("expected host node1.example.com, got %q", cfg.Host)
	}
	if cfg.User != "root" {
		t.Fatalf("expected user root, got %q", cfg.User)
	}
	if cfg.Port != 2222 {
		t.Fatalf("expected port 2222, got %d", cfg.Port)
	}
	if cfg.IdentityFile != "/home/user/.ssh/id_ed25519" {
		t.Fatalf("expected identity file path, got %q", cfg.IdentityFile)
	}
	if cfg.MinDiskGB != 80 {
		t.Fatalf("expected disk 80 GB, got %d", cfg.MinDiskGB)
	}
	wantPorts := []int{2379, 2380, 9094}
	if len(cfg.RequiredPorts) != len(wantPorts) {
		t.Fatalf("expected %d ports, got %d", len(wantPorts), len(cfg.RequiredPorts))
	}
	for i, port := range wantPorts {
		if cfg.RequiredPorts[i] != port {
			t.Fatalf("expected port %d at index %d, got %d", port, i, cfg.RequiredPorts[i])
		}
	}
}

func TestDocsBootstrapScriptMatchesEmbedded(t *testing.T) {
	docPath := filepath.Join("..", "..", "docs", "v2", "implement.sh")
	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", docPath, err)
	}
	if got := RenderBootstrapScript(); got != string(data) {
		t.Fatalf("embedded script diverges from documentation copy")
	}
}
