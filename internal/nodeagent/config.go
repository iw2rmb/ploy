package nodeagent

import (
	"errors"
	"fmt"
	"os"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"gopkg.in/yaml.v3"
)

// Config holds node agent configuration.
// Uses domain types (NodeID, ClusterID) for type-safe identification.
type Config struct {
	// HTTP configuration for the node agent API server.
	HTTP HTTPConfig `yaml:"http"`

	// Server URL for the control-plane server.
	ServerURL string `yaml:"server_url"`

	// NodeID identifies this node (NanoID-backed).
	NodeID domaintypes.NodeID `yaml:"node_id"`

	// ClusterID identifies the cluster this node belongs to.
	ClusterID domaintypes.ClusterID `yaml:"cluster_id"`

	// Concurrency defines the maximum number of concurrent runs.
	Concurrency int `yaml:"concurrency"`

	// Heartbeat configuration.
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`

	// Note: Build Gate image selection is configured via:
	//  1) default mapping file, 2) mig YAML overrides.
}

// HTTPConfig specifies HTTP listener and TLS settings for the node agent.
type HTTPConfig struct {
	// Listen address (e.g., ":8444").
	Listen string `yaml:"listen"`

	// TLS configuration.
	TLS TLSConfig `yaml:"tls"`

	// Timeouts.
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

// TLSConfig specifies mTLS certificate and key paths.
type TLSConfig struct {
	// Enabled indicates whether mTLS is enabled.
	Enabled bool `yaml:"enabled"`

	// CertPath is the path to the node certificate.
	CertPath string `yaml:"cert_path"`

	// KeyPath is the path to the node private key.
	KeyPath string `yaml:"key_path"`

	// CAPath is the path to the cluster CA certificate.
	CAPath string `yaml:"ca_path"`

	// BootstrapCAPath is the path to the CA certificate used to verify
	// the server during bootstrap (before mTLS certificates are obtained).
	// If empty, the cluster CA at CAPath is used if it exists; otherwise
	// system roots are used (for public PKI scenarios).
	BootstrapCAPath string `yaml:"bootstrap_ca_path"`
}

// HeartbeatConfig specifies heartbeat interval and timeout.
type HeartbeatConfig struct {
	// Interval between heartbeats.
	Interval time.Duration `yaml:"interval"`

	// Timeout for heartbeat requests.
	Timeout time.Duration `yaml:"timeout"`
}

// LoadConfig reads and parses the YAML configuration file.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}

	// Apply defaults.
	if cfg.HTTP.Listen == "" {
		cfg.HTTP.Listen = ":8444"
	}
	if cfg.HTTP.ReadTimeout == 0 {
		cfg.HTTP.ReadTimeout = 30 * time.Second
	}
	if cfg.HTTP.WriteTimeout == 0 {
		cfg.HTTP.WriteTimeout = 30 * time.Second
	}
	if cfg.HTTP.IdleTimeout == 0 {
		cfg.HTTP.IdleTimeout = 120 * time.Second
	}
	if cfg.Concurrency == 0 {
		cfg.Concurrency = 1
	}
	if cfg.Heartbeat.Interval == 0 {
		cfg.Heartbeat.Interval = 30 * time.Second
	}
	if cfg.Heartbeat.Timeout == 0 {
		cfg.Heartbeat.Timeout = 10 * time.Second
	}

	return cfg, nil
}

func (c Config) validate() error {
	if c.ServerURL == "" {
		return errors.New("server_url is required")
	}
	// Use domain type's IsZero method for validation.
	if c.NodeID.IsZero() {
		return errors.New("node_id is required")
	}
	if c.HTTP.TLS.Enabled {
		if c.ClusterID.IsZero() {
			return errors.New("cluster_id is required when TLS is enabled")
		}
		if c.HTTP.TLS.CertPath == "" {
			return errors.New("http.tls.cert_path is required when TLS is enabled")
		}
		if c.HTTP.TLS.KeyPath == "" {
			return errors.New("http.tls.key_path is required when TLS is enabled")
		}
		if c.HTTP.TLS.CAPath == "" {
			return errors.New("http.tls.ca_path is required when TLS is enabled")
		}
	}
	return nil
}
