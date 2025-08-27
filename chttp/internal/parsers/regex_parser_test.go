package parsers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegexParser_ParseSimplePattern(t *testing.T) {
	parser := NewRegexParser("test-regex")
	
	// Add a simple error pattern
	err := parser.AddPattern("error", `(\S+):(\d+): error: (.+)`, "error", 
		[]string{"file", "line", "message"})
	require.NoError(t, err)
	
	// Test parsing matching output
	output := `test.py:10: error: undefined variable
main.py:20: error: syntax error`
	
	issues, err := parser.ParseOutput(output, "", 0)
	require.NoError(t, err)
	assert.Len(t, issues, 2)
	
	assert.Equal(t, "test.py", issues[0].File)
	assert.Equal(t, 10, issues[0].Line)
	assert.Equal(t, "undefined variable", issues[0].Message)
	assert.Equal(t, "error", issues[0].Severity)
	
	assert.Equal(t, "main.py", issues[1].File)
	assert.Equal(t, 20, issues[1].Line)
	assert.Equal(t, "syntax error", issues[1].Message)
}

func TestRegexParser_ParseWithNamedGroups(t *testing.T) {
	parser := NewRegexParser("named-groups")
	
	// Add pattern with named groups
	err := parser.AddPattern("warning", 
		`(?P<file>\S+):(?P<line>\d+):(?P<column>\d+): warning: (?P<message>.+)`,
		"warning", nil)
	require.NoError(t, err)
	
	output := "file.go:100:5: warning: unused variable"
	
	issues, err := parser.ParseOutput(output, "", 0)
	require.NoError(t, err)
	assert.Len(t, issues, 1)
	
	assert.Equal(t, "file.go", issues[0].File)
	assert.Equal(t, 100, issues[0].Line)
	assert.Equal(t, 5, issues[0].Column)
	assert.Equal(t, "unused variable", issues[0].Message)
	assert.Equal(t, "warning", issues[0].Severity)
}

func TestRegexParser_MultiplePatterns(t *testing.T) {
	parser := NewRegexParser("multi-pattern")
	
	// Add multiple patterns with different severities
	parser.AddPattern("error", `ERROR: \[(\S+):(\d+)\] (.+)`, "error",
		[]string{"file", "line", "message"})
	parser.AddPattern("warning", `WARNING: (.+) at (\S+):(\d+)`, "warning",
		[]string{"message", "file", "line"})
	parser.AddPattern("info", `INFO: (.+)`, "info",
		[]string{"message"})
	
	output := `ERROR: [main.py:15] undefined function
WARNING: deprecated usage at utils.py:30
INFO: Analysis complete
ERROR: [test.py:5] missing import`
	
	issues, err := parser.ParseOutput(output, "", 0)
	require.NoError(t, err)
	assert.Len(t, issues, 4)
	
	// Check error issues
	errorIssues := filterBySeverity(issues, "error")
	assert.Len(t, errorIssues, 2)
	assert.Equal(t, "main.py", errorIssues[0].File)
	assert.Equal(t, 15, errorIssues[0].Line)
	
	// Check warning issues
	warningIssues := filterBySeverity(issues, "warning")
	assert.Len(t, warningIssues, 1)
	assert.Equal(t, "utils.py", warningIssues[0].File)
	assert.Equal(t, 30, warningIssues[0].Line)
	
	// Check info issues
	infoIssues := filterBySeverity(issues, "info")
	assert.Len(t, infoIssues, 1)
	assert.Equal(t, "Analysis complete", infoIssues[0].Message)
}

func TestRegexParser_ComplexPatterns(t *testing.T) {
	parser := NewRegexParser("complex")
	
	// ESLint-style pattern
	parser.AddPattern("eslint",
		`^\s*(\S+)\s+(\d+):(\d+)\s+(error|warning)\s+(.+?)\s+(\S+)$`,
		"", // Use captured severity
		[]string{"file", "line", "column", "severity", "message", "rule"})
	
	output := `  src/app.js  10:5  error    'foo' is not defined  no-undef
  src/app.js  15:10  warning  Missing semicolon     semi`
	
	issues, err := parser.ParseOutput(output, "", 0)
	require.NoError(t, err)
	assert.Len(t, issues, 2)
	
	assert.Equal(t, "src/app.js", issues[0].File)
	assert.Equal(t, 10, issues[0].Line)
	assert.Equal(t, 5, issues[0].Column)
	assert.Equal(t, "error", issues[0].Severity)
	assert.Equal(t, "'foo' is not defined", issues[0].Message)
	assert.Equal(t, "no-undef", issues[0].Rule)
	
	assert.Equal(t, "warning", issues[1].Severity)
	assert.Equal(t, "semi", issues[1].Rule)
}

