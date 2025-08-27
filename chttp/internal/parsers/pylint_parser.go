package parsers

import (
	"encoding/json"
	"fmt"
)

// PylintMessage represents a single message from Pylint JSON output
type PylintMessage struct {
	Type      string `json:"type"`
	Module    string `json:"module"`
	Object    string `json:"obj"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Path      string `json:"path"`
	Symbol    string `json:"symbol"`
	Message   string `json:"message"`
	MessageID string `json:"message-id"`
}

// PylintParser parses Pylint JSON output
type PylintParser struct {
	name string
}

// NewPylintParser creates a new Pylint output parser
func NewPylintParser() *PylintParser {
	return &PylintParser{
		name: "pylint",
	}
}

// Name returns the parser name
func (p *PylintParser) Name() string {
	return p.name
}

// ParseOutput parses Pylint JSON output into issues
func (p *PylintParser) ParseOutput(stdout, stderr string, exitCode int) ([]Issue, error) {
	var issues []Issue
	
	// Handle execution errors
	if exitCode == 127 || (exitCode != 0 && stderr != "" && stdout == "") {
		// Command not found or execution error
		issues = append(issues, Issue{
			File:     "unknown",
			Line:     0,
			Column:   0,
			Severity: "error",
			Rule:     "execution-error",
			Message:  fmt.Sprintf("Pylint execution failed: %s", stderr),
		})
		return issues, nil
	}
	
	// Parse JSON output
	if stdout != "" {
		var messages []PylintMessage
		if err := json.Unmarshal([]byte(stdout), &messages); err != nil {
			// Try to parse as error message
			return []Issue{{
				File:     "unknown",
				Line:     0,
				Severity: "error",
				Rule:     "parse-error",
				Message:  fmt.Sprintf("Failed to parse Pylint output: %v", err),
			}}, nil
		}
		
		// Convert Pylint messages to issues
		for _, msg := range messages {
			severity := p.mapPylintTypeToSeverity(msg.Type)
			issues = append(issues, Issue{
				File:     msg.Path,
				Line:     msg.Line,
				Column:   msg.Column,
				Severity: severity,
				Rule:     msg.Symbol,
				Message:  msg.Message,
			})
		}
	}
	
	return issues, nil
}

// mapPylintTypeToSeverity maps Pylint message types to severity levels
func (p *PylintParser) mapPylintTypeToSeverity(msgType string) string {
	switch msgType {
	case "error", "fatal":
		return "error"
	case "warning":
		return "warning"
	case "convention", "refactor":
		return "info"
	case "info":
		return "info"
	default:
		return "warning"
	}
}

// SupportsFormat returns true for JSON formats
func (p *PylintParser) SupportsFormat(format string) bool {
	return format == "json" || format == "pylint-json" || format == "application/json"
}

func init() {
	// Register Pylint parser with the default registry
	Register(NewPylintParser())
}