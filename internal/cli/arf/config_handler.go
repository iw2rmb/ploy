package arf

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// handleConfigCommand handles configuration management commands
func handleConfigCommand(args []string) error {
	if len(args) == 0 {
		return ShowConfig("table")
	}

	subcommand := args[0]
	switch subcommand {
	case "show", "get":
		outputFormat := "table"
		if len(args) > 1 && (args[1] == "--output" || args[1] == "-o") && len(args) > 2 {
			outputFormat = args[2]
		}
		return ShowConfig(outputFormat)

	case "set", "update":
		if len(args) < 3 {
			return NewCLIError("Configuration key and value are required", 1).
				WithSuggestion("Usage: ploy arf recipe config set <key> <value>").
				WithUsage()
		}
		return UpdateConfig(args[1], args[2])

	case "reset":
		return ResetConfig()

	case "list", "keys":
		return ListConfigKeys()

	case "init":
		return initializeConfig(args[1:])

	case "validate":
		return validateCurrentConfig()

	case "backup":
		return backupConfig(args[1:])

	case "restore":
		if len(args) < 2 {
			return NewCLIError("Backup file path is required", 1).
				WithSuggestion("Usage: ploy arf recipe config restore <backup-file>")
		}
		return restoreConfig(args[1])

	case "--help", "-h":
		return showConfigHelp()

	default:
		return NewCLIError(fmt.Sprintf("Unknown config command: %s", subcommand), 1).
			WithSuggestion("Use: show, set, reset, list, init, validate, backup, restore")
	}
}

// initializeConfig creates a new configuration interactively
func initializeConfig(args []string) error {
	PrintInfo("Initializing Ploy ARF configuration")
	fmt.Println()

	// Check if config already exists
	configPath := GetConfigPath()
	if _, err := os.Stat(configPath); err == nil {
		if !ConfirmAction(fmt.Sprintf("overwrite existing config at %s", configPath), false) {
			PrintInfo("Configuration initialization cancelled")
			return nil
		}
	}

	// Interactive configuration
	config := GetDefaultConfig()

	// Recipe settings
	fmt.Printf("Recipe Settings:\n")
	if author := promptInput(fmt.Sprintf("Default author [%s]: ", config.Recipes.DefaultAuthor)); author != "" {
		config.Recipes.DefaultAuthor = author
	}

	if license := promptInput(fmt.Sprintf("Default license [%s]: ", config.Recipes.DefaultLicense)); license != "" {
		config.Recipes.DefaultLicense = license
	}

	autoValidateStr := "y"
	if !config.Recipes.AutoValidate {
		autoValidateStr = "n"
	}
	if confirm := promptInput(fmt.Sprintf("Auto-validate recipes (y/n) [%s]: ", autoValidateStr)); confirm != "" {
		config.Recipes.AutoValidate = strings.ToLower(confirm) == "y"
	}
	fmt.Println()

	// Display settings
	fmt.Printf("Display Settings:\n")
	if format := promptInput(fmt.Sprintf("Default output format (table/json/yaml) [%s]: ", config.Display.DefaultOutputFormat)); format != "" {
		if err := ValidateOutputFormat(format); err != nil {
			PrintWarning(fmt.Sprintf("Invalid format '%s', using default", format))
		} else {
			config.Display.DefaultOutputFormat = format
		}
	}

	if pageSizeStr := promptInput(fmt.Sprintf("Default page size [%d]: ", config.Display.DefaultPageSize)); pageSizeStr != "" {
		if pageSize, err := strconv.Atoi(pageSizeStr); err == nil && pageSize > 0 && pageSize <= 1000 {
			config.Display.DefaultPageSize = pageSize
		} else {
			PrintWarning(fmt.Sprintf("Invalid page size '%s', using default", pageSizeStr))
		}
	}

	colorStr := "y"
	if !config.Display.ColorOutput {
		colorStr = "n"
	}
	if confirm := promptInput(fmt.Sprintf("Enable color output (y/n) [%s]: ", colorStr)); confirm != "" {
		config.Display.ColorOutput = strings.ToLower(confirm) == "y"
	}
	fmt.Println()

	// Execution settings
	fmt.Printf("Execution Settings:\n")
	if timeoutStr := promptInput(fmt.Sprintf("Default timeout [%v]: ", config.Execution.DefaultTimeout)); timeoutStr != "" {
		if timeout, err := time.ParseDuration(timeoutStr); err == nil {
			config.Execution.DefaultTimeout = timeout
		} else {
			PrintWarning(fmt.Sprintf("Invalid timeout '%s', using default", timeoutStr))
		}
	}

	if concurrentStr := promptInput(fmt.Sprintf("Max concurrent runs [%d]: ", config.Execution.MaxConcurrentRuns)); concurrentStr != "" {
		if concurrent, err := strconv.Atoi(concurrentStr); err == nil && concurrent > 0 && concurrent <= 10 {
			config.Execution.MaxConcurrentRuns = concurrent
		} else {
			PrintWarning(fmt.Sprintf("Invalid concurrent value '%s', using default", concurrentStr))
		}
	}
	fmt.Println()

	// Template settings
	fmt.Printf("Template Settings:\n")
	if template := promptInput(fmt.Sprintf("Default template (openrewrite/shell/composite) [%s]: ", config.Templates.DefaultTemplate)); template != "" {
		if isValidTemplate(template) {
			config.Templates.DefaultTemplate = template
		} else {
			PrintWarning(fmt.Sprintf("Invalid template '%s', using default", template))
		}
	}

	// Validate and save
	if err := ValidateConfig(config); err != nil {
		return NewCLIError("Configuration validation failed", 1).WithCause(err)
	}

	if err := SaveConfig(config); err != nil {
		return err
	}

	PrintSuccess(fmt.Sprintf("Configuration initialized at %s", configPath))
	return nil
}

