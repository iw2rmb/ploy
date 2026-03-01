package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// Ensures that when TLS is enabled, client_ca is required (mTLS mandatory).
func TestLoadConfig_TLSRequiresClientCA(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ployd.yaml")
	raw := `
http:
  listen: :8443
  tls:
    enabled: true
    cert: /etc/ploy/pki/server.crt
    key: /etc/ploy/pki/server.key
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected validation error for missing http.tls.client_ca when TLS enabled")
	}
}
