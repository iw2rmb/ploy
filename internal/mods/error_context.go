package mods

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrorContext represents error context for analysis and prompt helpers
// Minimal definition retained after removing legacy LLM types.
type ErrorContext struct {
	ErrorMessage   string                 `json:"error_message"`
	ErrorType      string                 `json:"error_type"`
	ErrorDetails   map[string]string      `json:"error_details"`
	StackTrace     []string               `json:"stack_trace,omitempty"`
	SourceFile     string                 `json:"source_file,omitempty"`
	CompilerOutput string                 `json:"compiler_output,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
}

// EnhancedErrorPattern represents a pattern for error detection in LLM analysis
type EnhancedErrorPattern struct {
	Pattern    *regexp.Regexp
	Type       string
	Confidence float64
	Language   string
	Extractor  func([]string) string // Extract relevant info from error
}

// ExtractErrorContext is a standalone function that extracts context from error messages
func ExtractErrorContext(errors []string, language string) ErrorContext {
	context := ErrorContext{
		ErrorMessage: strings.Join(errors, "\n"),
		ErrorType:    "compilation",
		ErrorDetails: make(map[string]string),
		Timestamp:    time.Now(),
	}

	// Detect error type
	errorText := strings.ToLower(strings.Join(errors, " "))
	if strings.Contains(errorText, "test") && (strings.Contains(errorText, "fail") || strings.Contains(errorText, "assertion")) {
		context.ErrorType = "test"
	} else if strings.Contains(errorText, "import") || strings.Contains(errorText, "module") {
		context.ErrorType = "import"
	} else if strings.Contains(errorText, "dependency") || strings.Contains(errorText, "version") {
		context.ErrorType = "dependency"
	}

	// Extract source file and line number
	for _, err := range errors {
		lines := strings.Split(err, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)

			// Look for file:line:column pattern
			if strings.Contains(trimmed, ".go:") || strings.Contains(trimmed, ".java:") ||
				strings.Contains(trimmed, ".py:") || strings.Contains(trimmed, ".js:") ||
				strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "/") {

				// Handle Go-specific pattern "main.go:5:2: error"
				if strings.Contains(trimmed, ".go:") {
					// Find the .go: pattern
					goIndex := strings.Index(trimmed, ".go:")
					if goIndex != -1 {
						// Find start of filename (could be just "main.go" or "./main.go")
						start := 0
						for i := goIndex - 1; i >= 0; i-- {
							if trimmed[i] == ' ' || trimmed[i] == '\t' {
								start = i + 1
								break
							}
						}
						filePath := trimmed[start : goIndex+3] // Include ".go"
						context.SourceFile = filePath

						// Extract line number
						remainder := trimmed[goIndex+4:] // After ".go:"
						parts := strings.Split(remainder, ":")
						if len(parts) > 0 {
							lineNum := strings.TrimSpace(parts[0])
							if _, err := strconv.Atoi(lineNum); err == nil {
								context.ErrorDetails["line_number"] = lineNum
							}
						}
						break
					}
				} else {
					// Generic file:line pattern
					parts := strings.Split(trimmed, ":")
					if len(parts) >= 2 {
						context.SourceFile = parts[0]
						// Try to parse line number
						lineNum := strings.TrimSpace(parts[1])
						if _, err := strconv.Atoi(lineNum); err == nil {
							context.ErrorDetails["line_number"] = lineNum
						}
						break
					}
				}
			}
		}
		if context.SourceFile != "" {
			break
		}
	}

	// Extract stack trace for runtime errors
	var stackTrace []string
	for _, err := range errors {
		if strings.Contains(err, "\tat ") || strings.Contains(err, "goroutine") {
			lines := strings.Split(err, "\n")
			for _, line := range lines {
				if strings.Contains(line, "\tat ") || strings.Contains(line, ".go:") || strings.Contains(line, ".java:") {
					stackTrace = append(stackTrace, strings.TrimSpace(line))
				}
			}
		}
	}
	if len(stackTrace) > 0 {
		context.StackTrace = stackTrace
		if context.ErrorType == "compilation" {
			context.ErrorType = "runtime"
		}
	}

	return context
}
