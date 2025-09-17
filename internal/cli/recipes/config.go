package recipes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// CLIConfig represents CLI configuration for ARF recipe management
type CLIConfig struct {
	// Recipe settings
	Recipes RecipeConfig `yaml:"recipes" json:"recipes"`

	// Storage settings
	Storage StorageConfig `yaml:"storage" json:"storage"`

	// Execution settings
	Execution ExecutionSettings `yaml:"execution" json:"execution"`

	// Display settings
	Display DisplayConfig `yaml:"display" json:"display"`

	// Template settings
	Templates TemplateConfig `yaml:"templates" json:"templates"`
}

// RecipeConfig contains recipe-specific settings
type RecipeConfig struct {
	DefaultAuthor    string   `yaml:"default_author" json:"default_author"`
	DefaultLicense   string   `yaml:"default_license" json:"default_license"`
	DefaultLanguages []string `yaml:"default_languages" json:"default_languages"`
	AutoValidate     bool     `yaml:"auto_validate" json:"auto_validate"`
	PreferredFormat  string   `yaml:"preferred_format" json:"preferred_format"`
}

// StorageConfig contains storage-related settings
type StorageConfig struct {
	CacheRecipes   bool          `yaml:"cache_recipes" json:"cache_recipes"`
	CacheDuration  time.Duration `yaml:"cache_duration" json:"cache_duration"`
	MaxCacheSize   string        `yaml:"max_cache_size" json:"max_cache_size"`
	BackupLocation string        `yaml:"backup_location" json:"backup_location"`
}

// ExecutionSettings contains execution-related settings
type ExecutionSettings struct {
	DefaultTimeout     time.Duration     `yaml:"default_timeout" json:"default_timeout"`
	AutoBackup         bool              `yaml:"auto_backup" json:"auto_backup"`
	BackupLocation     string            `yaml:"backup_location" json:"backup_location"`
	DefaultEnvironment map[string]string `yaml:"default_environment" json:"default_environment"`
	MaxConcurrentRuns  int               `yaml:"max_concurrent_runs" json:"max_concurrent_runs"`
	RetryAttempts      int               `yaml:"retry_attempts" json:"retry_attempts"`
	RetryDelay         time.Duration     `yaml:"retry_delay" json:"retry_delay"`
}

// DisplayConfig contains display and output settings
type DisplayConfig struct {
	DefaultOutputFormat string `yaml:"default_output_format" json:"default_output_format"`
	DefaultPageSize     int    `yaml:"default_page_size" json:"default_page_size"`
	ShowProgressBars    bool   `yaml:"show_progress_bars" json:"show_progress_bars"`
	ColorOutput         bool   `yaml:"color_output" json:"color_output"`
	VerboseByDefault    bool   `yaml:"verbose_by_default" json:"verbose_by_default"`
	ShowTimestamps      bool   `yaml:"show_timestamps" json:"show_timestamps"`
}

// TemplateConfig contains template-related settings
type TemplateConfig struct {
	DefaultTemplate    string            `yaml:"default_template" json:"default_template"`
	CustomTemplatePath string            `yaml:"custom_template_path" json:"custom_template_path"`
	TemplateVariables  map[string]string `yaml:"template_variables" json:"template_variables"`
	AutoloadCustom     bool              `yaml:"autoload_custom" json:"autoload_custom"`
}

var (
	globalConfig *CLIConfig
	configPath   string
)

// GetConfigPath returns the path to the CLI configuration file
func GetConfigPath() string {
	if configPath != "" {
		return configPath
	}

	// Check environment variable first
	if path := os.Getenv("PLOY_CONFIG_FILE"); path != "" {
		configPath = path
		return configPath
	}

	// Default to ~/.ploy/config.yaml
	homeDir, err := os.UserHomeDir()
	if err != nil {
		configPath = "./ploy-config.yaml" // Fallback to current directory
		return configPath
	}

	configPath = filepath.Join(homeDir, ".ploy", "config.yaml")
	return configPath
}

