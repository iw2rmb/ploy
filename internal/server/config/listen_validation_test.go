package config_test

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

func TestConfig_ListenAndTLSValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name:    "defaults preserved",
			yaml:    "\nlogging:\n  level: info\n",
			wantErr: false,
		},
		{
			name:    "invalid http listen (missing port)",
			yaml:    "\nhttp:\n  listen: 127.0.0.1\n",
			wantErr: true,
		},
		{
			name:    "invalid metrics listen (port out of range)",
			yaml:    "\nmetrics:\n  listen: 127.0.0.1:99999\n",
			wantErr: true,
		},
		{
			name:    "valid ipv6 and ephemeral port",
			yaml:    "\nhttp:\n  listen: \"[::1]:8443\"\nmetrics:\n  listen: 127.0.0.1:0\n",
			wantErr: false,
		},
		{
			name:    "invalid admin listen (missing port)",
			yaml:    "\nadmin:\n  listen: 127.0.0.1\n",
			wantErr: true,
		},
		{
			name: "TLS enabled without client_ca",
			yaml: `
http:
  listen: :8443
  tls:
    enabled: true
    cert: /etc/ploy/pki/server.crt
    key: /etc/ploy/pki/server.key
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Load(writeConfig(t, tt.yaml))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() succeeded, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}
			// For the defaults case, verify values.
			if tt.name == "defaults preserved" {
				if cfg.HTTP.Listen != ":8443" {
					t.Fatalf("HTTP.Listen = %q, want :8443", cfg.HTTP.Listen)
				}
				if cfg.Metrics.Listen != ":9100" {
					t.Fatalf("Metrics.Listen = %q, want :9100", cfg.Metrics.Listen)
				}
			}
		})
	}
}
