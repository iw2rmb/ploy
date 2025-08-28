package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	
	"github.com/iw2rmb/ploy/services/cllm/internal/sandbox"
)

// Config represents the complete CLLM service configuration
type Config struct {
	Server      ServerConfig    `yaml:"server" mapstructure:"server"`
	Sandbox     SandboxConfig   `yaml:"sandbox" mapstructure:"sandbox"`
	Providers   ProvidersConfig `yaml:"providers" mapstructure:"providers"`
	Logging     LoggingConfig   `yaml:"logging" mapstructure:"logging"`
	Security    SecurityConfig  `yaml:"security" mapstructure:"security"`
	Environment string          `yaml:"environment" mapstructure:"environment"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host            string        `yaml:"host" mapstructure:"host"`
	Port            int           `yaml:"port" mapstructure:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout" mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout" mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" mapstructure:"shutdown_timeout"`
}

// SandboxConfig holds sandbox execution configuration
type SandboxConfig struct {
	WorkDir        string `yaml:"work_dir" mapstructure:"work_dir"`
	MaxMemory      string `yaml:"max_memory" mapstructure:"max_memory"`
	MaxCPUTime     string `yaml:"max_cpu_time" mapstructure:"max_cpu_time"`
	MaxProcesses   int    `yaml:"max_processes" mapstructure:"max_processes"`
	CleanupTimeout string `yaml:"cleanup_timeout" mapstructure:"cleanup_timeout"`
}

// ProvidersConfig holds LLM provider configurations
type ProvidersConfig struct {
	Default string       `yaml:"default" mapstructure:"default"`
	Ollama  OllamaConfig `yaml:"ollama" mapstructure:"ollama"`
	OpenAI  OpenAIConfig `yaml:"openai" mapstructure:"openai"`
}

// OllamaConfig holds Ollama provider configuration
type OllamaConfig struct {
	Enabled    bool          `yaml:"enabled" mapstructure:"enabled"`
	BaseURL    string        `yaml:"base_url" mapstructure:"base_url"`
	Model      string        `yaml:"model" mapstructure:"model"`
	Timeout    time.Duration `yaml:"timeout" mapstructure:"timeout"`
	MaxContext int           `yaml:"max_context" mapstructure:"max_context"`
}

// OpenAIConfig holds OpenAI provider configuration
type OpenAIConfig struct {
	Enabled   bool          `yaml:"enabled" mapstructure:"enabled"`
	BaseURL   string        `yaml:"base_url" mapstructure:"base_url"`
	APIKey    string        `yaml:"api_key" mapstructure:"api_key"`
	APIKeyEnv string        `yaml:"api_key_env" mapstructure:"api_key_env"`
	Model     string        `yaml:"model" mapstructure:"model"`
	Timeout   time.Duration `yaml:"timeout" mapstructure:"timeout"`
	MaxTokens int           `yaml:"max_tokens" mapstructure:"max_tokens"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level" mapstructure:"level"`
	Format string `yaml:"format" mapstructure:"format"`
	Output string `yaml:"output" mapstructure:"output"`
}

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
	MaxRequestSize string   `yaml:"max_request_size" mapstructure:"max_request_size"`
	RateLimit      string   `yaml:"rate_limit" mapstructure:"rate_limit"`
	CORSOrigins    []string `yaml:"cors_origins" mapstructure:"cors_origins"`
	APIKeysEnv     string   `yaml:"api_keys_env" mapstructure:"api_keys_env"`
}

// Load loads configuration with defaults, environment variables, and config files
func Load() (*Config, error) {
	config := getDefaultConfig()
	
	// Override with environment variables
	if err := loadFromEnvironment(config); err != nil {
		return nil, fmt.Errorf("failed to load environment variables: %w", err)
	}
	
	// Try to load from default config file locations
	configPaths := []string{
		"./configs/cllm-config.yaml",
		"./cllm-config.yaml",
		"/etc/cllm/config.yaml",
	}
	
	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			fileConfig, err := LoadFromFile(path)
			if err != nil {
				return nil, fmt.Errorf("failed to load config from %s: %w", path, err)
			}
			// Merge file config with current config (file takes precedence)
			mergeConfigs(config, fileConfig)
			break
		}
	}
	
	// Validate final configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}
	
	return config, nil
}

// LoadFromFile loads configuration from a YAML file
func LoadFromFile(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	config := getDefaultConfig()
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	
	return config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Host == "" {
		return fmt.Errorf("server host cannot be empty")
	}
	
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535")
	}
	
	if c.Server.ReadTimeout <= 0 {
		return fmt.Errorf("server read timeout must be positive")
	}
	
	if c.Server.WriteTimeout <= 0 {
		return fmt.Errorf("server write timeout must be positive")
	}
	
	// Validate sandbox memory limit format
	if c.Sandbox.MaxMemory != "" {
		_, err := ParseMemoryLimit(c.Sandbox.MaxMemory)
		if err != nil {
			return fmt.Errorf("invalid sandbox max memory: %w", err)
		}
	}
	
	return nil
}

