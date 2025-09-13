package arf

import (
	"fmt"
	"os"
	"strings"
)

// CLIError represents a CLI-specific error with user-friendly messaging
type CLIError struct {
	Message    string
	Suggestion string
	ExitCode   int
	ShowUsage  bool
	Cause      error
}

func (e *CLIError) Error() string {
	return e.Message
}

// NewCLIError creates a new CLI error
func NewCLIError(message string, exitCode int) *CLIError {
	return &CLIError{
		Message:  message,
		ExitCode: exitCode,
	}
}

// WithSuggestion adds a suggestion to the error
func (e *CLIError) WithSuggestion(suggestion string) *CLIError {
	e.Suggestion = suggestion
	return e
}

// WithUsage marks the error as requiring usage display
func (e *CLIError) WithUsage() *CLIError {
	e.ShowUsage = true
	return e
}

// WithCause adds an underlying cause to the error
func (e *CLIError) WithCause(cause error) *CLIError {
	e.Cause = cause
	return e
}

// PrintError prints a formatted error message
func PrintError(err error) {
	if cliErr, ok := err.(*CLIError); ok {
		// Print main error message
		fmt.Fprintf(os.Stderr, "❌ Error: %s\n", cliErr.Message)

		// Print suggestion if available
		if cliErr.Suggestion != "" {
			fmt.Fprintf(os.Stderr, "💡 Suggestion: %s\n", cliErr.Suggestion)
		}

		// Print underlying cause if available and verbose mode
		if cliErr.Cause != nil && os.Getenv("PLOY_VERBOSE") == "true" {
			fmt.Fprintf(os.Stderr, "🔍 Details: %v\n", cliErr.Cause)
		}

		// Print usage if requested
		if cliErr.ShowUsage {
			fmt.Fprintf(os.Stderr, "\n")
			printRecipesUsage()
		}
	} else {
		// Generic error handling
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
	}
}

// PrintSuccess prints a success message
func PrintSuccess(message string) {
	fmt.Printf("✅ %s\n", message)
}

// PrintWarning prints a warning message
func PrintWarning(message string) {
	fmt.Printf("⚠️  Warning: %s\n", message)
}

// PrintInfo prints an info message
func PrintInfo(message string) {
	fmt.Printf("ℹ️  %s\n", message)
}

// ValidateRecipeID validates a recipe ID format
func ValidateRecipeID(recipeID string) error {
	if recipeID == "" {
		return NewCLIError("Recipe ID cannot be empty", 1).
			WithSuggestion("Provide a valid recipe ID, e.g., 'java11to17-1.0.0'")
	}

	if len(recipeID) < 3 {
		return NewCLIError("Recipe ID is too short", 1).
			WithSuggestion("Recipe ID should be at least 3 characters long")
	}

	if strings.Contains(recipeID, " ") {
		return NewCLIError("Recipe ID cannot contain spaces", 1).
			WithSuggestion("Use hyphens (-) or underscores (_) instead of spaces")
	}

	return nil
}

// ValidateFilePath validates a file path
func ValidateFilePath(filePath string) error {
	if filePath == "" {
		return NewCLIError("File path cannot be empty", 1)
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return NewCLIError(fmt.Sprintf("File not found: %s", filePath), 1).
			WithSuggestion("Check the file path and ensure the file exists")
	}

	return nil
}

// ValidateOutputFormat validates output format
func ValidateOutputFormat(format string) error {
	validFormats := []string{"table", "json", "yaml"}

	format = strings.ToLower(format)
	for _, valid := range validFormats {
		if format == valid {
			return nil
		}
	}

	return NewCLIError(fmt.Sprintf("Invalid output format: %s", format), 1).
		WithSuggestion(fmt.Sprintf("Valid formats: %s", strings.Join(validFormats, ", ")))
}

// ValidateRequired validates that a required field is not empty
func ValidateRequired(field, value, fieldName string) error {
	if value == "" {
		return NewCLIError(fmt.Sprintf("%s is required", fieldName), 1).
			WithSuggestion(fmt.Sprintf("Provide a valid %s", strings.ToLower(fieldName)))
	}
	return nil
}

// ConfirmAction prompts user for confirmation unless force flag is set
func ConfirmAction(action string, force bool) bool {
	if force {
		return true
	}

	fmt.Printf("Are you sure you want to %s? (y/N): ", action)
	var response string
	_, _ = fmt.Scanln(&response)

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// FormatFileSize formats a file size in bytes to human-readable format
func FormatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// TruncateString truncates a string to the specified length with ellipsis
func TruncateString(str string, length int) string {
	if len(str) <= length {
		return str
	}
	if length <= 3 {
		return str[:length]
	}
	return str[:length-3] + "..."
}
