package analyzers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPylintParser(t *testing.T) {
	parser := NewPylintParser()
	
	assert.NotNil(t, parser)
	assert.Equal(t, "pylint_json", parser.GetName())
}

func TestPylintParser_ParseOutput(t *testing.T) {
	parser := NewPylintParser()
	
	tests := []struct {
		name           string
		stdout         string
		stderr         string
		exitCode       int
		expectedIssues int
		expectedError  bool
	}{
		{
			name:           "valid pylint json output",
			stdout:         `[{"type": "error", "module": "test", "obj": "", "line": 1, "column": 0, "path": "test.py", "symbol": "syntax-error", "message": "invalid syntax", "message-id": "E0001"}]`,
			stderr:         "",
			exitCode:       1,
			expectedIssues: 1,
			expectedError:  false,
		},
		{
			name:           "multiple issues",
			stdout:         `[{"type": "warning", "module": "test", "obj": "", "line": 5, "column": 0, "path": "main.py", "symbol": "unused-import", "message": "Unused import 'os'", "message-id": "W0611"}, {"type": "convention", "module": "test", "obj": "", "line": 10, "column": 0, "path": "main.py", "symbol": "missing-docstring", "message": "Missing module docstring", "message-id": "C0111"}]`,
			stderr:         "",
			exitCode:       1,
			expectedIssues: 2,
			expectedError:  false,
		},
		{
			name:           "empty output",
			stdout:         "",
			stderr:         "",
			exitCode:       0,
			expectedIssues: 0,
			expectedError:  false,
		},
		{
			name:           "invalid json",
			stdout:         "invalid json output",
			stderr:         "",
			exitCode:       1,
			expectedIssues: 0,
			expectedError:  false, // Should handle gracefully
		},
		{
			name:           "execution error with stderr",
			stdout:         "",
			stderr:         "pylint: command not found",
			exitCode:       127,
			expectedIssues: 1, // Should create an execution error issue
			expectedError:  false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues, err := parser.ParseOutput(tt.stdout, tt.stderr, tt.exitCode)
			
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			assert.Len(t, issues, tt.expectedIssues)
			
			// Validate issue format for non-empty results
			if len(issues) > 0 && tt.stdout != "" && tt.exitCode != 127 {
				issue := issues[0]
				assert.NotEmpty(t, issue.File)
				assert.NotEmpty(t, issue.Message)
				assert.NotEmpty(t, issue.Rule)
				assert.NotEmpty(t, issue.Severity)
				assert.Greater(t, issue.Line, 0)
			}
		})
	}
}

func TestPylintParser_MapSeverity(t *testing.T) {
	parser := NewPylintParser()
	
	tests := []struct {
		pylintType string
		expected   string
	}{
		{"fatal", "error"},
		{"error", "error"},
		{"warning", "warning"},
		{"convention", "info"},
		{"refactor", "info"},
		{"info", "info"},
		{"unknown", "info"},
	}
	
	for _, tt := range tests {
		t.Run(tt.pylintType, func(t *testing.T) {
			severity := parser.mapSeverity(tt.pylintType)
			assert.Equal(t, tt.expected, severity)
		})
	}
}

func TestPylintParser_IsARFCompatible(t *testing.T) {
	parser := NewPylintParser()
	
	tests := []struct {
		ruleName string
		expected bool
	}{
		{"unused-import", true},
		{"unused-variable", true},
		{"missing-docstring", true},
		{"trailing-whitespace", true},
		{"syntax-error", false},
		{"duplicate-code", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.ruleName, func(t *testing.T) {
			compatible := parser.isARFCompatible(tt.ruleName)
			assert.Equal(t, tt.expected, compatible)
		})
	}
}

func TestPylintParser_IntegrationWithServer(t *testing.T) {
	// Test that the Pylint parser integrates properly with the CHTTP server
	parser := NewPylintParser()
	
	// Simulate server calling parser
	stdout := `[{"type": "warning", "module": "test", "obj": "", "line": 1, "column": 0, "path": "test.py", "symbol": "unused-import", "message": "Unused import", "message-id": "W0611"}]`
	
	issues, err := parser.ParseOutput(stdout, "", 1)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	
	issue := issues[0]
	
	// Verify issue format matches server.Issue structure expectations
	assert.Equal(t, "test.py", issue.File)
	assert.Equal(t, 1, issue.Line)
	assert.Equal(t, "warning", issue.Severity)
	assert.Equal(t, "unused-import", issue.Rule)
	assert.Equal(t, "Unused import", issue.Message)
}