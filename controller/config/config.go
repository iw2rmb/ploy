package config

import (
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/iw2rmb/ploy/internal/storage"
)

// StorageConfig represents the complete storage configuration
type StorageConfig struct {
	Provider    string                `yaml:"provider"`
	Master      string                `yaml:"master"`
	Filer       string                `yaml:"filer"`
	Collection  string                `yaml:"collection"`
	Replication string                `yaml:"replication"`
	Timeout     int                   `yaml:"timeout"`
	DataCenter  string                `yaml:"datacenter"`
	Rack        string                `yaml:"rack"`
	Collections CollectionConfig      `yaml:"collections"`
	Client      ClientConfig          `yaml:"client"`
}

// CollectionConfig defines collection organization
type CollectionConfig struct {
	Artifacts string `yaml:"artifacts"`
	Metadata  string `yaml:"metadata"`
	Debug     string `yaml:"debug"`
}

// ClientConfig configures the storage client behavior
type ClientConfig struct {
	RetryConfig       RetryConfig       `yaml:"retry_config"`
	HealthCheckConfig HealthCheckConfig `yaml:"health_check_config"`
	EnableMetrics     bool              `yaml:"enable_metrics"`
	EnableHealthCheck bool              `yaml:"enable_health_check"`
	MaxOperationTime  string            `yaml:"max_operation_time"`
}

// RetryConfig configures retry behavior
type RetryConfig struct {
	MaxRetries   int     `yaml:"max_retries"`
	InitialDelay string  `yaml:"initial_delay"`
	MaxDelay     string  `yaml:"max_delay"`
	Multiplier   float64 `yaml:"multiplier"`
}

// HealthCheckConfig configures health checking
type HealthCheckConfig struct {
	Interval         string `yaml:"interval"`
	Timeout          string `yaml:"timeout"`
	FailureThreshold int    `yaml:"failure_threshold"`
}

// Root represents the top-level configuration structure (keeping backward compatibility)
type Root struct {
	Storage StorageConfig `yaml:"storage"`
}

