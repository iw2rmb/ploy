package nodeagent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(*testing.T, Config)
	}{
		{
			name: "valid config with defaults",
			yaml: `
server_url: https://server.example.com:8443
node_id: node-001
http:
  listen: ":8444"
  tls:
    enabled: false
`,
			wantErr: false,
			check: func(t *testing.T, cfg Config) {
				if cfg.ServerURL != "https://server.example.com:8443" {
					t.Errorf("ServerURL = %q, want %q", cfg.ServerURL, "https://server.example.com:8443")
				}
				if cfg.NodeID != "node-001" {
					t.Errorf("NodeID = %q, want %q", cfg.NodeID, "node-001")
				}
				// Listen default comes from config file; no assertion on value.
				if cfg.Concurrency != 1 {
					t.Errorf("Concurrency = %d, want %d", cfg.Concurrency, 1)
				}
				if cfg.Heartbeat.Interval != 30*time.Second {
					t.Errorf("Heartbeat.Interval = %v, want %v", cfg.Heartbeat.Interval, 30*time.Second)
				}
			},
		},
		{
			name: "custom values",
			yaml: `
server_url: https://custom.example.com:9443
node_id: custom-node
concurrency: 4
http:
  listen: ":9000"
  read_timeout: 60s
  write_timeout: 60s
  idle_timeout: 180s
  tls:
    enabled: false
heartbeat:
  interval: 60s
  timeout: 20s
`,
			wantErr: false,
			check: func(t *testing.T, cfg Config) {
				if cfg.HTTP.Listen != ":9000" {
					t.Errorf("HTTP.Listen = %q, want %q", cfg.HTTP.Listen, ":9000")
				}
				if cfg.Concurrency != 4 {
					t.Errorf("Concurrency = %d, want %d", cfg.Concurrency, 4)
				}
				// TLS disabled; no cert expectations.
				if cfg.Heartbeat.Interval != 60*time.Second {
					t.Errorf("Heartbeat.Interval = %v, want %v", cfg.Heartbeat.Interval, 60*time.Second)
				}
			},
		},
		{
			name: "missing server_url",
			yaml: `
node_id: node-001
http:
  tls:
    enabled: false
`,
			wantErr: true,
		},
		{
			name: "missing node_id",
			yaml: `
server_url: https://server.example.com:8443
http:
  tls:
    enabled: false
`,
			wantErr: true,
		},
		{
			name: "tls enabled but missing cert_path",
			yaml: `
server_url: https://server.example.com:8443
node_id: node-001
http:
  tls:
    enabled: true
    key_path: /etc/ploy/node.key
    ca_path: /etc/ploy/ca.crt
`,
			wantErr: true,
		},
		{
			name: "buildgate_worker_enabled defaults to false",
			yaml: `
server_url: https://server.example.com:8443
node_id: node-001
http:
  tls:
    enabled: false
`,
			wantErr: false,
			check: func(t *testing.T, cfg Config) {
				// BuildGateWorkerEnabled should default to false when not set.
				if cfg.BuildGateWorkerEnabled {
					t.Error("BuildGateWorkerEnabled = true, want false (default)")
				}
			},
		},
		{
			name: "buildgate_worker_enabled set to true in YAML",
			yaml: `
server_url: https://server.example.com:8443
node_id: node-001
buildgate_worker_enabled: true
http:
  tls:
    enabled: false
`,
			wantErr: false,
			check: func(t *testing.T, cfg Config) {
				if !cfg.BuildGateWorkerEnabled {
					t.Error("BuildGateWorkerEnabled = false, want true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cfgPath := filepath.Join(tmpDir, "config.yaml")

			if err := os.WriteFile(cfgPath, []byte(tt.yaml), 0600); err != nil {
				t.Fatalf("write config file: %v", err)
			}

			cfg, err := LoadConfig(cfgPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

// TestLoadConfig_BuildGateWorkerEnvOverride verifies that the
// PLOY_BUILDGATE_WORKER_ENABLED environment variable takes precedence
// over the YAML config value.
func TestLoadConfig_BuildGateWorkerEnvOverride(t *testing.T) {
	tests := []struct {
		name    string
		yamlVal bool   // value in YAML config
		envVal  string // environment variable value (empty = not set)
		wantVal bool   // expected BuildGateWorkerEnabled
	}{
		{
			name:    "env true overrides yaml false",
			yamlVal: false,
			envVal:  "true",
			wantVal: true,
		},
		{
			name:    "env 1 overrides yaml false",
			yamlVal: false,
			envVal:  "1",
			wantVal: true,
		},
		{
			name:    "env yes overrides yaml false",
			yamlVal: false,
			envVal:  "yes",
			wantVal: true,
		},
		{
			name:    "env TRUE (uppercase) overrides yaml false",
			yamlVal: false,
			envVal:  "TRUE",
			wantVal: true,
		},
		{
			name:    "env false overrides yaml true",
			yamlVal: true,
			envVal:  "false",
			wantVal: false,
		},
		{
			name:    "env empty string preserves yaml true",
			yamlVal: true,
			envVal:  "",
			wantVal: true,
		},
		{
			name:    "env unset preserves yaml true",
			yamlVal: true,
			envVal:  "",
			wantVal: true,
		},
		{
			name:    "env invalid value treated as false",
			yamlVal: true,
			envVal:  "invalid",
			wantVal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build YAML content based on test case.
			yamlContent := `
server_url: https://server.example.com:8443
node_id: node-001
buildgate_worker_enabled: ` + boolToYAML(tt.yamlVal) + `
http:
  tls:
    enabled: false
`
			tmpDir := t.TempDir()
			cfgPath := tmpDir + "/config.yaml"

			if err := os.WriteFile(cfgPath, []byte(yamlContent), 0600); err != nil {
				t.Fatalf("write config file: %v", err)
			}

			// Set or clear the environment variable.
			if tt.envVal != "" {
				t.Setenv("PLOY_BUILDGATE_WORKER_ENABLED", tt.envVal)
			}

			cfg, err := LoadConfig(cfgPath)
			if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}

			if cfg.BuildGateWorkerEnabled != tt.wantVal {
				t.Errorf("BuildGateWorkerEnabled = %v, want %v", cfg.BuildGateWorkerEnabled, tt.wantVal)
			}
		})
	}
}

// boolToYAML converts a bool to a YAML-compatible string.
func boolToYAML(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
