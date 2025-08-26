package analysis

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCHTTPPylintAnalyzer(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	analyzer := NewCHTTPPylintAnalyzer("https://pylint.chttp.example.com", "test-client", privateKey)

	assert.NotNil(t, analyzer)
	assert.Equal(t, "https://pylint.chttp.example.com", analyzer.serviceURL)
	assert.NotNil(t, analyzer.client)
	
	info := analyzer.GetAnalyzerInfo()
	assert.Equal(t, "pylint-chttp", info.Name)
	assert.Equal(t, "python", info.Language)
	assert.Contains(t, info.Capabilities, "arf-integration")
}

func TestCHTTPPylintAnalyzer_GetSupportedFileTypes(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	analyzer := NewCHTTPPylintAnalyzer("https://test.com", "test-client", privateKey)
	
	fileTypes := analyzer.GetSupportedFileTypes()
	assert.Contains(t, fileTypes, ".py")
	assert.Contains(t, fileTypes, ".pyw")
}

func TestCHTTPPylintAnalyzer_SupportsCHTTP(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	analyzer := NewCHTTPPylintAnalyzer("https://test.com", "test-client", privateKey)
	
	assert.True(t, analyzer.SupportsCHTTP())
}

func TestCHTTPPylintAnalyzer_Analyze(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Mock CHTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "analysis-123",
			"status": "success",
			"result": {
				"issues": [
					{
						"file": "test.py",
						"line": 10,
						"column": 4,
						"severity": "error",
						"rule": "syntax-error", 
						"message": "Invalid syntax"
					},
					{
						"file": "main.py",
						"line": 5,
						"column": 0,
						"severity": "warning",
						"rule": "unused-import",
						"message": "Unused import 'os'"
					}
				]
			}
		}`))
	}))
	defer server.Close()

	analyzer := NewCHTTPPylintAnalyzer(server.URL, "test-client", privateKey)
	
	codebase := Codebase{
		Repository: Repository{
			ID:   "test-repo",
			Name: "test-python-project",
		},
		RootPath: "/tmp/test",
		Files:    []string{"test.py", "main.py"},
	}

	result, err := analyzer.Analyze(context.Background(), codebase)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "python", result.Language)
	assert.Equal(t, "pylint-chttp", result.Analyzer)
	assert.True(t, result.Success)
	assert.Len(t, result.Issues, 2)

	// Check first issue
	issue1 := result.Issues[0]
	assert.Equal(t, "test.py", issue1.File)
	assert.Equal(t, 10, issue1.Line)
	assert.Equal(t, SeverityHigh, issue1.Severity)
	assert.Equal(t, CategoryMaintenance, issue1.Category) // "syntax-error" maps to maintenance
	assert.Equal(t, "syntax-error", issue1.RuleName)

	// Check second issue
	issue2 := result.Issues[1]
	assert.Equal(t, "main.py", issue2.File)
	assert.Equal(t, 5, issue2.Line)
	assert.Equal(t, SeverityMedium, issue2.Severity)
	assert.Equal(t, "unused-import", issue2.RuleName)
	assert.True(t, issue2.ARFCompatible) // unused-import should be ARF compatible
}

func TestCHTTPPylintAnalyzer_GetARFRecipes(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	analyzer := NewCHTTPPylintAnalyzer("https://test.com", "test-client", privateKey)

	tests := []struct {
		name         string
		issue        Issue
		expectRecipe bool
	}{
		{
			name: "unused import",
			issue: Issue{
				RuleName: "unused-import",
			},
			expectRecipe: true,
		},
		{
			name: "unused variable", 
			issue: Issue{
				RuleName: "unused-variable",
			},
			expectRecipe: true,
		},
		{
			name: "missing docstring",
			issue: Issue{
				RuleName: "missing-docstring",
			},
			expectRecipe: true,
		},
		{
			name: "syntax error",
			issue: Issue{
				RuleName: "syntax-error",
			},
			expectRecipe: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recipes := analyzer.GetARFRecipes(tt.issue)
			
			if tt.expectRecipe {
				assert.NotEmpty(t, recipes)
			} else {
				assert.Empty(t, recipes)
			}
		})
	}
}

func TestCHTTPPylintAnalyzer_mapSeverity(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	analyzer := NewCHTTPPylintAnalyzer("https://test.com", "test-client", privateKey)

	tests := []struct {
		chttpSeverity string
		expected      SeverityLevel
	}{
		{"fatal", SeverityCritical},
		{"error", SeverityHigh},
		{"warning", SeverityMedium},
		{"convention", SeverityLow},
		{"refactor", SeverityLow},
		{"info", SeverityInfo},
		{"unknown", SeverityInfo},
	}

	for _, tt := range tests {
		t.Run(tt.chttpSeverity, func(t *testing.T) {
			severity := analyzer.mapSeverity(tt.chttpSeverity)
			assert.Equal(t, tt.expected, severity)
		})
	}
}

func TestCHTTPPylintAnalyzer_mapCategory(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	analyzer := NewCHTTPPylintAnalyzer("https://test.com", "test-client", privateKey)

	tests := []struct {
		ruleName string
		expected IssueCategory
	}{
		{"E0001", CategoryBug},          // Error
		{"W0612", CategoryDeprecation},  // Warning - W06xx is deprecation
		{"C0111", CategoryStyle},        // Convention
		{"R0903", CategoryComplexity},   // Refactor
		{"I0011", CategoryStyle},        // Information
		{"F0001", CategoryBug},          // Fatal
	}

	for _, tt := range tests {
		t.Run(tt.ruleName, func(t *testing.T) {
			category := analyzer.mapCategory(tt.ruleName)
			assert.Equal(t, tt.expected, category)
		})
	}
}