package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the CHTTP service configuration
type Config struct {
	Service    ServiceConfig    `yaml:"service"`
	Executable ExecutableConfig `yaml:"executable"`
	Security   SecurityConfig   `yaml:"security"`
	Input      InputConfig      `yaml:"input"`
	Output     OutputConfig     `yaml:"output"`
}

// ServiceConfig contains service-level configuration
type ServiceConfig struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

// ExecutableConfig contains configuration for the CLI tool to execute
type ExecutableConfig struct {
	Path    string        `yaml:"path"`
	Args    []string      `yaml:"args"`
	Timeout time.Duration `yaml:"timeout"`
}

// SecurityConfig contains security-related configuration
type SecurityConfig struct {
	AuthMethod    string `yaml:"auth_method"`
	PublicKeyPath string `yaml:"public_key_path"`
	RunAsUser     string `yaml:"run_as_user"`
	MaxMemory     string `yaml:"max_memory"`
	MaxCPU        string `yaml:"max_cpu"`
}

// InputConfig contains input validation configuration
type InputConfig struct {
	Formats           []string `yaml:"formats"`
	AllowedExtensions []string `yaml:"allowed_extensions"`
	MaxArchiveSize    string   `yaml:"max_archive_size"`
}

// OutputConfig contains output formatting configuration
type OutputConfig struct {
	Format string `yaml:"format"`
	Parser string `yaml:"parser"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	
	// Validate the configuration
	if err := ValidateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	return &config, nil
}

// ValidateConfig validates the configuration for required fields and valid values
func ValidateConfig(config *Config) error {
	// Validate service configuration
	if config.Service.Name == "" {
		return fmt.Errorf("service name is required")
	}
	
	if config.Service.Port < 1 || config.Service.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	
	// Validate executable configuration
	if config.Executable.Path == "" {
		return fmt.Errorf("executable path is required")
	}
	
	// Validate security configuration
	if config.Security.AuthMethod != "public_key" {
		return fmt.Errorf("auth_method must be 'public_key'")
	}
	
	if config.Security.PublicKeyPath == "" {
		return fmt.Errorf("public_key_path is required")
	}
	
	if config.Security.RunAsUser == "" {
		return fmt.Errorf("run_as_user is required")
	}
	
	// Set default timeout if not specified
	if config.Executable.Timeout == 0 {
		config.Executable.Timeout = 5 * time.Minute
	}
	
	return nil
}

// GetListenAddr returns the address the server should listen on
func (c *Config) GetListenAddr() string {
	return fmt.Sprintf(":%d", c.Service.Port)
}

// GetTimeoutDuration returns the command execution timeout
func (c *Config) GetTimeoutDuration() time.Duration {
	return c.Executable.Timeout
}

// IsValidInputFormat checks if the given format is allowed
func (c *Config) IsValidInputFormat(format string) bool {
	for _, allowed := range c.Input.Formats {
		if allowed == format {
			return true
		}
	}
	return false
}

// IsValidFileExtension checks if the given file extension is allowed
func (c *Config) IsValidFileExtension(ext string) bool {
	for _, allowed := range c.Input.AllowedExtensions {
		if allowed == ext {
			return true
		}
	}
	return false
}