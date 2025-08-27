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
	Path           string        `yaml:"path"`
	Args           []string      `yaml:"args"`
	Timeout        time.Duration `yaml:"timeout"`
	TimeoutSeconds int           `yaml:"-"` // Computed from Timeout
}

// SecurityConfig contains security-related configuration
type SecurityConfig struct {
	AuthMethod         string `yaml:"auth_method"`
	PublicKeyPath      string `yaml:"public_key_path"`
	RunAsUser          string `yaml:"run_as_user"`
	MaxMemory          string `yaml:"max_memory"`
	MaxCPU             string `yaml:"max_cpu"`
	SandboxEnabled     bool   `yaml:"sandbox_enabled"`
	TempDir            string `yaml:"temp_dir"`
	RateLimitPerSecond int    `yaml:"rate_limit_per_sec"`
	RateLimitBurst     int    `yaml:"rate_limit_burst"`
	MaxOpenFiles       int    `yaml:"max_open_files"`
}

// InputConfig contains input validation configuration
type InputConfig struct {
	Formats              []string `yaml:"formats"`
	AllowedExtensions    []string `yaml:"allowed_extensions"`
	MaxArchiveSize       string   `yaml:"max_archive_size"`
	StreamingEnabled     bool     `yaml:"streaming_enabled"`
	BufferSize           int      `yaml:"buffer_size"`           // Size of buffers for streaming (in bytes)
	BufferPoolSize       int      `yaml:"buffer_pool_size"`      // Number of buffers in the pool
	MaxConcurrentStreams int      `yaml:"max_concurrent_streams"` // Max concurrent streaming requests
}

// OutputConfig contains output formatting configuration
type OutputConfig struct {
	Format        string                 `yaml:"format"`
	Parser        string                 `yaml:"parser"`
	CustomParser  *CustomParserConfig    `yaml:"custom_parser,omitempty"`
	ParserOptions map[string]interface{} `yaml:"parser_options,omitempty"`
}

// CustomParserConfig contains custom parser configuration
type CustomParserConfig struct {
	Type     string                   `yaml:"type"`     // "regex", "json", etc.
	Patterns []PatternConfig          `yaml:"patterns"` // For regex parser
}

// PatternConfig defines a regex pattern for custom parsing
type PatternConfig struct {
	Name     string   `yaml:"name"`
	Pattern  string   `yaml:"pattern"`
	Severity string   `yaml:"severity"`
	Groups   []string `yaml:"groups,omitempty"`
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
	if config.Security.AuthMethod != "public_key" && config.Security.AuthMethod != "none" {
		return fmt.Errorf("auth_method must be 'public_key' or 'none' (for testing only)")
	}
	
	if config.Security.AuthMethod == "public_key" && config.Security.PublicKeyPath == "" {
		return fmt.Errorf("public_key_path is required when using public_key auth")
	}
	
	if config.Security.RunAsUser == "" {
		return fmt.Errorf("run_as_user is required")
	}
	
	// Set default timeout if not specified
	if config.Executable.Timeout == 0 {
		config.Executable.Timeout = 5 * time.Minute
	}
	
	// Set default streaming configuration if not specified
	if config.Input.StreamingEnabled {
		if config.Input.BufferSize == 0 {
			config.Input.BufferSize = 32 * 1024 // Default 32KB buffers
		}
		if config.Input.BufferPoolSize == 0 {
			config.Input.BufferPoolSize = 10 // Default pool of 10 buffers
		}
		if config.Input.MaxConcurrentStreams == 0 {
			config.Input.MaxConcurrentStreams = 5 // Default max 5 concurrent streams
		}
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