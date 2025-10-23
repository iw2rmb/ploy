package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/ployd/config"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "ployd.yaml")
	raw := `
mode: worker
control_plane:
  endpoint: https://control.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
pki:
  bundle_dir: /etc/ploy/pki
runtime:
  plugins:
    - name: local
      module: internal
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Mode != config.ModeWorker {
		t.Fatalf("expected Mode=%q, got %q", config.ModeWorker, cfg.Mode)
	}
	if expect := ":8443"; cfg.HTTP.Listen != expect {
		t.Fatalf("HTTP.Listen = %q, want %q", cfg.HTTP.Listen, expect)
	}
	if expect := ":9100"; cfg.Metrics.Listen != expect {
		t.Fatalf("Metrics.Listen = %q, want %q", cfg.Metrics.Listen, expect)
	}
	if cfg.Admin.Socket == "" {
		t.Fatal("Admin.Socket should default to non-empty path")
	}
	if cfg.ControlPlane.HeartbeatInterval != 10*time.Second {
		t.Fatalf("HeartbeatInterval = %v, want 10s", cfg.ControlPlane.HeartbeatInterval)
	}
	if cfg.ControlPlane.AssignmentPollInterval != 5*time.Second {
		t.Fatalf("AssignmentPollInterval = %v, want 5s", cfg.ControlPlane.AssignmentPollInterval)
	}
	if cfg.PKI.RenewBefore != time.Hour {
		t.Fatalf("PKI.RenewBefore = %v, want 1h", cfg.PKI.RenewBefore)
	}
	if cfg.Runtime.DefaultAdapter != "local" {
		t.Fatalf("Runtime.DefaultAdapter = %q, want local", cfg.Runtime.DefaultAdapter)
	}
	if len(cfg.Runtime.Plugins) != 1 {
		t.Fatalf("Runtime.Plugins length = %d, want 1", len(cfg.Runtime.Plugins))
	}
	if cfg.Runtime.Plugins[0].Name != "local" {
		t.Fatalf("Runtime.Plugins[0].Name = %q, want local", cfg.Runtime.Plugins[0].Name)
	}
}

func TestLoadConfigCustomizations(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "ployd.yaml")
	raw := `
mode: beacon
http:
  listen: 127.0.0.1:18443
  tls:
    enabled: true
    cert: /etc/ploy/pki/ployd.pem
    key: /etc/ploy/pki/ployd-key.pem
    client_ca: /etc/ploy/pki/clients.pem
    require_client_cert: true
metrics:
  listen: 127.0.0.1:19100
admin:
  socket: /run/custom-ployd.sock
control_plane:
  endpoint: https://beacon.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/beacon.pem
  key: /etc/ploy/pki/beacon-key.pem
  heartbeat_interval: 2s
  assignment_poll_interval: 3s
pki:
  bundle_dir: /var/lib/ploy/pki
  renew_before: 12m
runtime:
  default_adapter: nomad
  plugins:
    - name: nomad
      module: github.com/example/ployd-nomad
      config:
        address: https://nomad.service.consul
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Mode != config.ModeBeacon {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, config.ModeBeacon)
	}
	if cfg.HTTP.Listen != "127.0.0.1:18443" {
		t.Fatalf("HTTP.Listen = %q", cfg.HTTP.Listen)
	}
	if !cfg.HTTP.TLS.Enabled {
		t.Fatal("HTTP.TLS.Enabled expected true")
	}
	if cfg.HTTP.TLS.CertPath != "/etc/ploy/pki/ployd.pem" {
		t.Fatalf("HTTP.TLS.CertPath = %q", cfg.HTTP.TLS.CertPath)
	}
	if cfg.HTTP.TLS.RequireClientCert != true {
		t.Fatal("HTTP.TLS.RequireClientCert expected true")
	}
	if cfg.Metrics.Listen != "127.0.0.1:19100" {
		t.Fatalf("Metrics.Listen = %q", cfg.Metrics.Listen)
	}
	if cfg.Admin.Socket != "/run/custom-ployd.sock" {
		t.Fatalf("Admin.Socket = %q", cfg.Admin.Socket)
	}
	if cfg.ControlPlane.HeartbeatInterval != 2*time.Second {
		t.Fatalf("HeartbeatInterval = %v", cfg.ControlPlane.HeartbeatInterval)
	}
	if cfg.ControlPlane.AssignmentPollInterval != 3*time.Second {
		t.Fatalf("AssignmentPollInterval = %v", cfg.ControlPlane.AssignmentPollInterval)
	}
	if cfg.PKI.BundleDir != "/var/lib/ploy/pki" {
		t.Fatalf("PKI.BundleDir = %q", cfg.PKI.BundleDir)
	}
	if cfg.PKI.RenewBefore != 12*time.Minute {
		t.Fatalf("PKI.RenewBefore = %v", cfg.PKI.RenewBefore)
	}
	if cfg.Runtime.DefaultAdapter != "nomad" {
		t.Fatalf("Runtime.DefaultAdapter = %q", cfg.Runtime.DefaultAdapter)
	}
	if len(cfg.Runtime.Plugins) != 1 {
		t.Fatalf("Runtime.Plugins length = %d", len(cfg.Runtime.Plugins))
	}
	if cfg.Runtime.Plugins[0].Module != "github.com/example/ployd-nomad" {
		t.Fatalf("Runtime.Plugins[0].Module = %q", cfg.Runtime.Plugins[0].Module)
	}
	if cfg.Runtime.Plugins[0].Config["address"] != "https://nomad.service.consul" {
		t.Fatalf("Runtime.Plugins[0].Config[address] = %v", cfg.Runtime.Plugins[0].Config["address"])
	}
}

func TestLoadConfigValidation(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "ployd.yaml")
	raw := `
mode: worker
http:
  listen: :8443
control_plane:
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := config.Load(path); err == nil {
		t.Fatal("Load() error = nil, want validation failure for missing endpoint")
	}
}