// ConfigManager handles configuration loading and reloading for stateless operation
type ConfigManager struct {
	configPath   string
	lastModTime  time.Time
	mu           sync.RWMutex
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(configPath string) *ConfigManager {
	return &ConfigManager{
		configPath: configPath,
	}
}

// Load loads configuration from the specified path (backward compatible)
func Load(path string) (Root, error) {
	manager := NewConfigManager(path)
	return manager.LoadConfig()
}

// LoadConfig loads the configuration file with validation
func (cm *ConfigManager) LoadConfig() (Root, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Check if file exists
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		return Root{}, fmt.Errorf("configuration file not found: %s", cm.configPath)
	}

	// Read the file
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return Root{}, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var config Root
	if err := yaml.Unmarshal(data, &config); err != nil {
		return Root{}, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Validate the configuration
	if err := cm.validateConfig(&config); err != nil {
		return Root{}, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// validateConfig validates the loaded configuration
func (cm *ConfigManager) validateConfig(config *Root) error {
	storage := &config.Storage

	// Validate provider
	if storage.Provider == "" {
		return fmt.Errorf("storage provider is required")
	}

	// Validate SeaweedFS specific configuration
	if storage.Provider == "seaweedfs" {
		if storage.Master == "" {
			return fmt.Errorf("seaweedfs master address is required")
		}
		if storage.Filer == "" {
			return fmt.Errorf("seaweedfs filer address is required")
		}
		if storage.Collection == "" {
			return fmt.Errorf("seaweedfs collection is required")
		}
	}

	// Validate client configuration if present
	if storage.Client.RetryConfig.MaxRetries < 0 {
		return fmt.Errorf("max_retries must be non-negative")
	}
	if storage.Client.RetryConfig.Multiplier > 0 && storage.Client.RetryConfig.Multiplier <= 0 {
		return fmt.Errorf("retry multiplier must be positive")
	}

	// Validate duration strings if present
	durations := map[string]string{
		"initial_delay":        storage.Client.RetryConfig.InitialDelay,
		"max_delay":           storage.Client.RetryConfig.MaxDelay,
		"health_check_interval": storage.Client.HealthCheckConfig.Interval,
		"health_check_timeout":  storage.Client.HealthCheckConfig.Timeout,
		"max_operation_time":   storage.Client.MaxOperationTime,
	}

	for name, duration := range durations {
		if duration != "" {
			if _, err := time.ParseDuration(duration); err != nil {
				return fmt.Errorf("invalid duration for %s: %w", name, err)
			}
		}
	}

	return nil
}

// ReloadIfChanged checks if the config file has changed and reloads if necessary
func (cm *ConfigManager) ReloadIfChanged() (Root, bool, error) {
	fileInfo, err := os.Stat(cm.configPath)
	if err != nil {
		return Root{}, false, fmt.Errorf("failed to stat config file: %w", err)
	}

	cm.mu.RLock()
	lastModTime := cm.lastModTime
	cm.mu.RUnlock()

	// Check if file has been modified
	if fileInfo.ModTime().After(lastModTime) {
		config, err := cm.LoadConfig()
		if err != nil {
			return Root{}, false, err
		}
		return config, true, nil
	}

	// Return current config without reloading
	config, err := cm.LoadConfig()
	return config, false, err
}

// ToStorageConfig converts the configuration to storage.Config format
func (sc *StorageConfig) ToStorageConfig() storage.Config {
	return storage.Config{
		Master:      sc.Master,
		Filer:       sc.Filer,
		Collection:  sc.Collection,
		Replication: sc.Replication,
		Timeout:     sc.Timeout,
		DataCenter:  sc.DataCenter,
		Rack:        sc.Rack,
		Collections: struct {
			Artifacts string `yaml:"artifacts"`
			Metadata  string `yaml:"metadata"`
			Debug     string `yaml:"debug"`
		}{
			Artifacts: sc.Collections.Artifacts,
			Metadata:  sc.Collections.Metadata,
			Debug:     sc.Collections.Debug,
		},
	}
}

// ToClientConfig converts the configuration to storage.ClientConfig format
func (cc *ClientConfig) ToClientConfig() (*storage.ClientConfig, error) {
	// Use defaults if client config is empty
	if cc.MaxOperationTime == "" {
		return storage.DefaultClientConfig(), nil
	}

	// Parse duration strings
	maxOpTime, err := time.ParseDuration(cc.MaxOperationTime)
	if err != nil {
		return nil, fmt.Errorf("invalid max_operation_time: %w", err)
	}

	initialDelay, err := time.ParseDuration(cc.RetryConfig.InitialDelay)
	if err != nil {
		return nil, fmt.Errorf("invalid initial_delay: %w", err)
	}

	maxDelay, err := time.ParseDuration(cc.RetryConfig.MaxDelay)
	if err != nil {
		return nil, fmt.Errorf("invalid max_delay: %w", err)
	}

	healthInterval, err := time.ParseDuration(cc.HealthCheckConfig.Interval)
	if err != nil {
		return nil, fmt.Errorf("invalid health_check_interval: %w", err)
	}

	healthTimeout, err := time.ParseDuration(cc.HealthCheckConfig.Timeout)
	if err != nil {
		return nil, fmt.Errorf("invalid health_check_timeout: %w", err)
	}

	return &storage.ClientConfig{
		RetryConfig: &storage.RetryConfig{
			MaxAttempts:       cc.RetryConfig.MaxRetries,
			InitialDelay:      initialDelay,
			MaxDelay:          maxDelay,
			BackoffMultiplier: cc.RetryConfig.Multiplier,
		},
		HealthCheckConfig: &storage.HealthCheckConfig{
			CheckInterval:    healthInterval,
			Timeout:          healthTimeout,
			TestBucket:       "health-check",
			TestObjectSize:   1024,
			EnableDeepCheck:  true,
		},
		EnableMetrics:     cc.EnableMetrics,
		EnableHealthCheck: cc.EnableHealthCheck,
		MaxOperationTime:  maxOpTime,
	}, nil
}

// CreateStorageClientFromConfig creates a storage client from configuration (per-request initialization)
func CreateStorageClientFromConfig(configPath string) (*storage.StorageClient, error) {
	// Load configuration fresh for each request (stateless)
	config, err := Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load storage config: %w", err)
	}

	// Convert to storage config format
	storageConfig := config.Storage.ToStorageConfig()

	// Create storage provider
	provider, err := storage.New(storageConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage provider: %w", err)
	}

	// Convert client config
	clientConfig, err := config.Storage.Client.ToClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to convert client config: %w", err)
	}

	// Create enhanced storage client
	return storage.NewStorageClient(provider, clientConfig), nil
}

// GetStorageConfigPath returns the storage configuration path with fallback logic
func GetStorageConfigPath() string {
	// Check environment variable first
	if path := os.Getenv("PLOY_STORAGE_CONFIG"); path != "" {
		return path
	}

	// Check standard external locations (prefer external config)
	externalPaths := []string{
		"/etc/ploy/storage/config.yaml",
		"/etc/ploy/config.yaml",
	}

	for _, path := range externalPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Fallback to embedded config
	return "configs/storage-config.yaml"
}
