package parsers

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// GenericJSONParser parses JSON output using configurable field mappings
type GenericJSONParser struct {
	name        string
	fieldMap    map[string]string // Maps JSON fields to Issue fields
	issuesPath  string            // Path to issues array in JSON (e.g., "results", "violations")
	options     map[string]interface{}
}

// NewGenericJSONParser creates a new generic JSON parser
func NewGenericJSONParser(name string) *GenericJSONParser {
	return &GenericJSONParser{
		name: name,
		fieldMap: map[string]string{
			"file":     "file",
			"filename": "file",
			"path":     "file",
			"line":     "line",
			"lineno":   "line",
			"column":   "column",
			"col":      "column",
			"severity": "severity",
			"level":    "severity",
			"rule":     "rule",
			"code":     "rule",
			"message":  "message",
			"msg":      "message",
			"text":     "message",
		},
		options: make(map[string]interface{}),
	}
}

// Name returns the parser name
func (p *GenericJSONParser) Name() string {
	return p.name
}

// SetFieldMapping sets custom field mappings
func (p *GenericJSONParser) SetFieldMapping(jsonField, issueField string) {
	p.fieldMap[jsonField] = issueField
}

// SetIssuesPath sets the path to the issues array in the JSON
func (p *GenericJSONParser) SetIssuesPath(path string) {
	p.issuesPath = path
}

// ParseOutput parses generic JSON output into issues
func (p *GenericJSONParser) ParseOutput(stdout, stderr string, exitCode int) ([]Issue, error) {
	var issues []Issue
	
	if stdout == "" {
		return issues, nil
	}
	
	// Parse JSON
	var data interface{}
	if err := json.Unmarshal([]byte(stdout), &data); err != nil {
		return []Issue{{
			File:     "unknown",
			Line:     0,
			Severity: "error",
			Rule:     "parse-error",
			Message:  fmt.Sprintf("Failed to parse JSON output: %v", err),
		}}, nil
	}
	
	// Extract issues array
	var issuesData []interface{}
	
	if p.issuesPath != "" {
		// Navigate to issues array using path
		issuesData = p.navigateToArray(data, p.issuesPath)
	} else {
		// Try to auto-detect issues array
		issuesData = p.findIssuesArray(data)
	}
	
	// Convert to issues
	for _, item := range issuesData {
		if issueMap, ok := item.(map[string]interface{}); ok {
			issue := p.mapToIssue(issueMap)
			if issue.Message != "" || issue.File != "" {
				issues = append(issues, issue)
			}
		}
	}
	
	return issues, nil
}

// navigateToArray navigates JSON structure to find array at path
func (p *GenericJSONParser) navigateToArray(data interface{}, path string) []interface{} {
	// Simple path navigation (e.g., "results.issues")
	// For more complex paths, could use JSONPath library
	
	current := data
	if path == "" {
		if arr, ok := current.([]interface{}); ok {
			return arr
		}
		return nil
	}
	
	// For now, just check if data is a map and path is a key
	if m, ok := current.(map[string]interface{}); ok {
		if val, exists := m[path]; exists {
			if arr, ok := val.([]interface{}); ok {
				return arr
			}
		}
	}
	
	return nil
}

// findIssuesArray attempts to auto-detect the issues array
func (p *GenericJSONParser) findIssuesArray(data interface{}) []interface{} {
	// If data is already an array, use it
	if arr, ok := data.([]interface{}); ok {
		return arr
	}
	
	// If data is a map, look for common issue array keys
	if m, ok := data.(map[string]interface{}); ok {
		commonKeys := []string{"issues", "results", "violations", "errors", "warnings", "problems", "diagnostics"}
		for _, key := range commonKeys {
			if val, exists := m[key]; exists {
				if arr, ok := val.([]interface{}); ok {
					return arr
				}
			}
		}
		
		// If no common key found, look for any array value
		for _, val := range m {
			if arr, ok := val.([]interface{}); ok && len(arr) > 0 {
				// Check if it looks like issues (has relevant fields)
				if p.looksLikeIssue(arr[0]) {
					return arr
				}
			}
		}
	}
	
	return nil
}

// looksLikeIssue checks if an item looks like an issue object
func (p *GenericJSONParser) looksLikeIssue(item interface{}) bool {
	if m, ok := item.(map[string]interface{}); ok {
		// Check for common issue fields
		hasRelevantField := false
		for field := range m {
			if _, mapped := p.fieldMap[field]; mapped {
				hasRelevantField = true
				break
			}
		}
		return hasRelevantField
	}
	return false
}

// mapToIssue converts a JSON object to an Issue
func (p *GenericJSONParser) mapToIssue(data map[string]interface{}) Issue {
	issue := Issue{}
	
	for jsonField, value := range data {
		if issueField, ok := p.fieldMap[jsonField]; ok {
			p.setIssueFieldFromValue(&issue, issueField, value)
		}
	}
	
	// Set default severity if not set
	if issue.Severity == "" {
		issue.Severity = "warning"
	}
	
	return issue
}

// setIssueFieldFromValue sets an issue field from a JSON value
func (p *GenericJSONParser) setIssueFieldFromValue(issue *Issue, field string, value interface{}) {
	switch field {
	case "file":
		if s, ok := value.(string); ok {
			issue.File = s
		}
	case "line":
		issue.Line = p.toInt(value)
	case "column":
		issue.Column = p.toInt(value)
	case "severity":
		if s, ok := value.(string); ok {
			issue.Severity = s
		}
	case "rule":
		if s, ok := value.(string); ok {
			issue.Rule = s
		}
	case "message":
		if s, ok := value.(string); ok {
			issue.Message = s
		}
	}
}

// toInt converts various types to int
func (p *GenericJSONParser) toInt(value interface{}) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	case int64:
		return int(v)
	}
	return 0
}

// SupportsFormat returns true for JSON formats
func (p *GenericJSONParser) SupportsFormat(format string) bool {
	return format == "json" || format == "generic-json"
}

// Configure sets parser options
func (p *GenericJSONParser) Configure(options map[string]interface{}) error {
	p.options = options
	
	// Configure field mappings
	if mappings, ok := options["field_mappings"].(map[string]interface{}); ok {
		for jsonField, issueField := range mappings {
			if field, ok := issueField.(string); ok {
				p.fieldMap[jsonField] = field
			}
		}
	}
	
	// Configure issues path
	if path, ok := options["issues_path"].(string); ok {
		p.issuesPath = path
	}
	
	return nil
}

// GetOption retrieves a configuration option
func (p *GenericJSONParser) GetOption(key string) interface{} {
	return p.options[key]
}

func init() {
	// Register a default generic JSON parser
	Register(NewGenericJSONParser("generic-json"))
}