// validateCurrentConfig validates the current configuration
func validateCurrentConfig() error {
	config, err := GetConfig()
	if err != nil {
		return err
	}

	if err := ValidateConfig(config); err != nil {
		PrintError(err)
		return err
	}

	PrintSuccess("Configuration is valid")

	// Show any recommendations
	recommendations := []string{}

	if config.Storage.BackupLocation == "" {
		recommendations = append(recommendations, "Consider setting a backup location for recipes")
	}

	if config.Execution.DefaultTimeout > 30*time.Minute {
		recommendations = append(recommendations, "Default timeout is quite high, consider reducing it")
	}

	if len(recommendations) > 0 {
		fmt.Printf("\nRecommendations:\n")
		for _, rec := range recommendations {
			fmt.Printf("  • %s\n", rec)
		}
	}

	return nil
}

// backupConfig creates a backup of the current configuration
func backupConfig(args []string) error {
	config, err := GetConfig()
	if err != nil {
		return err
	}

	// Determine backup file name
	backupFile := ""
	if len(args) > 0 {
		backupFile = args[0]
	} else {
		timestamp := time.Now().Format("20060102-150405")
		backupFile = fmt.Sprintf("ploy-config-backup-%s.yaml", timestamp)
	}

	// Convert to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return NewCLIError("Failed to serialize configuration", 1).WithCause(err)
	}

	// Write backup file
	if err := os.WriteFile(backupFile, data, 0644); err != nil {
		return NewCLIError(fmt.Sprintf("Failed to write backup file: %s", backupFile), 1).WithCause(err)
	}

	PrintSuccess(fmt.Sprintf("Configuration backed up to %s", backupFile))
	return nil
}

// restoreConfig restores configuration from a backup file
func restoreConfig(backupFile string) error {
	// Validate backup file exists
	if err := ValidateFilePath(backupFile); err != nil {
		return err
	}

	// Confirm restore operation
	if !ConfirmAction(fmt.Sprintf("restore configuration from %s", backupFile), false) {
		PrintInfo("Configuration restore cancelled")
		return nil
	}

	// Read backup file
	data, err := os.ReadFile(backupFile)
	if err != nil {
		return NewCLIError(fmt.Sprintf("Failed to read backup file: %s", backupFile), 1).WithCause(err)
	}

	// Parse backup configuration
	var config CLIConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return NewCLIError("Invalid backup file format", 1).
			WithCause(err).
			WithSuggestion("Ensure the backup file is a valid YAML configuration")
	}

	// Validate restored config
	if err := ValidateConfig(&config); err != nil {
		return NewCLIError("Backup configuration is invalid", 1).WithCause(err)
	}

	// Save restored configuration
	if err := SaveConfig(&config); err != nil {
		return err
	}

	PrintSuccess(fmt.Sprintf("Configuration restored from %s", backupFile))
	return nil
}

// showConfigHelp displays help for configuration commands
func showConfigHelp() error {
	fmt.Printf("Ploy ARF Configuration Management\n")
	fmt.Printf("=================================\n\n")

	fmt.Printf("Usage: ploy arf recipe config <command> [arguments]\n\n")

	fmt.Printf("Available commands:\n")
	fmt.Printf("  show, get                        Display current configuration\n")
	fmt.Printf("  set <key> <value>                Update configuration value\n")
	fmt.Printf("  reset                            Reset to default configuration\n")
	fmt.Printf("  list, keys                       List all configuration keys\n")
	fmt.Printf("  init                             Interactive configuration setup\n")
	fmt.Printf("  validate                         Validate current configuration\n")
	fmt.Printf("  backup [file]                    Backup current configuration\n")
	fmt.Printf("  restore <file>                   Restore from backup file\n")

	fmt.Printf("\nExamples:\n")
	fmt.Printf("  ploy arf recipe config show\n")
	fmt.Printf("  ploy arf recipe config show --output json\n")
	fmt.Printf("  ploy arf recipe config set recipes.default_author 'John Doe'\n")
	fmt.Printf("  ploy arf recipe config set display.default_page_size 50\n")
	fmt.Printf("  ploy arf recipe config backup my-config-backup.yaml\n")
	fmt.Printf("  ploy arf recipe config restore my-config-backup.yaml\n")

	fmt.Printf("\nConfiguration file location: %s\n", GetConfigPath())
	fmt.Printf("Use 'ploy arf recipe config list' to see all available configuration keys.\n")

	return nil
}

// ApplyConfigDefaults applies configuration defaults to command flags
func ApplyConfigDefaults(flags *CommandFlags) error {
	config, err := GetConfig()
	if err != nil {
		// If we can't load config, continue with current flags
		return nil
	}

	// Apply defaults only if not explicitly set
	if flags.OutputFormat == "table" && config.Display.DefaultOutputFormat != "table" {
		flags.OutputFormat = config.Display.DefaultOutputFormat
	}

	if !flags.Verbose && config.Display.VerboseByDefault {
		flags.Verbose = true
	}

	// Apply template defaults for creation commands
	if flags.Template == "" && config.Templates.DefaultTemplate != "" {
		flags.Template = config.Templates.DefaultTemplate
	}

	return nil
}

// GetConfiguredPageSize returns the configured page size for listings
func GetConfiguredPageSize() int {
	config, err := GetConfig()
	if err != nil {
		return 20 // Default fallback
	}
	return config.Display.DefaultPageSize
}

// GetConfiguredTimeout returns the configured timeout for executions
func GetConfiguredTimeout() time.Duration {
	config, err := GetConfig()
	if err != nil {
		return 15 * time.Minute // Default fallback
	}
	return config.Execution.DefaultTimeout
}
