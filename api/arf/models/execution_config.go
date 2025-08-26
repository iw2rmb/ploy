package models

import (
	"fmt"
	"strings"
	"time"
)

// ExecutionConfig controls recipe execution behavior
type ExecutionConfig struct {
	Parallelism int               `json:"parallelism,omitempty" yaml:"parallelism,omitempty"`
	MaxDuration Duration          `json:"max_duration,omitempty" yaml:"max_duration,omitempty"`
	RetryPolicy RetryPolicy       `json:"retry_policy,omitempty" yaml:"retry_policy,omitempty"`
	Sandbox     SandboxConfig     `json:"sandbox,omitempty" yaml:"sandbox,omitempty"`
	Environment map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`
	WorkingDir  string            `json:"working_dir,omitempty" yaml:"working_dir,omitempty"`
}

// RetryPolicy defines retry behavior for failed steps
type RetryPolicy struct {
	MaxAttempts int      `json:"max_attempts,omitempty" yaml:"max_attempts,omitempty"`
	Backoff     Duration `json:"backoff,omitempty" yaml:"backoff,omitempty"`
	MaxBackoff  Duration `json:"max_backoff,omitempty" yaml:"max_backoff,omitempty"`
}

// SandboxConfig defines sandbox execution environment settings
type SandboxConfig struct {
	Enabled           bool     `json:"enabled" yaml:"enabled"`
	AllowNetwork      bool     `json:"allow_network,omitempty" yaml:"allow_network,omitempty"`
	AllowFileWrite    bool     `json:"allow_file_write,omitempty" yaml:"allow_file_write,omitempty"`
	MaxMemory         string   `json:"max_memory,omitempty" yaml:"max_memory,omitempty"`
	MaxCPU            float64  `json:"max_cpu,omitempty" yaml:"max_cpu,omitempty"`
	MaxDiskUsage      string   `json:"max_disk_usage,omitempty" yaml:"max_disk_usage,omitempty"`
	AllowedPaths      []string `json:"allowed_paths,omitempty" yaml:"allowed_paths,omitempty"`
	BlockedPaths      []string `json:"blocked_paths,omitempty" yaml:"blocked_paths,omitempty"`
	AllowedCommands   []string `json:"allowed_commands,omitempty" yaml:"allowed_commands,omitempty"`
	BlockedCommands   []string `json:"blocked_commands,omitempty" yaml:"blocked_commands,omitempty"`
	DockerImage       string   `json:"docker_image,omitempty" yaml:"docker_image,omitempty"`
	IsolationLevel    string   `json:"isolation_level,omitempty" yaml:"isolation_level,omitempty"`
}

// Validate validates the execution configuration
func (c *ExecutionConfig) Validate() error {
	// Validate parallelism
	if c.Parallelism < 0 {
		return fmt.Errorf("parallelism cannot be negative")
	}
	if c.Parallelism > 10 {
		return fmt.Errorf("parallelism cannot exceed 10")
	}

	// Validate max duration
	if c.MaxDuration.Duration < 0 {
		return fmt.Errorf("max_duration cannot be negative")
	}
	if c.MaxDuration.Duration > 0 && c.MaxDuration.Duration < time.Second {
		return fmt.Errorf("max_duration must be at least 1 second")
	}
	if c.MaxDuration.Duration > 2*time.Hour {
		return fmt.Errorf("max_duration cannot exceed 2 hours")
	}

	// Validate retry policy
	if err := c.RetryPolicy.Validate(); err != nil {
		return fmt.Errorf("retry policy validation failed: %w", err)
	}

	// Validate sandbox configuration
	if err := c.Sandbox.Validate(); err != nil {
		return fmt.Errorf("sandbox validation failed: %w", err)
	}

	// Validate environment variables
	for key := range c.Environment {
		if key == "" {
			return fmt.Errorf("environment variable key cannot be empty")
		}
		// Check for reserved environment variables
		if isReservedEnvVar(key) {
			return fmt.Errorf("cannot override reserved environment variable: %s", key)
		}
	}

	return nil
}

// Validate validates the retry policy
func (p *RetryPolicy) Validate() error {
	if p.MaxAttempts < 0 {
		return fmt.Errorf("max_attempts cannot be negative")
	}
	if p.MaxAttempts > 5 {
		return fmt.Errorf("max_attempts cannot exceed 5")
	}

	if p.Backoff.Duration < 0 {
		return fmt.Errorf("backoff cannot be negative")
	}
	if p.MaxBackoff.Duration < 0 {
		return fmt.Errorf("max_backoff cannot be negative")
	}

	if p.MaxBackoff.Duration > 0 && p.Backoff.Duration > p.MaxBackoff.Duration {
		return fmt.Errorf("backoff cannot exceed max_backoff")
	}

	return nil
}

// Validate validates the sandbox configuration
func (s *SandboxConfig) Validate() error {
	// If sandbox is not enabled, skip validation
	if !s.Enabled {
		return nil
	}

	// Validate memory limit
	if s.MaxMemory != "" {
		if !isValidMemorySize(s.MaxMemory) {
			return fmt.Errorf("invalid memory size format: %s", s.MaxMemory)
		}
	}

	// Validate CPU limit
	if s.MaxCPU < 0 {
		return fmt.Errorf("max_cpu cannot be negative")
	}
	if s.MaxCPU > 8 {
		return fmt.Errorf("max_cpu cannot exceed 8 cores")
	}

	// Validate disk usage limit
	if s.MaxDiskUsage != "" {
		if !isValidDiskSize(s.MaxDiskUsage) {
			return fmt.Errorf("invalid disk size format: %s", s.MaxDiskUsage)
		}
	}

	// Validate blocked commands
	for _, cmd := range s.BlockedCommands {
		if cmd == "" {
			return fmt.Errorf("blocked command cannot be empty")
		}
	}

	// Validate isolation level
	if s.IsolationLevel != "" {
		validLevels := []string{"none", "low", "medium", "high", "strict"}
		valid := false
		for _, level := range validLevels {
			if s.IsolationLevel == level {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid isolation level: %s", s.IsolationLevel)
		}
	}

	// Validate Docker image if specified
	if s.DockerImage != "" {
		if !isValidDockerImage(s.DockerImage) {
			return fmt.Errorf("invalid Docker image format: %s", s.DockerImage)
		}
	}

	return nil
}

// SetDefaults sets default values for execution configuration
func (c *ExecutionConfig) SetDefaults() {
	if c.Parallelism == 0 {
		c.Parallelism = 1
	}

	if c.MaxDuration.Duration == 0 {
		c.MaxDuration.Duration = 15 * time.Minute
	}

	if c.RetryPolicy.MaxAttempts == 0 {
		c.RetryPolicy.MaxAttempts = 1 // No retries by default
	}

	if c.RetryPolicy.Backoff.Duration == 0 {
		c.RetryPolicy.Backoff.Duration = 5 * time.Second
	}

	if c.RetryPolicy.MaxBackoff.Duration == 0 {
		c.RetryPolicy.MaxBackoff.Duration = 30 * time.Second
	}

	// Set sandbox defaults
	c.Sandbox.SetDefaults()
}

// SetDefaults sets default values for sandbox configuration
func (s *SandboxConfig) SetDefaults() {
	if !s.Enabled {
		return
	}

	if s.MaxMemory == "" {
		s.MaxMemory = "2GB"
	}

	if s.MaxCPU == 0 {
		s.MaxCPU = 2
	}

	if s.MaxDiskUsage == "" {
		s.MaxDiskUsage = "5GB"
	}

	if s.IsolationLevel == "" {
		s.IsolationLevel = "medium"
	}

	// Add default blocked commands for security
	defaultBlocked := []string{
		"rm -rf /",
		"sudo",
		"su",
		"passwd",
		"shutdown",
		"reboot",
		"mkfs",
		"dd if=/dev/zero",
		"fork bomb",
		":(){ :|:& };:",
	}

	// Merge with existing blocked commands
	blockMap := make(map[string]bool)
	for _, cmd := range s.BlockedCommands {
		blockMap[cmd] = true
	}
	for _, cmd := range defaultBlocked {
		blockMap[cmd] = true
	}

	s.BlockedCommands = make([]string, 0, len(blockMap))
	for cmd := range blockMap {
		s.BlockedCommands = append(s.BlockedCommands, cmd)
	}
}

// Helper functions

func isReservedEnvVar(key string) bool {
	reserved := []string{
		"PATH", "HOME", "USER", "SHELL", "PWD",
		"LANG", "LC_ALL", "TZ", "TMPDIR",
		"PLOY_RECIPE_ID", "PLOY_RECIPE_NAME",
		"PLOY_SANDBOX", "PLOY_WORKSPACE",
	}
	
	for _, r := range reserved {
		if key == r {
			return true
		}
	}
	return false
}

func isValidMemorySize(size string) bool {
	// Simple validation for memory size format (e.g., "512MB", "2GB")
	validSuffixes := []string{"B", "KB", "MB", "GB", "TB"}
	for _, suffix := range validSuffixes {
		if len(size) > len(suffix) {
			if size[len(size)-len(suffix):] == suffix {
				// Check if the prefix is a valid number
				num := size[:len(size)-len(suffix)]
				for _, r := range num {
					if r < '0' || r > '9' {
						return false
					}
				}
				return true
			}
		}
	}
	return false
}

func isValidDiskSize(size string) bool {
	// Uses same validation as memory size
	return isValidMemorySize(size)
}

func isValidDockerImage(image string) bool {
	// Simple validation for Docker image format
	// Format: [registry/]namespace/image[:tag]
	if image == "" {
		return false
	}
	
	// Check for invalid characters
	invalidChars := []string{" ", "\t", "\n", "\r"}
	for _, char := range invalidChars {
		if strings.Contains(image, char) {
			return false
		}
	}
	
	return true
}