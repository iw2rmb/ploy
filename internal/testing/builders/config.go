package builders

import "time"

// Config represents a test configuration
type Config struct {
	Name        string
	Environment string
	StorageType string
	StorageURL  string
	NomadAddr   string
	ConsulAddr  string
	LogLevel    string
	Debug       bool
	Timeout     time.Duration
	MaxRetries  int
	Features    map[string]bool
}

// ConfigBuilder provides a fluent interface for creating test configurations
type ConfigBuilder struct {
	config Config
}

// NewConfig creates a new config builder with defaults
func NewConfig() *ConfigBuilder {
	return &ConfigBuilder{
		config: Config{
			Name:        "test-config",
			Environment: "test",
			StorageType: "memory",
			StorageURL:  "memory://",
			NomadAddr:   "http://localhost:4646",
			ConsulAddr:  "http://localhost:8500",
			LogLevel:    "info",
			Debug:       false,
			Timeout:     30 * time.Second,
			MaxRetries:  3,
			Features:    make(map[string]bool),
		},
	}
}

// WithName sets the configuration name
func (b *ConfigBuilder) WithName(name string) *ConfigBuilder {
	b.config.Name = name
	return b
}

// WithEnvironment sets the environment (test, dev, staging, prod)
func (b *ConfigBuilder) WithEnvironment(env string) *ConfigBuilder {
	b.config.Environment = env
	return b
}

// WithStorage sets the storage configuration
func (b *ConfigBuilder) WithStorage(storageType, url string) *ConfigBuilder {
	b.config.StorageType = storageType
	b.config.StorageURL = url
	return b
}

// WithNomad sets the Nomad address
func (b *ConfigBuilder) WithNomad(addr string) *ConfigBuilder {
	b.config.NomadAddr = addr
	return b
}

// WithConsul sets the Consul address
func (b *ConfigBuilder) WithConsul(addr string) *ConfigBuilder {
	b.config.ConsulAddr = addr
	return b
}

// WithLogLevel sets the log level
func (b *ConfigBuilder) WithLogLevel(level string) *ConfigBuilder {
	b.config.LogLevel = level
	return b
}

// WithDebug enables or disables debug mode
func (b *ConfigBuilder) WithDebug(debug bool) *ConfigBuilder {
	b.config.Debug = debug
	return b
}

// WithTimeout sets the operation timeout
func (b *ConfigBuilder) WithTimeout(timeout time.Duration) *ConfigBuilder {
	b.config.Timeout = timeout
	return b
}

// WithMaxRetries sets the maximum number of retries
func (b *ConfigBuilder) WithMaxRetries(retries int) *ConfigBuilder {
	b.config.MaxRetries = retries
	return b
}

// WithFeature enables or disables a feature flag
func (b *ConfigBuilder) WithFeature(name string, enabled bool) *ConfigBuilder {
	if b.config.Features == nil {
		b.config.Features = make(map[string]bool)
	}
	b.config.Features[name] = enabled
	return b
}

// Build creates the final Config instance
func (b *ConfigBuilder) Build() *Config {
	config := b.config
	if config.Features == nil {
		config.Features = make(map[string]bool)
	}
	return &config
}

// Config presets for common scenarios

// TestConfig creates a configuration for unit tests
func TestConfig() *ConfigBuilder {
	return NewConfig().
		WithEnvironment("test").
		WithStorage("memory", "memory://").
		WithDebug(true).
		WithLogLevel("debug").
		WithTimeout(5 * time.Second)
}

// IntegrationConfig creates a configuration for integration tests
func IntegrationConfig() *ConfigBuilder {
	return NewConfig().
		WithEnvironment("integration").
		WithStorage("seaweedfs", "http://localhost:9333").
		WithNomad("http://localhost:4646").
		WithConsul("http://localhost:8500").
		WithTimeout(60 * time.Second)
}

// ProductionConfig creates a production-like configuration for testing
func ProductionConfig() *ConfigBuilder {
	return NewConfig().
		WithEnvironment("production").
		WithStorage("s3", "s3://prod-bucket").
		WithLogLevel("warn").
		WithDebug(false).
		WithMaxRetries(5).
		WithTimeout(120 * time.Second)
}
