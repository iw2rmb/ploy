package parsers

import (
	"encoding/json"
	"fmt"
)

// BanditResult represents the Bandit JSON output structure
type BanditResult struct {
	Errors  []BanditError `json:"errors"`
	Results []BanditIssue `json:"results"`
	Metrics BanditMetrics `json:"metrics"`
}

// BanditError represents an error in Bandit execution
type BanditError struct {
	Filename string `json:"filename"`
	Reason   string `json:"reason"`
}

// BanditIssue represents a security issue found by Bandit
type BanditIssue struct {
	Code       string `json:"code"`
	Filename   string `json:"filename"`
	IssueText  string `json:"issue_text"`
	LineNumber int    `json:"line_number"`
	LineRange  []int  `json:"line_range"`
	Severity   string `json:"issue_severity"`
	Confidence string `json:"issue_confidence"`
	TestID     string `json:"test_id"`
	TestName   string `json:"test_name"`
}

// BanditMetrics contains scan metrics
type BanditMetrics struct {
	FilesScanned int `json:"_totals.files_scanned"`
	LinesScanned int `json:"_totals.lines_scanned"`
}

// BanditParser parses Bandit security scanner JSON output
type BanditParser struct {
	name string
}

// NewBanditParser creates a new Bandit output parser
func NewBanditParser() *BanditParser {
	return &BanditParser{
		name: "bandit",
	}
}

// Name returns the parser name
func (p *BanditParser) Name() string {
	return p.name
}

// ParseOutput parses Bandit JSON output into issues
func (p *BanditParser) ParseOutput(stdout, stderr string, exitCode int) ([]Issue, error) {
	var issues []Issue
	
	// Handle execution errors
	if exitCode == 127 || (exitCode != 0 && stderr != "" && stdout == "") {
		issues = append(issues, Issue{
			File:     "unknown",
			Line:     0,
			Severity: "error",
			Rule:     "execution-error",
			Message:  fmt.Sprintf("Bandit execution failed: %s", stderr),
		})
		return issues, nil
	}
	
	// Parse JSON output
	if stdout != "" {
		var result BanditResult
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			return []Issue{{
				File:     "unknown",
				Line:     0,
				Severity: "error",
				Rule:     "parse-error",
				Message:  fmt.Sprintf("Failed to parse Bandit output: %v", err),
			}}, nil
		}
		
		// Convert errors to issues
		for _, error := range result.Errors {
			issues = append(issues, Issue{
				File:     error.Filename,
				Line:     0,
				Severity: "error",
				Rule:     "scan-error",
				Message:  error.Reason,
			})
		}
		
		// Convert security issues
		for _, issue := range result.Results {
			severity := p.mapBanditSeverity(issue.Severity, issue.Confidence)
			
			issues = append(issues, Issue{
				File:     issue.Filename,
				Line:     issue.LineNumber,
				Severity: severity,
				Rule:     issue.TestID,
				Message:  fmt.Sprintf("%s: %s", issue.TestName, issue.IssueText),
			})
		}
	}
	
	return issues, nil
}

// mapBanditSeverity maps Bandit severity and confidence to our severity levels
func (p *BanditParser) mapBanditSeverity(severity, confidence string) string {
	// Combine severity and confidence for mapping
	switch severity {
	case "HIGH":
		if confidence == "HIGH" || confidence == "MEDIUM" {
			return "error"
		}
		return "warning"
	case "MEDIUM":
		if confidence == "HIGH" {
			return "warning"
		}
		return "info"
	case "LOW":
		return "info"
	default:
		return "info"
	}
}

// SupportsFormat returns true for Bandit JSON formats
func (p *BanditParser) SupportsFormat(format string) bool {
	return format == "json" || format == "bandit-json"
}

func init() {
	// Register Bandit parser with the default registry
	Register(NewBanditParser())
}