// ParseMemoryLimit parses memory limit string to bytes
func ParseMemoryLimit(limit string) (int64, error) {
	if limit == "" {
		return 0, nil
	}
	
	// Simple parser for formats like "512MB", "1GB"
	limit = strings.ToUpper(strings.TrimSpace(limit))
	
	multiplier := int64(1)
	if strings.HasSuffix(limit, "KB") {
		multiplier = 1024
		limit = strings.TrimSuffix(limit, "KB")
	} else if strings.HasSuffix(limit, "MB") {
		multiplier = 1024 * 1024
		limit = strings.TrimSuffix(limit, "MB")
	} else if strings.HasSuffix(limit, "GB") {
		multiplier = 1024 * 1024 * 1024
		limit = strings.TrimSuffix(limit, "GB")
	}
	
	value, err := strconv.ParseInt(limit, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory limit format: %s", limit)
	}
	
	return value * multiplier, nil
}

// getDefaultConfig returns the default configuration
func getDefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            8082,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		Sandbox: SandboxConfig{
			WorkDir:        "/tmp/cllm-sandbox",
			MaxMemory:      "1GB",
			MaxCPUTime:     "300s",
			MaxProcesses:   20,
			CleanupTimeout: "30s",
		},
		Providers: ProvidersConfig{
			Default: "ollama",
			Ollama: OllamaConfig{
				Enabled:    true,
				BaseURL:    "http://localhost:11434",
				Model:      "codellama:7b",
				Timeout:    120 * time.Second,
				MaxContext: 4096,
			},
			OpenAI: OpenAIConfig{
				Enabled:   false,
				BaseURL:   "https://api.openai.com",
				APIKeyEnv: "OPENAI_API_KEY",
				Model:     "gpt-4",
				Timeout:   60 * time.Second,
				MaxTokens: 2048,
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		Security: SecurityConfig{
			MaxRequestSize: "50MB",
			RateLimit:      "100/min",
			CORSOrigins:    []string{"*"},
			APIKeysEnv:     "CLLM_API_KEYS",
		},
		Environment: "development",
	}
}

// loadFromEnvironment loads configuration from environment variables
func loadFromEnvironment(config *Config) error {
	if host := os.Getenv("CLLM_SERVER_HOST"); host != "" {
		config.Server.Host = host
	}
	
	if port := os.Getenv("CLLM_SERVER_PORT"); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("invalid CLLM_SERVER_PORT: %w", err)
		}
		config.Server.Port = p
	}
	
	if timeout := os.Getenv("CLLM_SERVER_READ_TIMEOUT"); timeout != "" {
		t, err := time.ParseDuration(timeout)
		if err != nil {
			return fmt.Errorf("invalid CLLM_SERVER_READ_TIMEOUT: %w", err)
		}
		config.Server.ReadTimeout = t
	}
	
	if timeout := os.Getenv("CLLM_SERVER_WRITE_TIMEOUT"); timeout != "" {
		t, err := time.ParseDuration(timeout)
		if err != nil {
			return fmt.Errorf("invalid CLLM_SERVER_WRITE_TIMEOUT: %w", err)
		}
		config.Server.WriteTimeout = t
	}
	
	// Load sandbox configuration from environment
	if workDir := os.Getenv("CLLM_SANDBOX_WORK_DIR"); workDir != "" {
		config.Sandbox.WorkDir = workDir
	}
	
	if maxMemory := os.Getenv("CLLM_SANDBOX_MAX_MEMORY"); maxMemory != "" {
		config.Sandbox.MaxMemory = maxMemory
	}
	
	// Load provider configuration from environment
	if ollamaURL := os.Getenv("CLLM_OLLAMA_URL"); ollamaURL != "" {
		config.Providers.Ollama.BaseURL = ollamaURL
	}
	
	if ollamaModel := os.Getenv("CLLM_OLLAMA_MODEL"); ollamaModel != "" {
		config.Providers.Ollama.Model = ollamaModel
	}
	
	return nil
}

// ToSandboxManagerConfig converts SandboxConfig to sandbox manager config
func (s *SandboxConfig) ToManagerConfig() sandbox.ManagerConfig {
	return sandbox.ManagerConfig{
		WorkDir:        s.WorkDir,
		MaxMemory:      s.MaxMemory,
		MaxCPUTime:     s.MaxCPUTime,
		MaxProcesses:   s.MaxProcesses,
		CleanupTimeout: s.CleanupTimeout,
	}
}

// mergeConfigs merges source config into target config
func mergeConfigs(target, source *Config) {
	if source.Server.Host != "" {
		target.Server.Host = source.Server.Host
	}
	if source.Server.Port != 0 {
		target.Server.Port = source.Server.Port
	}
	if source.Server.ReadTimeout != 0 {
		target.Server.ReadTimeout = source.Server.ReadTimeout
	}
	if source.Server.WriteTimeout != 0 {
		target.Server.WriteTimeout = source.Server.WriteTimeout
	}
	if source.Server.ShutdownTimeout != 0 {
		target.Server.ShutdownTimeout = source.Server.ShutdownTimeout
	}
	
	if source.Sandbox.WorkDir != "" {
		target.Sandbox.WorkDir = source.Sandbox.WorkDir
	}
	if source.Sandbox.MaxMemory != "" {
		target.Sandbox.MaxMemory = source.Sandbox.MaxMemory
	}
	if source.Sandbox.MaxCPUTime != "" {
		target.Sandbox.MaxCPUTime = source.Sandbox.MaxCPUTime
	}
	if source.Sandbox.MaxProcesses != 0 {
		target.Sandbox.MaxProcesses = source.Sandbox.MaxProcesses
	}
	if source.Sandbox.CleanupTimeout != "" {
		target.Sandbox.CleanupTimeout = source.Sandbox.CleanupTimeout
	}
}