package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

func TestConfig_ListenAddressValidation(t *testing.T) {
	t.Helper()

	t.Run("defaults_preserved", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ployd.yaml")
		raw := "\nlogging:\n  level: info\n"
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		cfg, err := config.Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.HTTP.Listen != ":8443" {
			t.Fatalf("HTTP.Listen = %q, want :8443", cfg.HTTP.Listen)
		}
		if cfg.Metrics.Listen != ":9100" {
			t.Fatalf("Metrics.Listen = %q, want :9100", cfg.Metrics.Listen)
		}
	})

	t.Run("invalid_http_listen", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ployd.yaml")
		raw := "\nhttp:\n  listen: 127.0.0.1\n"
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if _, err := config.Load(path); err == nil {
			t.Fatal("Load() succeeded, want error for invalid http.listen")
		}
	})

	t.Run("invalid_metrics_listen", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ployd.yaml")
		raw := "\nmetrics:\n  listen: 127.0.0.1:99999\n"
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if _, err := config.Load(path); err == nil {
			t.Fatal("Load() succeeded, want error for invalid metrics.listen")
		}
	})

	t.Run("valid_ipv6_and_ephemeral", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ployd.yaml")
		raw := "\nhttp:\n  listen: \"[::1]:8443\"\nmetrics:\n  listen: 127.0.0.1:0\n"
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if _, err := config.Load(path); err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}
	})

	t.Run("invalid_admin_listen", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ployd.yaml")
		raw := "\nadmin:\n  listen: 127.0.0.1\n"
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if _, err := config.Load(path); err == nil {
			t.Fatal("Load() succeeded, want error for invalid admin.listen")
		}
	})
}
