package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the simplified CHTTP service configuration
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Security SecurityConfig `yaml:"security"`
	Commands CommandsConfig `yaml:"commands"`
	Logging  LoggingConfig  `yaml:"logging"`
	Health   HealthConfig   `yaml:"health"`
}

// ServerConfig contains basic server configuration
type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// SecurityConfig contains basic security configuration
type SecurityConfig struct {
	APIKey string `yaml:"api_key"`
}

// CommandsConfig contains allowed CLI commands configuration
type CommandsConfig struct {
	Allowed      []string      `yaml:"allowed"`
	DefaultTimeout time.Duration `yaml:"default_timeout"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`  // info, warn, error
	Format string `yaml:"format"` // json, text
}

// HealthConfig contains health check configuration
type HealthConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Endpoint string `yaml:"endpoint"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(configPath string) (*Config, error) {
	// Set defaults
	config := &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Security: SecurityConfig{
			APIKey: "change-me",
		},
		Commands: CommandsConfig{
			Allowed:        []string{"echo", "ls", "cat"},
			DefaultTimeout: 30 * time.Second,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Health: HealthConfig{
			Enabled:  true,
			Endpoint: "/health",
		},
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535")
	}

	if c.Security.APIKey == "" || c.Security.APIKey == "change-me" {
		return fmt.Errorf("security.api_key must be set to a secure value")
	}

	if len(c.Commands.Allowed) == 0 {
		return fmt.Errorf("commands.allowed must contain at least one command")
	}

	validLogLevels := map[string]bool{"info": true, "warn": true, "error": true}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("logging.level must be one of: info, warn, error")
	}

	validLogFormats := map[string]bool{"json": true, "text": true}
	if !validLogFormats[c.Logging.Format] {
		return fmt.Errorf("logging.format must be one of: json, text")
	}

	return nil
}

// IsCommandAllowed checks if a command is in the allowed list
func (c *Config) IsCommandAllowed(command string) bool {
	for _, allowed := range c.Commands.Allowed {
		if allowed == command {
			return true
		}
	}
	return false
}