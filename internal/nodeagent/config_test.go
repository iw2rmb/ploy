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
node_id: aB3xY9
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
				if cfg.NodeID != "aB3xY9" {
					t.Errorf("NodeID = %q, want %q", cfg.NodeID, "aB3xY9")
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
node_id: Z9yX3b
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
node_id: aB3xY9
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
node_id: aB3xY9
http:
  tls:
    enabled: true
    key_path: /etc/ploy/node.key
    ca_path: /etc/ploy/ca.crt
`,
			wantErr: true,
		},
		{
			name: "valid gates.build_gate.images",
			yaml: `
server_url: https://server.example.com:8443
node_id: aB3xY9
http:
  tls:
    enabled: false
gates:
  build_gate:
    images:
      - stack:
          language: java
          release: "17"
          tool: maven
        image: maven:3-eclipse-temurin-17
      - stack:
          language: java
          release: "17"
        image: eclipse-temurin:17-jdk
`,
			wantErr: false,
			check: func(t *testing.T, cfg Config) {
				if len(cfg.Gates.BuildGate.Images) != 2 {
					t.Errorf("len(Gates.BuildGate.Images) = %d, want 2", len(cfg.Gates.BuildGate.Images))
					return
				}
				// Verify first rule (tool-specific).
				if cfg.Gates.BuildGate.Images[0].Stack.Language != "java" {
					t.Errorf("Images[0].Stack.Language = %q, want java", cfg.Gates.BuildGate.Images[0].Stack.Language)
				}
				if cfg.Gates.BuildGate.Images[0].Stack.Tool != "maven" {
					t.Errorf("Images[0].Stack.Tool = %q, want maven", cfg.Gates.BuildGate.Images[0].Stack.Tool)
				}
				if cfg.Gates.BuildGate.Images[0].Image != "maven:3-eclipse-temurin-17" {
					t.Errorf("Images[0].Image = %q, want maven:3-eclipse-temurin-17", cfg.Gates.BuildGate.Images[0].Image)
				}
				// Verify second rule (tool-agnostic).
				if cfg.Gates.BuildGate.Images[1].Stack.Tool != "" {
					t.Errorf("Images[1].Stack.Tool = %q, want empty", cfg.Gates.BuildGate.Images[1].Stack.Tool)
				}
			},
		},
		{
			name: "invalid gates.build_gate.images - missing language",
			yaml: `
server_url: https://server.example.com:8443
node_id: aB3xY9
http:
  tls:
    enabled: false
gates:
  build_gate:
    images:
      - stack:
          release: "17"
        image: test:latest
`,
			wantErr: true,
		},
		{
			name: "invalid gates.build_gate.images - duplicate selector",
			yaml: `
server_url: https://server.example.com:8443
node_id: aB3xY9
http:
  tls:
    enabled: false
gates:
  build_gate:
    images:
      - stack:
          language: java
          release: "17"
        image: image1:latest
      - stack:
          language: java
          release: "17"
        image: image2:latest
`,
			wantErr: true,
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
