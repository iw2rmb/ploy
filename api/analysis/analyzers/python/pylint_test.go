package python

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/api/analysis"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPylintAnalyzer(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	assert.NotNil(t, analyzer)
	assert.NotNil(t, analyzer.config)
	assert.NotNil(t, analyzer.logger)
	assert.Equal(t, "pylint", analyzer.config.PylintPath)
}

func TestPylintAnalyzer_GetSupportedFileTypes(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	fileTypes := analyzer.GetSupportedFileTypes()
	assert.Contains(t, fileTypes, ".py")
	assert.Contains(t, fileTypes, ".pyw")
}

func TestPylintAnalyzer_GetAnalyzerInfo(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	info := analyzer.GetAnalyzerInfo()
	assert.Equal(t, "pylint", info.Name)
	assert.Equal(t, "python", info.Language)
	assert.Contains(t, info.Description, "Python")
	assert.Greater(t, len(info.Capabilities), 0)
}

func TestPylintAnalyzer_ValidateConfiguration(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	tests := []struct {
		name    string
		config  interface{}
		wantErr bool
	}{
		{
			name: "valid PylintConfig",
			config: &PylintConfig{
				Enabled:     true,
				PylintPath:  "pylint",
				MinScore:    7.0,
			},
			wantErr: false,
		},
		{
			name: "valid map config",
			config: map[string]interface{}{
				"enabled":      true,
				"pylint_path":  "pylint",
				"min_score":    7.0,
			},
			wantErr: false,
		},
		{
			name:    "invalid config type",
			config:  "invalid",
			wantErr: true,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: false, // Should use defaults
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := analyzer.ValidateConfiguration(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPylintAnalyzer_Configure(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	config := &PylintConfig{
		Enabled:      true,
		PylintPath:   "/custom/pylint",
		MinScore:     8.0,
		DisableRules: []string{"C0111", "R0903"},
	}
	
	err := analyzer.Configure(config)
	assert.NoError(t, err)
	assert.Equal(t, "/custom/pylint", analyzer.config.PylintPath)
	assert.Equal(t, 8.0, analyzer.config.MinScore)
	assert.Contains(t, analyzer.config.DisableRules, "C0111")
}

func TestPylintAnalyzer_parsePylintOutput(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	// Sample Pylint JSON output
	pylintOutput := `[
		{
			"type": "error",
			"module": "test_module",
			"obj": "TestClass",
			"line": 10,
			"column": 4,
			"path": "test.py",
			"symbol": "syntax-error",
			"message": "Syntax error in test.py",
			"message-id": "E0001"
		},
		{
			"type": "warning",
			"module": "test_module",
			"obj": "function",
			"line": 25,
			"column": 8,
			"path": "test.py",
			"symbol": "unused-variable",
			"message": "Unused variable 'x'",
			"message-id": "W0612"
		},
		{
			"type": "convention",
			"module": "test_module",
			"obj": "",
			"line": 1,
			"column": 0,
			"path": "test.py",
			"symbol": "missing-docstring",
			"message": "Missing module docstring",
			"message-id": "C0111"
		}
	]`
	
	issues := analyzer.parsePylintOutput(pylintOutput)
	
	require.Len(t, issues, 3)
	
	// Check first issue (error)
	assert.Equal(t, "syntax-error", issues[0].RuleName)
	assert.Equal(t, analysis.SeverityHigh, issues[0].Severity)
	assert.Equal(t, "test.py", issues[0].File)
	assert.Equal(t, 10, issues[0].Line)
	assert.Equal(t, 4, issues[0].Column)
	assert.Contains(t, issues[0].Message, "Syntax error")
	
	// Check second issue (warning)
	assert.Equal(t, "unused-variable", issues[1].RuleName)
	assert.Equal(t, analysis.SeverityMedium, issues[1].Severity)
	assert.Equal(t, 25, issues[1].Line)
	
	// Check third issue (convention)
	assert.Equal(t, "missing-docstring", issues[2].RuleName)
	assert.Equal(t, analysis.SeverityLow, issues[2].Severity)
}

func TestPylintAnalyzer_mapSeverity(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	tests := []struct {
		pylintSeverity string
		expected       analysis.SeverityLevel
	}{
		{"fatal", analysis.SeverityCritical},
		{"error", analysis.SeverityHigh},
		{"warning", analysis.SeverityMedium},
		{"convention", analysis.SeverityLow},
		{"refactor", analysis.SeverityLow},
		{"info", analysis.SeverityInfo},
		{"unknown", analysis.SeverityInfo},
	}
	
	for _, tt := range tests {
		t.Run(tt.pylintSeverity, func(t *testing.T) {
			severity := analyzer.mapSeverity(tt.pylintSeverity)
			assert.Equal(t, tt.expected, severity)
		})
	}
}

func TestPylintAnalyzer_categorizeMessage(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	tests := []struct {
		messageID string
		expected  analysis.IssueCategory
	}{
		{"E0001", analysis.CategoryBug},          // Error
		{"W0612", analysis.CategoryDeprecation},  // Warning - W06xx is deprecation
		{"C0111", analysis.CategoryStyle},        // Convention
		{"R0903", analysis.CategoryComplexity},   // Refactor
		{"I0011", analysis.CategoryStyle},        // Information
		{"F0001", analysis.CategoryBug},          // Fatal
		{"X0000", analysis.CategoryMaintenance},  // Unknown
	}
	
	for _, tt := range tests {
		t.Run(tt.messageID, func(t *testing.T) {
			category := analyzer.categorizeMessage(tt.messageID)
			assert.Equal(t, tt.expected, category)
		})
	}
}

func TestPylintAnalyzer_findPythonFiles(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	codebase := analysis.Codebase{
		Files: []string{
			"main.py",
			"test.py",
			"src/module.py",
			"tests/test_module.py",
			"script.pyw",
			"docs/README.md",
			"requirements.txt",
			"setup.py",
			"__pycache__/cached.pyc",
		},
	}
	
	pythonFiles := analyzer.findPythonFiles(codebase)
	
	assert.Len(t, pythonFiles, 6) // Including script.pyw
	assert.Contains(t, pythonFiles, "main.py")
	assert.Contains(t, pythonFiles, "test.py")
	assert.Contains(t, pythonFiles, "src/module.py")
	assert.Contains(t, pythonFiles, "tests/test_module.py")
	assert.Contains(t, pythonFiles, "script.pyw")
	assert.Contains(t, pythonFiles, "setup.py")
	assert.NotContains(t, pythonFiles, "__pycache__/cached.pyc")
	assert.NotContains(t, pythonFiles, "docs/README.md")
}

func TestPylintAnalyzer_detectPythonProject(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	tests := []struct {
		name     string
		files    []string
		expected string
	}{
		{
			name:     "pip project",
			files:    []string{"requirements.txt", "main.py"},
			expected: "pip",
		},
		{
			name:     "poetry project",
			files:    []string{"pyproject.toml", "poetry.lock", "main.py"},
			expected: "poetry",
		},
		{
			name:     "setuptools project",
			files:    []string{"setup.py", "setup.cfg", "main.py"},
			expected: "setuptools",
		},
		{
			name:     "conda project",
			files:    []string{"environment.yml", "main.py"},
			expected: "conda",
		},
		{
			name:     "standalone project",
			files:    []string{"main.py", "test.py"},
			expected: "standalone",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock codebase with specific files
			codebase := analysis.Codebase{
				RootPath: "/tmp/test",
				Files:    tt.files,
			}
			
			projectType := analyzer.detectPythonProject(codebase)
			assert.Equal(t, tt.expected, projectType)
		})
	}
}

func TestPylintAnalyzer_GenerateFixSuggestions(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	tests := []struct {
		name        string
		issue       analysis.Issue
		expectFix   bool
		expectCount int
	}{
		{
			name: "unused variable",
			issue: analysis.Issue{
				RuleName: "unused-variable",
				Message:  "Unused variable 'x'",
			},
			expectFix:   true,
			expectCount: 1,
		},
		{
			name: "missing docstring",
			issue: analysis.Issue{
				RuleName: "missing-docstring",
				Message:  "Missing module docstring",
			},
			expectFix:   true,
			expectCount: 1,
		},
		{
			name: "syntax error",
			issue: analysis.Issue{
				RuleName: "syntax-error",
				Message:  "Syntax error",
			},
			expectFix:   false,
			expectCount: 0,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestions, err := analyzer.GenerateFixSuggestions(tt.issue)
			assert.NoError(t, err)
			
			if tt.expectFix {
				assert.Len(t, suggestions, tt.expectCount)
				if len(suggestions) > 0 {
					assert.NotEmpty(t, suggestions[0].Description)
					assert.Greater(t, suggestions[0].Confidence, 0.0)
				}
			} else {
				assert.Empty(t, suggestions)
			}
		})
	}
}

func TestPylintAnalyzer_CanAutoFix(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	tests := []struct {
		name     string
		issue    analysis.Issue
		expected bool
	}{
		{
			name:     "unused import",
			issue:    analysis.Issue{RuleName: "unused-import"},
			expected: true,
		},
		{
			name:     "unused variable",
			issue:    analysis.Issue{RuleName: "unused-variable"},
			expected: true,
		},
		{
			name:     "missing docstring",
			issue:    analysis.Issue{RuleName: "missing-docstring"},
			expected: true,
		},
		{
			name:     "trailing whitespace",
			issue:    analysis.Issue{RuleName: "trailing-whitespace"},
			expected: true,
		},
		{
			name:     "syntax error",
			issue:    analysis.Issue{RuleName: "syntax-error"},
			expected: false,
		},
		{
			name:     "import error",
			issue:    analysis.Issue{RuleName: "import-error"},
			expected: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canFix := analyzer.CanAutoFix(tt.issue)
			assert.Equal(t, tt.expected, canFix)
		})
	}
}

func TestPylintAnalyzer_GetARFRecipes(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	tests := []struct {
		name         string
		issue        analysis.Issue
		expectRecipe bool
	}{
		{
			name:         "unused import",
			issue:        analysis.Issue{RuleName: "unused-import"},
			expectRecipe: true,
		},
		{
			name:         "unused variable",
			issue:        analysis.Issue{RuleName: "unused-variable"},
			expectRecipe: true,
		},
		{
			name:         "missing docstring",
			issue:        analysis.Issue{RuleName: "missing-docstring"},
			expectRecipe: true,
		},
		{
			name:         "unknown issue",
			issue:        analysis.Issue{RuleName: "unknown-rule"},
			expectRecipe: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recipes := analyzer.GetARFRecipes(tt.issue)
			
			if tt.expectRecipe {
				assert.NotEmpty(t, recipes)
				for _, recipe := range recipes {
					assert.True(t, strings.HasPrefix(recipe, "org.openrewrite.python.") ||
						strings.HasPrefix(recipe, "com.ploy.python."))
				}
			} else {
				assert.Empty(t, recipes)
			}
		})
	}
}

func TestPylintAnalyzer_Analyze_EmptyCodebase(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	ctx := context.Background()
	codebase := analysis.Codebase{
		RootPath: "/tmp/empty",
		Files:    []string{},
	}
	
	result, err := analyzer.Analyze(ctx, codebase)
	
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "python", result.Language)
	assert.Equal(t, "pylint", result.Analyzer)
	assert.True(t, result.Success)
	assert.Empty(t, result.Issues)
}

func TestPylintAnalyzer_Analyze_Timeout(t *testing.T) {
	logger := logrus.New()
	analyzer := NewPylintAnalyzer(logger)
	
	// Create a context that times out immediately
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	
	// Sleep to ensure context is cancelled
	time.Sleep(10 * time.Millisecond)
	
	codebase := analysis.Codebase{
		RootPath: "/tmp/test",
		Files:    []string{"test.py"},
	}
	
	result, err := analyzer.Analyze(ctx, codebase)
	
	// Should handle timeout gracefully
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "context")
}