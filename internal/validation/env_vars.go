package validation

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// MaxEnvVarNameLength is the maximum length for an environment variable name
	MaxEnvVarNameLength = 255

	// MaxEnvVarValueLength is the maximum length for an environment variable value (64KB)
	MaxEnvVarValueLength = 65536

	// MaxEnvVarCount is the maximum number of environment variables per app
	MaxEnvVarCount = 1000
)

// envVarNameRegex validates environment variable names
// Must start with letter or underscore, followed by letters, numbers, or underscores
var envVarNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// reservedEnvVars contains environment variables that should not be modified
var reservedEnvVars = map[string]bool{
	"PATH":                  true,
	"HOME":                  true,
	"USER":                  true,
	"SHELL":                 true,
	"PWD":                   true,
	"OLDPWD":                true,
	"LD_PRELOAD":            true,
	"LD_LIBRARY_PATH":       true,
	"DYLD_INSERT_LIBRARIES": true,
	"DYLD_LIBRARY_PATH":     true,
	"IFS":                   true,
	"CDPATH":                true,
	"ENV":                   true,
	"BASH_ENV":              true,
}

// ValidateEnvVarName validates an environment variable name
func ValidateEnvVarName(name string) error {
	// Check for empty name
	if name == "" {
		return fmt.Errorf("environment variable name cannot be empty")
	}

	// Check length
	if len(name) > MaxEnvVarNameLength {
		return fmt.Errorf("environment variable name too long (max %d characters)", MaxEnvVarNameLength)
	}

	// Check for null bytes
	if strings.Contains(name, "\x00") {
		return fmt.Errorf("environment variable name contains invalid character (null byte)")
	}

	// Check if it's a reserved variable
	if IsReservedEnvVar(name) {
		return fmt.Errorf("environment variable name '%s' is reserved and cannot be modified", name)
	}

	// Check format using regex
	if !envVarNameRegex.MatchString(name) {
		// Provide more specific error messages
		if name[0] >= '0' && name[0] <= '9' {
			return fmt.Errorf("environment variable name must start with letter or underscore, not a number")
		}

		// Check for common invalid characters
		for _, char := range name {
			if char == ' ' {
				return fmt.Errorf("environment variable name contains invalid character (space)")
			}
			if char == '-' || char == '.' || char == '=' || char == '$' ||
				char == '[' || char == ']' || char == ';' || char == ',' {
				return fmt.Errorf("environment variable name contains invalid character '%c'", char)
			}
			if char < 32 || char > 126 {
				return fmt.Errorf("environment variable name contains invalid character (control or non-ASCII)")
			}
		}

		return fmt.Errorf("environment variable name contains invalid characters (only letters, numbers, and underscores allowed)")
	}

	return nil
}

// ValidateEnvVarValue validates an environment variable value
func ValidateEnvVarValue(value string) error {
	// Empty values are allowed
	if value == "" {
		return nil
	}

	// Check length
	if len(value) > MaxEnvVarValueLength {
		return fmt.Errorf("environment variable value too long (max %d characters)", MaxEnvVarValueLength)
	}

	// Check for null bytes (security risk)
	if strings.Contains(value, "\x00") {
		return fmt.Errorf("environment variable value contains null byte")
	}

	// Check for other control characters (except common ones like \n, \t)
	for i, char := range value {
		if char < 32 && char != '\n' && char != '\r' && char != '\t' {
			return fmt.Errorf("environment variable value contains control character at position %d", i)
		}
	}

	// Note: We allow values that look like commands, SQL, or scripts
	// because they might be legitimate values. The application using
	// these values should properly escape/quote them.

	return nil
}

// ValidateEnvVars validates a map of environment variables
func ValidateEnvVars(envVars map[string]string) error {
	// Nil or empty map is valid
	if envVars == nil || len(envVars) == 0 {
		return nil
	}

	// Check total count
	if len(envVars) > MaxEnvVarCount {
		return fmt.Errorf("too many environment variables (max %d)", MaxEnvVarCount)
	}

	// Validate each variable
	for name, value := range envVars {
		if err := ValidateEnvVarName(name); err != nil {
			return fmt.Errorf("invalid environment variable '%s': %w", name, err)
		}

		if err := ValidateEnvVarValue(value); err != nil {
			return fmt.Errorf("invalid value for environment variable '%s': %w", name, err)
		}
	}

	return nil
}

// IsReservedEnvVar checks if an environment variable name is reserved
func IsReservedEnvVar(name string) bool {
	return reservedEnvVars[name]
}

// SanitizeEnvVarValue removes potentially dangerous characters from a value
// This is a helper function for cases where you want to clean input rather than reject it
func SanitizeEnvVarValue(value string) string {
	// Remove null bytes
	value = strings.ReplaceAll(value, "\x00", "")

	// Remove other control characters except common whitespace
	var sanitized strings.Builder
	for _, char := range value {
		if char >= 32 || char == '\n' || char == '\r' || char == '\t' {
			sanitized.WriteRune(char)
		}
	}

	result := sanitized.String()

	// Truncate if too long
	if len(result) > MaxEnvVarValueLength {
		result = result[:MaxEnvVarValueLength]
	}

	return result
}
