package parsers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// RegexPattern represents a regex pattern for parsing output
type RegexPattern struct {
	Name     string
	Regex    *regexp.Regexp
	Severity string
	Groups   []string // Names of capture groups in order
}

// RegexParser parses output using configurable regex patterns
type RegexParser struct {
	name     string
	patterns []*RegexPattern
	options  map[string]interface{}
}

// NewRegexParser creates a new regex-based parser
func NewRegexParser(name string) *RegexParser {
	return &RegexParser{
		name:     name,
		patterns: make([]*RegexPattern, 0),
		options:  make(map[string]interface{}),
	}
}

// Name returns the parser name
func (p *RegexParser) Name() string {
	return p.name
}

// AddPattern adds a regex pattern to the parser
func (p *RegexParser) AddPattern(name, pattern, severity string, groups []string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern '%s': %w", pattern, err)
	}
	
	p.patterns = append(p.patterns, &RegexPattern{
		Name:     name,
		Regex:    re,
		Severity: severity,
		Groups:   groups,
	})
	
	return nil
}

// ParseOutput parses output using configured regex patterns
func (p *RegexParser) ParseOutput(stdout, stderr string, exitCode int) ([]Issue, error) {
	var issues []Issue
	
	// Parse both stdout and stderr
	for _, output := range []string{stdout, stderr} {
		if output == "" {
			continue
		}
		
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			
			// Try each pattern
			for _, pattern := range p.patterns {
				matches := pattern.Regex.FindStringSubmatch(line)
				if matches == nil {
					continue
				}
				
				issue := p.extractIssue(pattern, matches)
				if issue != nil {
					issues = append(issues, *issue)
				}
				break // Only use first matching pattern per line
			}
		}
	}
	
	return issues, nil
}

// extractIssue extracts an Issue from regex matches
func (p *RegexParser) extractIssue(pattern *RegexPattern, matches []string) *Issue {
	issue := &Issue{
		Severity: pattern.Severity,
	}
	
	// Check if we have named groups
	names := pattern.Regex.SubexpNames()
	hasNamedGroups := false
	for _, name := range names {
		if name != "" {
			hasNamedGroups = true
			break
		}
	}
	
	if hasNamedGroups {
		// Handle named groups
		for i, name := range names {
			if i == 0 || name == "" || i >= len(matches) {
				continue
			}
			p.setIssueField(issue, name, matches[i])
		}
	} else if pattern.Groups != nil && len(pattern.Groups) > 0 {
		// Handle positional groups with explicit names
		for i, groupName := range pattern.Groups {
			if i+1 >= len(matches) {
				break
			}
			p.setIssueField(issue, groupName, matches[i+1])
		}
	}
	
	// Override severity if captured
	if issue.Severity == "" && pattern.Severity != "" {
		issue.Severity = pattern.Severity
	}
	
	// Default severity to error if not set
	if issue.Severity == "" {
		issue.Severity = "error"
	}
	
	return issue
}

// setIssueField sets a field on the issue based on the group name
func (p *RegexParser) setIssueField(issue *Issue, fieldName, value string) {
	switch strings.ToLower(fieldName) {
	case "file", "filename", "path":
		issue.File = value
	case "line", "linenumber", "lineno":
		if line, err := strconv.Atoi(value); err == nil {
			issue.Line = line
		}
	case "column", "col", "colnumber":
		if col, err := strconv.Atoi(value); err == nil {
			issue.Column = col
		}
	case "severity", "level":
		issue.Severity = strings.ToLower(value)
	case "rule", "code", "ruleid":
		issue.Rule = value
	case "message", "msg", "description":
		issue.Message = value
	}
}

// SupportsFormat returns true for text formats
func (p *RegexParser) SupportsFormat(format string) bool {
	return format == "text" || format == "text/plain" || format == "regex"
}

// Configure sets parser options from a configuration map
func (p *RegexParser) Configure(options map[string]interface{}) error {
	p.options = options
	
	// Clear existing patterns
	p.patterns = make([]*RegexPattern, 0)
	
	// Load patterns from options
	if patternsRaw, ok := options["patterns"]; ok {
		patterns, ok := patternsRaw.([]map[string]interface{})
		if !ok {
			// Try to handle as []interface{} and convert
			if patternsArray, ok := patternsRaw.([]interface{}); ok {
				patterns = make([]map[string]interface{}, len(patternsArray))
				for i, p := range patternsArray {
					if patternMap, ok := p.(map[string]interface{}); ok {
						patterns[i] = patternMap
					}
				}
			} else {
				return fmt.Errorf("patterns must be an array of pattern configurations")
			}
		}
		
		for _, patternConfig := range patterns {
			name, _ := patternConfig["name"].(string)
			pattern, _ := patternConfig["pattern"].(string)
			severity, _ := patternConfig["severity"].(string)
			
			var groups []string
			if groupsRaw, ok := patternConfig["groups"]; ok {
				if groupsArray, ok := groupsRaw.([]string); ok {
					groups = groupsArray
				} else if groupsInterface, ok := groupsRaw.([]interface{}); ok {
					groups = make([]string, len(groupsInterface))
					for i, g := range groupsInterface {
						groups[i], _ = g.(string)
					}
				}
			}
			
			if pattern != "" {
				if err := p.AddPattern(name, pattern, severity, groups); err != nil {
					return fmt.Errorf("failed to add pattern '%s': %w", name, err)
				}
			}
		}
	}
	
	return nil
}

// GetOption retrieves a configuration option
func (p *RegexParser) GetOption(key string) interface{} {
	return p.options[key]
}

// CreateDefaultRegexParser creates a regex parser with common patterns
func CreateDefaultRegexParser() *RegexParser {
	parser := NewRegexParser("default-regex")
	
	// Common error patterns
	parser.AddPattern("gcc-style", 
		`^(\S+):(\d+):(\d+): (error|warning): (.+)$`,
		"", []string{"file", "line", "column", "severity", "message"})
	
	parser.AddPattern("python-style",
		`File "([^"]+)", line (\d+)(?:, in .+)?: (.+)`,
		"error", []string{"file", "line", "message"})
	
	parser.AddPattern("java-style",
		`^(\S+\.java):(\d+): (error|warning): (.+)$`,
		"", []string{"file", "line", "severity", "message"})
	
	return parser
}