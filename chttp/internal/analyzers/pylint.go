package analyzers

import (
	"encoding/json"
	"fmt"
)

// PylintParser implements output parsing for Pylint JSON format
type PylintParser struct {
	name string
}

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

// NewPylintParser creates a new Pylint output parser
func NewPylintParser() *PylintParser {
	return &PylintParser{
		name: "pylint_json",
	}
}

// GetName returns the parser name
func (p *PylintParser) GetName() string {
	return p.name
}

// ParseOutput parses Pylint JSON output into CHTTP issues
func (p *PylintParser) ParseOutput(stdout, stderr string, exitCode int) ([]Issue, error) {
	var issues []Issue
	
	// Handle execution errors first
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
	
	// Handle empty output (no issues found)
	if stdout == "" {
		return issues, nil
	}
	
	// Parse Pylint JSON output
	var messages []PylintMessage
	if err := json.Unmarshal([]byte(stdout), &messages); err != nil {
		// If JSON parsing fails, don't return error - just log and return empty
		// This allows the service to continue working even with malformed output
		return issues, nil
	}
	
	// Convert Pylint messages to CHTTP issues
	for _, msg := range messages {
		issue := Issue{
			File:     msg.Path,
			Line:     msg.Line,
			Column:   msg.Column,
			Severity: p.mapSeverity(msg.Type),
			Rule:     msg.Symbol,
			Message:  msg.Message,
		}
		
		issues = append(issues, issue)
	}
	
	return issues, nil
}

// mapSeverity maps Pylint message types to CHTTP severity levels
func (p *PylintParser) mapSeverity(pylintType string) string {
	switch pylintType {
	case "fatal", "error":
		return "error"
	case "warning":
		return "warning"
	case "convention", "refactor", "info":
		return "info"
	default:
		return "info"
	}
}

// isARFCompatible determines if an issue can be handled by ARF
func (p *PylintParser) isARFCompatible(ruleName string) bool {
	compatibleRules := map[string]bool{
		"unused-import":       true,
		"unused-variable":     true,
		"missing-docstring":   true,
		"trailing-whitespace": true,
		"line-too-long":       true,
		"bad-indentation":     true,
		"wrong-import-order":  true,
		"multiple-imports":    true,
	}
	
	return compatibleRules[ruleName]
}

// GetARFRecipes returns ARF recipes for an issue (for future ARF integration)
func (p *PylintParser) GetARFRecipes(ruleName string) []string {
	recipeMap := map[string][]string{
		"unused-import": {
			"org.openrewrite.python.cleanup.RemoveUnusedImports",
			"com.ploy.python.cleanup.OrganizeImports",
		},
		"unused-variable": {
			"org.openrewrite.python.cleanup.RemoveUnusedVariables",
		},
		"missing-docstring": {
			"com.ploy.python.style.AddMissingDocstrings",
		},
		"trailing-whitespace": {
			"org.openrewrite.python.format.RemoveTrailingWhitespace",
		},
		"wrong-import-order": {
			"org.openrewrite.python.cleanup.OrganizeImports",
		},
		"line-too-long": {
			"org.openrewrite.python.format.BreakLongLines",
		},
	}
	
	if recipes, exists := recipeMap[ruleName]; exists {
		return recipes
	}
	
	return []string{}
}