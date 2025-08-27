package parsers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParserRegistry_Register(t *testing.T) {
	registry := NewRegistry()
	
	// Test registering a parser
	parser := &mockParser{name: "test-parser"}
	err := registry.Register(parser)
	require.NoError(t, err)
	
	// Test duplicate registration should fail
	err = registry.Register(parser)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestParserRegistry_Get(t *testing.T) {
	registry := NewRegistry()
	
	// Register a parser
	parser := &mockParser{name: "test-parser"}
	registry.Register(parser)
	
	// Test getting existing parser
	retrieved, err := registry.Get("test-parser")
	require.NoError(t, err)
	assert.Equal(t, parser, retrieved)
	
	// Test getting non-existent parser
	_, err = registry.Get("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestParserRegistry_List(t *testing.T) {
	registry := NewRegistry()
	
	// Register multiple parsers
	parser1 := &mockParser{name: "parser1"}
	parser2 := &mockParser{name: "parser2"}
	
	registry.Register(parser1)
	registry.Register(parser2)
	
	// Test listing all parsers
	parsers := registry.List()
	assert.Len(t, parsers, 2)
	assert.Contains(t, parsers, "parser1")
	assert.Contains(t, parsers, "parser2")
}

func TestParserRegistry_AutoDetect(t *testing.T) {
	registry := NewRegistry()
	
	// Register parsers with format support
	jsonParser := &mockParser{
		name:    "json-parser",
		formats: []string{"json", "application/json"},
	}
	textParser := &mockParser{
		name:    "text-parser",
		formats: []string{"text", "text/plain"},
	}
	
	registry.Register(jsonParser)
	registry.Register(textParser)
	
	// Test auto-detection for JSON
	parser, err := registry.AutoDetect(`{"issues": []}`, "", 0)
	require.NoError(t, err)
	assert.Equal(t, jsonParser, parser)
	
	// Test auto-detection for plain text
	parser, err = registry.AutoDetect("ERROR: Something went wrong", "", 1)
	require.NoError(t, err)
	assert.Equal(t, textParser, parser)
	
	// Test when no parser matches (XML without XML parser)
	noMatchParser, err := registry.AutoDetect("<xml>data</xml>", "", 0)
	assert.Error(t, err)
	assert.Nil(t, noMatchParser)
	assert.Contains(t, err.Error(), "no XML parser found")
}

func TestParser_ParseOutput(t *testing.T) {
	tests := []struct {
		name         string
		stdout       string
		stderr       string
		exitCode     int
		expectedLen  int
		expectError  bool
	}{
		{
			name:        "successful parse",
			stdout:      `[{"file": "test.py", "line": 10, "message": "test issue"}]`,
			stderr:      "",
			exitCode:    0,
			expectedLen: 1,
			expectError: false,
		},
		{
			name:        "parse with warnings",
			stdout:      `[{"file": "test.py", "line": 10, "message": "warning"}]`,
			stderr:      "Warning: deprecated feature",
			exitCode:    0,
			expectedLen: 1,
			expectError: false,
		},
		{
			name:        "parse error",
			stdout:      "invalid json",
			stderr:      "Parse error",
			exitCode:    1,
			expectedLen: 0,
			expectError: true,
		},
		{
			name:        "empty output",
			stdout:      "",
			stderr:      "",
			exitCode:    0,
			expectedLen: 0,
			expectError: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := &mockParser{name: "test"}
			parser.parseFunc = func(stdout, stderr string, exitCode int) ([]Issue, error) {
				if tt.expectError {
					return nil, assert.AnError
				}
				
				var issues []Issue
				if tt.expectedLen > 0 {
					issues = append(issues, Issue{
						File:     "test.py",
						Line:     10,
						Message:  "test issue",
						Severity: "error",
					})
				}
				return issues, nil
			}
			
			issues, err := parser.ParseOutput(tt.stdout, tt.stderr, tt.exitCode)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, issues, tt.expectedLen)
			}
		})
	}
}

func TestCompositeParser(t *testing.T) {
	// Test parser that combines multiple parsers
	composite := NewCompositeParser(
		&mockParser{
			name: "parser1",
			parseFunc: func(stdout, stderr string, exitCode int) ([]Issue, error) {
				return []Issue{{File: "file1.py", Line: 1, Message: "issue1"}}, nil
			},
		},
		&mockParser{
			name: "parser2",
			parseFunc: func(stdout, stderr string, exitCode int) ([]Issue, error) {
				return []Issue{{File: "file2.py", Line: 2, Message: "issue2"}}, nil
			},
		},
	)
	
	issues, err := composite.ParseOutput("test", "", 0)
	require.NoError(t, err)
	assert.Len(t, issues, 2)
	assert.Equal(t, "file1.py", issues[0].File)
	assert.Equal(t, "file2.py", issues[1].File)
}

func TestParserWithOptions(t *testing.T) {
	parser := &mockParser{name: "configurable"}
	
	// Test setting options
	options := map[string]interface{}{
		"severity_threshold": "warning",
		"max_issues":         100,
		"include_info":       false,
	}
	
	// Since mockParser implements Configure, we can call it directly
	err := parser.Configure(options)
	assert.NoError(t, err)
	
	// Verify options were applied
	assert.Equal(t, "warning", parser.GetOption("severity_threshold"))
	assert.Equal(t, 100, parser.GetOption("max_issues"))
	assert.Equal(t, false, parser.GetOption("include_info"))
}

// Mock parser for testing
type mockParser struct {
	name      string
	formats   []string
	parseFunc func(stdout, stderr string, exitCode int) ([]Issue, error)
	options   map[string]interface{}
}

func (m *mockParser) Name() string {
	return m.name
}

func (m *mockParser) ParseOutput(stdout, stderr string, exitCode int) ([]Issue, error) {
	if m.parseFunc != nil {
		return m.parseFunc(stdout, stderr, exitCode)
	}
	return []Issue{}, nil
}

func (m *mockParser) SupportsFormat(format string) bool {
	for _, f := range m.formats {
		if f == format {
			return true
		}
	}
	return false
}

func (m *mockParser) Configure(options map[string]interface{}) error {
	m.options = options
	return nil
}

func (m *mockParser) GetOption(key string) interface{} {
	if m.options != nil {
		return m.options[key]
	}
	return nil
}