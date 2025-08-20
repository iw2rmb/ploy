package cleanup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ConfigManager manages TTL cleanup configuration
type ConfigManager struct {
	configPath string
	config     *TTLConfig
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(configPath string) *ConfigManager {
	if configPath == "" {
		configPath = "/etc/ploy/cleanup-config.json"
	}
	
	return &ConfigManager{
		configPath: configPath,
		config:     DefaultTTLConfig(),
	}
}

// LoadConfig loads configuration from file or creates default
func (cm *ConfigManager) LoadConfig() (*TTLConfig, error) {
	// Check if config file exists
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		// Create default config file
		if err := cm.SaveConfig(cm.config); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
		return cm.config, nil
	}
	
	// Load existing config
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	var config TTLConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	
	// Validate and set defaults for missing fields
	cm.validateAndSetDefaults(&config)
	
	cm.config = &config
	return &config, nil
}

// SaveConfig saves the current configuration to file
func (cm *ConfigManager) SaveConfig(config *TTLConfig) error {
	// Ensure directory exists
	dir := filepath.Dir(cm.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	if err := os.WriteFile(cm.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	
	return nil
}

// validateAndSetDefaults ensures the config has valid values
func (cm *ConfigManager) validateAndSetDefaults(config *TTLConfig) {
	defaults := DefaultTTLConfig()
	
	if config.PreviewTTL == 0 {
		config.PreviewTTL = defaults.PreviewTTL
	}
	
	if config.CleanupInterval == 0 {
		config.CleanupInterval = defaults.CleanupInterval
	}
	
	if config.NomadAddr == "" {
		config.NomadAddr = defaults.NomadAddr
	}
	
	if config.MaxAge == 0 {
		config.MaxAge = defaults.MaxAge
	}
	
	// Validate minimum values
	if config.PreviewTTL < time.Minute {
		config.PreviewTTL = time.Minute
	}
	
	if config.CleanupInterval < 5*time.Minute {
		config.CleanupInterval = 5 * time.Minute
	}
	
	if config.MaxAge < config.PreviewTTL {
		config.MaxAge = config.PreviewTTL * 2
	}
}

// UpdateConfig updates specific configuration values
func (cm *ConfigManager) UpdateConfig(updates map[string]interface{}) error {
	config := *cm.config // Copy current config
	
	for key, value := range updates {
		switch key {
		case "preview_ttl":
			if ttl, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
				config.PreviewTTL = ttl
			}
		case "cleanup_interval":
			if interval, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
				config.CleanupInterval = interval
			}
		case "max_age":
			if maxAge, err := time.ParseDuration(fmt.Sprintf("%v", value)); err == nil {
				config.MaxAge = maxAge
			}
		case "dry_run":
			if dryRun, ok := value.(bool); ok {
				config.DryRun = dryRun
			}
		case "nomad_addr":
			if addr, ok := value.(string); ok {
				config.NomadAddr = addr
			}
		}
	}
	
	// Validate updated config
	cm.validateAndSetDefaults(&config)
	
	// Save updated config
	if err := cm.SaveConfig(&config); err != nil {
		return fmt.Errorf("failed to save updated config: %w", err)
	}
	
	cm.config = &config
	return nil
}

// GetConfig returns the current configuration
func (cm *ConfigManager) GetConfig() *TTLConfig {
	return cm.config
}

// GetConfigPath returns the path to the configuration file
func (cm *ConfigManager) GetConfigPath() string {
	return cm.configPath
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() *TTLConfig {
	config := DefaultTTLConfig()
	
	// Override with environment variables if present
	if ttl := os.Getenv("PLOY_PREVIEW_TTL"); ttl != "" {
		if duration, err := time.ParseDuration(ttl); err == nil {
			config.PreviewTTL = duration
		}
	}
	
	if interval := os.Getenv("PLOY_CLEANUP_INTERVAL"); interval != "" {
		if duration, err := time.ParseDuration(interval); err == nil {
			config.CleanupInterval = duration
		}
	}
	
	if maxAge := os.Getenv("PLOY_MAX_PREVIEW_AGE"); maxAge != "" {
		if duration, err := time.ParseDuration(maxAge); err == nil {
			config.MaxAge = duration
		}
	}
	
	if dryRun := os.Getenv("PLOY_CLEANUP_DRY_RUN"); dryRun == "true" {
		config.DryRun = true
	}
	
	if nomadAddr := os.Getenv("NOMAD_ADDR"); nomadAddr != "" {
		config.NomadAddr = nomadAddr
	}
	
	return config
}