// LoadConfig loads the CLI configuration from file
func LoadConfig() (*CLIConfig, error) {
	if globalConfig != nil {
		return globalConfig, nil
	}

	configFile := GetConfigPath()

	// Create default config if file doesn't exist
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		defaultConfig := GetDefaultConfig()
		if err := SaveConfig(defaultConfig); err != nil {
			// If we can't save, return default config without persisting
			PrintWarning(fmt.Sprintf("Could not save default config: %v", err))
			globalConfig = defaultConfig
			return globalConfig, nil
		}
		globalConfig = defaultConfig
		return globalConfig, nil
	}

	// Load existing config
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, NewCLIError(fmt.Sprintf("Failed to read config file: %s", configFile), 1).WithCause(err)
	}

	config := GetDefaultConfig() // Start with defaults
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, NewCLIError("Invalid configuration file format", 1).
			WithCause(err).
			WithSuggestion("Check YAML syntax or delete the file to regenerate defaults")
	}

	globalConfig = config
	return globalConfig, nil
}

// SaveConfig saves the configuration to file
func SaveConfig(config *CLIConfig) error {
	configFile := GetConfigPath()

	// Ensure config directory exists
	configDir := filepath.Dir(configFile)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return NewCLIError(fmt.Sprintf("Failed to create config directory: %s", configDir), 1).WithCause(err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return NewCLIError("Failed to serialize configuration", 1).WithCause(err)
	}

	// Write to file
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return NewCLIError(fmt.Sprintf("Failed to write config file: %s", configFile), 1).WithCause(err)
	}

	globalConfig = config
	return nil
}

// GetDefaultConfig returns the default CLI configuration
func GetDefaultConfig() *CLIConfig {
	homeDir, _ := os.UserHomeDir()

	return &CLIConfig{
		Recipes: RecipeConfig{
			DefaultAuthor:    "ploy-user",
			DefaultLicense:   "MIT",
			DefaultLanguages: []string{},
			AutoValidate:     true,
			PreferredFormat:  "yaml",
		},
		Storage: StorageConfig{
			CacheRecipes:   true,
			CacheDuration:  1 * time.Hour,
			MaxCacheSize:   "100MB",
			BackupLocation: filepath.Join(homeDir, ".ploy", "recipe-backups"),
		},
		Execution: ExecutionSettings{
			DefaultTimeout:     15 * time.Minute,
			AutoBackup:         true,
			BackupLocation:     filepath.Join(homeDir, ".ploy", "execution-backups"),
			DefaultEnvironment: make(map[string]string),
			MaxConcurrentRuns:  3,
			RetryAttempts:      2,
			RetryDelay:         5 * time.Second,
		},
		Display: DisplayConfig{
			DefaultOutputFormat: "table",
			DefaultPageSize:     20,
			ShowProgressBars:    true,
			ColorOutput:         true,
			VerboseByDefault:    false,
			ShowTimestamps:      false,
		},
		Templates: TemplateConfig{
			DefaultTemplate:    "openrewrite",
			CustomTemplatePath: filepath.Join(homeDir, ".ploy", "templates"),
			TemplateVariables:  make(map[string]string),
			AutoloadCustom:     true,
		},
	}
}

// GetConfig returns the current configuration, loading it if necessary
func GetConfig() (*CLIConfig, error) {
	if globalConfig == nil {
		return LoadConfig()
	}
	return globalConfig, nil
}

