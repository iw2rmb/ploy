package security

import (
	"os"
	"strconv"
	"time"
)

// Config represents the security subsystem configuration (currently NVD-only).
type Config struct {
	NVD NVDConfig `yaml:"nvd" json:"nvd"`
}

// NVDConfig configures the NVD CVE database integration
type NVDConfig struct {
	Enabled bool          `yaml:"enabled" json:"enabled"`
	APIKey  string        `yaml:"api_key" json:"api_key"`
	BaseURL string        `yaml:"base_url" json:"base_url"`
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// DefaultConfig returns the default security configuration.
func DefaultConfig() *Config {
	return &Config{
		NVD: NVDConfig{
			Enabled: true,
			APIKey:  "",
			BaseURL: "https://services.nvd.nist.gov/rest/json/cves/2.0",
			Timeout: 30 * time.Second,
		},
	}
}

// ProductionConfig returns the production security configuration.
func ProductionConfig() *Config {
	cfg := DefaultConfig()
	return cfg
}

// Validate validates the configuration (currently no-op as only NVD settings exist).
func (c *Config) Validate() error {
	return nil
}

// LoadConfigFromEnv loads security configuration from environment variables.
func LoadConfigFromEnv() *Config {
	// Use production config in production environment
	if os.Getenv("PLOY_ENVIRONMENT") == "production" {
		cfg := ProductionConfig()
		applyNVDEnvOverrides(&cfg.NVD)
		return cfg
	}

	cfg := DefaultConfig()
	applyNVDEnvOverrides(&cfg.NVD)
	return cfg
}

func applyNVDEnvOverrides(nvd *NVDConfig) {
	if enabled := os.Getenv("NVD_ENABLED"); enabled != "" {
		switch enabled {
		case "1", "true", "TRUE", "yes", "YES", "on", "ON":
			nvd.Enabled = true
		case "0", "false", "FALSE", "no", "NO", "off", "OFF":
			nvd.Enabled = false
		}
	}
	if apiKey := os.Getenv("NVD_API_KEY"); apiKey != "" {
		nvd.APIKey = apiKey
	}
	if baseURL := os.Getenv("NVD_BASE_URL"); baseURL != "" {
		nvd.BaseURL = baseURL
	}
	if to := os.Getenv("NVD_TIMEOUT_MS"); to != "" {
		if ms, err := strconv.Atoi(to); err == nil && ms > 0 {
			nvd.Timeout = time.Duration(ms) * time.Millisecond
		}
	}
}
