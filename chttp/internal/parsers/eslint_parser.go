package parsers

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// ESLintResult represents the ESLint JSON output structure
type ESLintResult struct {
	FilePath         string           `json:"filePath"`
	Messages         []ESLintMessage  `json:"messages"`
	ErrorCount       int              `json:"errorCount"`
	WarningCount     int              `json:"warningCount"`
	FatalErrorCount  int              `json:"fatalErrorCount"`
	FixableErrorCount   int           `json:"fixableErrorCount"`
	FixableWarningCount int           `json:"fixableWarningCount"`
}

// ESLintMessage represents a single ESLint issue
type ESLintMessage struct {
	RuleID      string      `json:"ruleId"`
	Severity    int         `json:"severity"`
	Message     string      `json:"message"`
	Line        int         `json:"line"`
	Column      int         `json:"column"`
	NodeType    string      `json:"nodeType"`
	Source      string      `json:"source"`
	EndLine     int         `json:"endLine"`
	EndColumn   int         `json:"endColumn"`
	Fix         interface{} `json:"fix"`
	Suggestions interface{} `json:"suggestions"`
}

// ESLintParser parses ESLint JSON output
type ESLintParser struct {
	name string
}

// NewESLintParser creates a new ESLint output parser
func NewESLintParser() *ESLintParser {
	return &ESLintParser{
		name: "eslint",
	}
}

// Name returns the parser name
func (p *ESLintParser) Name() string {
	return p.name
}

// ParseOutput parses ESLint JSON output into issues
func (p *ESLintParser) ParseOutput(stdout, stderr string, exitCode int) ([]Issue, error) {
	var issues []Issue
	
	// Handle execution errors
	if exitCode == 127 || (exitCode != 0 && stderr != "" && stdout == "") {
		issues = append(issues, Issue{
			File:     "unknown",
			Line:     0,
			Severity: "error",
			Rule:     "execution-error",
			Message:  fmt.Sprintf("ESLint execution failed: %s", stderr),
		})
		return issues, nil
	}
	
	// Parse JSON output
	if stdout != "" {
		var results []ESLintResult
		if err := json.Unmarshal([]byte(stdout), &results); err != nil {
			return []Issue{{
				File:     "unknown",
				Line:     0,
				Severity: "error",
				Rule:     "parse-error",
				Message:  fmt.Sprintf("Failed to parse ESLint output: %v", err),
			}}, nil
		}
		
		// Convert ESLint results to issues
		for _, result := range results {
			// Use relative path if possible
			file := result.FilePath
			if absPath, err := filepath.Abs(file); err == nil {
				if relPath, err := filepath.Rel(".", absPath); err == nil {
					file = relPath
				}
			}
			
			for _, msg := range result.Messages {
				severity := p.mapESLintSeverity(msg.Severity)
				rule := msg.RuleID
				if rule == "" {
					rule = "eslint"
				}
				
				issues = append(issues, Issue{
					File:     file,
					Line:     msg.Line,
					Column:   msg.Column,
					Severity: severity,
					Rule:     rule,
					Message:  msg.Message,
				})
			}
		}
	}
	
	return issues, nil
}

// mapESLintSeverity maps ESLint severity levels to our severity levels
func (p *ESLintParser) mapESLintSeverity(severity int) string {
	switch severity {
	case 2:
		return "error"
	case 1:
		return "warning"
	default:
		return "info"
	}
}

// SupportsFormat returns true for ESLint JSON formats
func (p *ESLintParser) SupportsFormat(format string) bool {
	return format == "json" || format == "eslint-json"
}

func init() {
	// Register ESLint parser with the default registry
	Register(NewESLintParser())
}