// ValidateConfig validates the configuration values
func ValidateConfig(config *CLIConfig) error {
	// Validate output format
	validFormats := []string{"table", "json", "yaml"}
	validFormat := false
	for _, format := range validFormats {
		if config.Display.DefaultOutputFormat == format {
			validFormat = true
			break
		}
	}
	if !validFormat {
		return NewCLIError(fmt.Sprintf("Invalid default output format: %s", config.Display.DefaultOutputFormat), 1).
			WithSuggestion(fmt.Sprintf("Valid formats: %s", strings.Join(validFormats, ", ")))
	}

	// Validate page size
	if config.Display.DefaultPageSize <= 0 {
		return NewCLIError("Default page size must be positive", 1)
	}
	if config.Display.DefaultPageSize > 1000 {
		return NewCLIError("Default page size cannot exceed 1000", 1)
	}

	// Validate timeout
	if config.Execution.DefaultTimeout <= 0 {
		return NewCLIError("Default timeout must be positive", 1)
	}

	// Validate concurrent runs
	if config.Execution.MaxConcurrentRuns <= 0 {
		config.Execution.MaxConcurrentRuns = 1
	}
	if config.Execution.MaxConcurrentRuns > 10 {
		return NewCLIError("Maximum concurrent runs cannot exceed 10", 1).
			WithSuggestion("Use a smaller value to avoid resource exhaustion")
	}

	// Validate retry settings
	if config.Execution.RetryAttempts < 0 {
		config.Execution.RetryAttempts = 0
	}
	if config.Execution.RetryDelay < 0 {
		config.Execution.RetryDelay = 1 * time.Second
	}

	return nil
}

// ShowConfig displays the current configuration
func ShowConfig(outputFormat string) error {
	config, err := GetConfig()
	if err != nil {
		return err
	}

	switch strings.ToLower(outputFormat) {
	case "json":
		data, _ := json.MarshalIndent(config, "", "  ")
		fmt.Println(string(data))
	case "yaml":
		data, _ := yaml.Marshal(config)
		fmt.Println(string(data))
	default: // table
		fmt.Printf("Ploy ARF Configuration\n")
		fmt.Printf("======================\n\n")

		fmt.Printf("Configuration file: %s\n\n", GetConfigPath())

		fmt.Printf("Recipe Settings:\n")
		fmt.Printf("  Default Author:    %s\n", config.Recipes.DefaultAuthor)
		fmt.Printf("  Default License:   %s\n", config.Recipes.DefaultLicense)
		fmt.Printf("  Auto Validate:     %t\n", config.Recipes.AutoValidate)
		fmt.Printf("  Preferred Format:  %s\n", config.Recipes.PreferredFormat)
		fmt.Printf("\n")

		fmt.Printf("Storage Settings:\n")
		fmt.Printf("  Cache Recipes:     %t\n", config.Storage.CacheRecipes)
		fmt.Printf("  Cache Duration:    %v\n", config.Storage.CacheDuration)
		fmt.Printf("  Max Cache Size:    %s\n", config.Storage.MaxCacheSize)
		fmt.Printf("  Backup Location:   %s\n", config.Storage.BackupLocation)
		fmt.Printf("\n")

		fmt.Printf("Execution Settings:\n")
		fmt.Printf("  Default Timeout:   %v\n", config.Execution.DefaultTimeout)
		fmt.Printf("  Auto Backup:       %t\n", config.Execution.AutoBackup)
		fmt.Printf("  Backup Location:   %s\n", config.Execution.BackupLocation)
		fmt.Printf("  Max Concurrent:    %d\n", config.Execution.MaxConcurrentRuns)
		fmt.Printf("  Retry Attempts:    %d\n", config.Execution.RetryAttempts)
		fmt.Printf("\n")

		fmt.Printf("Display Settings:\n")
		fmt.Printf("  Output Format:     %s\n", config.Display.DefaultOutputFormat)
		fmt.Printf("  Page Size:         %d\n", config.Display.DefaultPageSize)
		fmt.Printf("  Progress Bars:     %t\n", config.Display.ShowProgressBars)
		fmt.Printf("  Color Output:      %t\n", config.Display.ColorOutput)
		fmt.Printf("  Verbose Default:   %t\n", config.Display.VerboseByDefault)
		fmt.Printf("\n")

		fmt.Printf("Template Settings:\n")
		fmt.Printf("  Default Template:  %s\n", config.Templates.DefaultTemplate)
		fmt.Printf("  Custom Path:       %s\n", config.Templates.CustomTemplatePath)
		fmt.Printf("  Autoload Custom:   %t\n", config.Templates.AutoloadCustom)
	}

	return nil
}