func TestRegexParser_StderrParsing(t *testing.T) {
	parser := NewRegexParser("stderr")
	
	parser.AddPattern("error", `Error: (.+)`, "error", []string{"message"})
	
	// Test parsing stderr
	issues, err := parser.ParseOutput("", "Error: compilation failed", 1)
	require.NoError(t, err)
	assert.Len(t, issues, 1)
	assert.Equal(t, "compilation failed", issues[0].Message)
	assert.Equal(t, "error", issues[0].Severity)
}

func TestRegexParser_InvalidPattern(t *testing.T) {
	parser := NewRegexParser("invalid")
	
	// Test invalid regex pattern
	err := parser.AddPattern("bad", `[invalid(regex`, "error", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid regex")
}

func TestRegexParser_EmptyOutput(t *testing.T) {
	parser := NewRegexParser("empty")
	parser.AddPattern("error", `ERROR: (.+)`, "error", []string{"message"})
	
	issues, err := parser.ParseOutput("", "", 0)
	require.NoError(t, err)
	assert.Len(t, issues, 0)
}

func TestRegexParser_ConfigureFromOptions(t *testing.T) {
	parser := NewRegexParser("configurable")
	
	options := map[string]interface{}{
		"patterns": []map[string]interface{}{
			{
				"name":     "error",
				"pattern":  `ERROR: (.+)`,
				"severity": "error",
				"groups":   []string{"message"},
			},
			{
				"name":     "warning",
				"pattern":  `WARN: (.+) at line (\d+)`,
				"severity": "warning",
				"groups":   []string{"message", "line"},
			},
		},
	}
	
	err := parser.Configure(options)
	require.NoError(t, err)
	
	output := `ERROR: critical failure
WARN: deprecated function at line 50`
	
	issues, err := parser.ParseOutput(output, "", 0)
	require.NoError(t, err)
	assert.Len(t, issues, 2)
	assert.Equal(t, "critical failure", issues[0].Message)
	assert.Equal(t, "deprecated function", issues[1].Message)
	assert.Equal(t, 50, issues[1].Line)
}

func TestRegexParser_LineColumnExtraction(t *testing.T) {
	parser := NewRegexParser("location")
	
	// Pattern that extracts all location information
	parser.AddPattern("location",
		`File "([^"]+)", line (\d+), col (\d+): (.+)`,
		"error",
		[]string{"file", "line", "column", "message"})
	
	output := `File "module/test.py", line 25, col 10: undefined variable 'x'`
	
	issues, err := parser.ParseOutput(output, "", 0)
	require.NoError(t, err)
	assert.Len(t, issues, 1)
	assert.Equal(t, "module/test.py", issues[0].File)
	assert.Equal(t, 25, issues[0].Line)
	assert.Equal(t, 10, issues[0].Column)
	assert.Equal(t, "undefined variable 'x'", issues[0].Message)
}

func TestRegexParser_RuleExtraction(t *testing.T) {
	parser := NewRegexParser("rules")
	
	// Pattern that includes rule/code information
	parser.AddPattern("violation",
		`(\S+):(\d+): \[(\w+)\] (.+)`,
		"error",
		[]string{"file", "line", "rule", "message"})
	
	output := "config.yaml:10: [E001] invalid indentation"
	
	issues, err := parser.ParseOutput(output, "", 0)
	require.NoError(t, err)
	assert.Len(t, issues, 1)
	assert.Equal(t, "E001", issues[0].Rule)
	assert.Equal(t, "invalid indentation", issues[0].Message)
}

// Helper function to filter issues by severity
func filterBySeverity(issues []Issue, severity string) []Issue {
	var filtered []Issue
	for _, issue := range issues {
		if issue.Severity == severity {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}