package config

import (
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/iw2rmb/ploy/internal/storage"
)

// ConfigCacheEntry represents a cached configuration with expiration
type ConfigCacheEntry struct {
	Key       string
	Config    interface{}
	ExpiresAt time.Time
}

// ConfigCache provides simple caching for configuration
type ConfigCache struct {
	entries map[string]*ConfigCacheEntry
	ttl     time.Duration
	mu      sync.RWMutex
	hits    int64
	misses  int64
}

// NewConfigCache creates a new config cache
func NewConfigCache(ttl time.Duration) *ConfigCache {
	return &ConfigCache{
		entries: make(map[string]*ConfigCacheEntry),
		ttl:     ttl,
	}
}

// Get retrieves cached config if still valid
func (c *ConfigCache) Get(key string) (interface{}, bool) {
	if c == nil {
		return nil, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists || time.Now().After(entry.ExpiresAt) {
		c.misses++
		return nil, false
	}

	c.hits++
	return entry.Config, true
}

// Set stores config in cache
func (c *ConfigCache) Set(key string, config interface{}) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &ConfigCacheEntry{
		Key:       key,
		Config:    config,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// GetStats returns cache statistics
func (c *ConfigCache) GetStats() map[string]interface{} {
	if c == nil {
		return map[string]interface{}{
			"size":     0,
			"hits":     int64(0),
			"misses":   int64(0),
			"hit_rate": 0.0,
		}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	var hitRate float64
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	// Count valid entries
	validEntries := 0
	now := time.Now()
	for _, entry := range c.entries {
		if now.Before(entry.ExpiresAt) {
			validEntries++
		}
	}

	return map[string]interface{}{
		"size":     validEntries,
		"hits":     c.hits,
		"misses":   c.misses,
		"hit_rate": hitRate,
	}
}

// Clear clears the cache
func (c *ConfigCache) Clear() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*ConfigCacheEntry)
}

// StorageConfig represents the complete storage configuration
type StorageConfig struct {
	Provider    string           `yaml:"provider"`
	Master      string           `yaml:"master"`
	Filer       string           `yaml:"filer"`
	Collection  string           `yaml:"collection"`
	Replication string           `yaml:"replication"`
	Timeout     int              `yaml:"timeout"`
	DataCenter  string           `yaml:"datacenter"`
	Rack        string           `yaml:"rack"`
	Collections CollectionConfig `yaml:"collections"`
	Client      ClientConfig     `yaml:"client"`
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
	cache        *ConfigCache
	cacheEnabled bool
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(configPath string) *ConfigManager {
	return &ConfigManager{
		configPath:   configPath,
		cache:        NewConfigCache(10 * time.Minute), // 10-minute cache
		cacheEnabled: true,
	}
}

// NewConfigManagerWithCache creates a config manager with configurable caching
func NewConfigManagerWithCache(configPath string, cacheTTL time.Duration, enableCache bool) *ConfigManager {
	var cache *ConfigCache
	if enableCache && cacheTTL > 0 {
		cache = NewConfigCache(cacheTTL)
	}
	return &ConfigManager{
		configPath:   configPath,
		cache:        cache,
		cacheEnabled: enableCache,
	}
}

// Load loads configuration from the specified path (backward compatible)
func Load(path string) (Root, error) {
	manager := NewConfigManager(path)
	return manager.LoadConfig()
}

// LoadConfig loads the configuration file with validation and caching
func (cm *ConfigManager) LoadConfig() (Root, error) {
	// Check cache first if enabled
	if cm.cacheEnabled && cm.cache != nil {
		fileInfo, err := os.Stat(cm.configPath)
		if err == nil {
			cacheKey := fmt.Sprintf("config:%s:%d", cm.configPath, fileInfo.ModTime().Unix())
			if cached, found := cm.cache.Get(cacheKey); found {
				if config, ok := cached.(Root); ok {
					return config, nil
				}
			}
		}
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Check if file exists
	fileInfo, err := os.Stat(cm.configPath)
	if os.IsNotExist(err) {
		return Root{}, fmt.Errorf("configuration file not found: %s", cm.configPath)
	}
	if err != nil {
		return Root{}, fmt.Errorf("failed to stat config file: %w", err)
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

	// Cache the result if caching is enabled
	if cm.cacheEnabled && cm.cache != nil {
		cacheKey := fmt.Sprintf("config:%s:%d", cm.configPath, fileInfo.ModTime().Unix())
		cm.cache.Set(cacheKey, config)
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
		"initial_delay":         storage.Client.RetryConfig.InitialDelay,
		"max_delay":             storage.Client.RetryConfig.MaxDelay,
		"health_check_interval": storage.Client.HealthCheckConfig.Interval,
		"health_check_timeout":  storage.Client.HealthCheckConfig.Timeout,
		"max_operation_time":    storage.Client.MaxOperationTime,
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
			CheckInterval:   healthInterval,
			Timeout:         healthTimeout,
			TestBucket:      "health-check",
			TestObjectSize:  1024,
			EnableDeepCheck: true,
		},
		EnableMetrics:     cc.EnableMetrics,
		EnableHealthCheck: cc.EnableHealthCheck,
		MaxOperationTime:  maxOpTime,
	}, nil
}

// Deprecated helpers removed: storage clients must be created via internal/config.Service

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

// ClearCache clears the configuration cache
func (cm *ConfigManager) ClearCache() {
	if cm.cache != nil {
		cm.cache.Clear()
	}
}

// GetCacheStats returns cache performance statistics
func (cm *ConfigManager) GetCacheStats() map[string]interface{} {
	if cm.cache != nil {
		return cm.cache.GetStats()
	}
	return map[string]interface{}{
		"size":     0,
		"hits":     int64(0),
		"misses":   int64(0),
		"hit_rate": 0.0,
	}
}

// OptimizedStorageClientFactory creates storage clients with connection pooling
type OptimizedStorageClientFactory struct {
	configManager *ConfigManager
	mu            sync.RWMutex
}

// NewOptimizedStorageClientFactory creates a factory for optimized storage clients
func NewOptimizedStorageClientFactory(configPath string) *OptimizedStorageClientFactory {
	return &OptimizedStorageClientFactory{
		configManager: NewConfigManagerWithCache(configPath, 5*time.Minute, true),
	}
}

// CreateClient creates a storage client with optimized configuration loading
func (f *OptimizedStorageClientFactory) CreateClient() (*storage.StorageClient, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Load configuration from cache if possible
	config, err := f.configManager.LoadConfig()
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

// GetStats returns factory statistics
func (f *OptimizedStorageClientFactory) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"config_cache_stats": f.configManager.GetCacheStats(),
	}
}