// UpdateConfig updates a configuration value
func UpdateConfig(key, value string) error {
	config, err := GetConfig()
	if err != nil {
		return err
	}

	// Parse the key path and update the appropriate field
	switch key {
	case "recipes.default_author":
		config.Recipes.DefaultAuthor = value
	case "recipes.default_license":
		config.Recipes.DefaultLicense = value
	case "recipes.preferred_format":
		if value != "yaml" && value != "json" {
			return NewCLIError("Preferred format must be 'yaml' or 'json'", 1)
		}
		config.Recipes.PreferredFormat = value
	case "display.default_output_format":
		if err := ValidateOutputFormat(value); err != nil {
			return err
		}
		config.Display.DefaultOutputFormat = value
	case "display.default_page_size":
		pageSize, err := strconv.Atoi(value)
		if err != nil {
			return NewCLIError("Page size must be a number", 1)
		}
		if pageSize <= 0 || pageSize > 1000 {
			return NewCLIError("Page size must be between 1 and 1000", 1)
		}
		config.Display.DefaultPageSize = pageSize
	case "execution.default_timeout":
		timeout, err := time.ParseDuration(value)
		if err != nil {
			return NewCLIError("Invalid timeout format", 1).
				WithSuggestion("Use format like '15m', '1h', '30s'")
		}
		config.Execution.DefaultTimeout = timeout
	case "templates.default_template":
		// Validate template exists
		if !isValidTemplate(value) {
			return NewCLIError(fmt.Sprintf("Invalid template: %s", value), 1).
				WithSuggestion("Use: openrewrite, shell, composite")
		}
		config.Templates.DefaultTemplate = value
	default:
		return NewCLIError(fmt.Sprintf("Unknown configuration key: %s", key), 1).
			WithSuggestion("Use 'ploy recipe config list' to see available keys")
	}

	// Validate the updated config
	if err := ValidateConfig(config); err != nil {
		return err
	}

	// Save the config
	if err := SaveConfig(config); err != nil {
		return err
	}

	PrintSuccess(fmt.Sprintf("Configuration updated: %s = %s", key, value))
	return nil
}

// ResetConfig resets configuration to defaults
func ResetConfig() error {
	if !ConfirmAction("reset configuration to defaults", false) {
		PrintInfo("Configuration reset cancelled")
		return nil
	}

	defaultConfig := GetDefaultConfig()
	if err := SaveConfig(defaultConfig); err != nil {
		return err
	}

	PrintSuccess("Configuration reset to defaults")
	return nil
}

// ListConfigKeys lists all available configuration keys
func ListConfigKeys() error {
	keys := []struct {
		Key         string
		Description string
		Default     string
	}{
		{"recipes.default_author", "Default recipe author name", "ploy-user"},
		{"recipes.default_license", "Default recipe license", "MIT"},
		{"recipes.preferred_format", "Preferred recipe file format (yaml|json)", "yaml"},
		{"display.default_output_format", "Default output format (table|json|yaml)", "table"},
		{"display.default_page_size", "Default page size for listings", "20"},
		{"execution.default_timeout", "Default execution timeout", "15m"},
		{"templates.default_template", "Default template for recipe creation", "openrewrite"},
	}

	fmt.Printf("Available Configuration Keys:\n\n")
	for _, key := range keys {
		fmt.Printf("  %-30s %s\n", key.Key, key.Description)
		fmt.Printf("  %-30s (default: %s)\n\n", "", key.Default)
	}

	fmt.Printf("Usage:\n")
	fmt.Printf("  ploy recipe config show                    # Show current config\n")
	fmt.Printf("  ploy recipe config set <key> <value>      # Update config value\n")
	fmt.Printf("  ploy recipe config reset                  # Reset to defaults\n")

	return nil
}

// Helper functions

func isValidTemplate(template string) bool {
	validTemplates := []string{"openrewrite", "shell", "composite"}
	for _, valid := range validTemplates {
		if template == valid {
			return true
		}
	}
	return false
}
