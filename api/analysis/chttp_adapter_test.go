package analysis

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/chttp"
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

func TestCHTTPPylintAnalyzer_createCodebaseArchive(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	analyzer := NewCHTTPPylintAnalyzer("https://test.com", "test-client", privateKey)

	// Create temporary test directory
	tempDir := t.TempDir()
	
	// Create Python files
	pythonFile1 := filepath.Join(tempDir, "main.py")
	err = os.WriteFile(pythonFile1, []byte(`
import os
def hello():
    print("Hello, world!")
`), 0644)
	require.NoError(t, err)

	pythonFile2 := filepath.Join(tempDir, "utils.py")
	err = os.WriteFile(pythonFile2, []byte(`
def process_data(data):
    return data.strip()
`), 0644)
	require.NoError(t, err)

	// Create non-Python file (should be ignored)
	textFile := filepath.Join(tempDir, "README.txt")
	err = os.WriteFile(textFile, []byte("This is a readme file"), 0644)
	require.NoError(t, err)

	// Create codebase
	codebase := Codebase{
		RootPath: tempDir,
		Files:    []string{"main.py", "utils.py", "README.txt"},
	}

	// Test archive creation
	archiveData, err := analyzer.createCodebaseArchive(codebase)
	assert.NoError(t, err)
	assert.NotEmpty(t, archiveData)
}

func TestCHTTPPylintAnalyzer_createCodebaseArchive_NoPythonFiles(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	analyzer := NewCHTTPPylintAnalyzer("https://test.com", "test-client", privateKey)

	// Create temporary test directory with no Python files
	tempDir := t.TempDir()
	
	// Create non-Python file
	textFile := filepath.Join(tempDir, "README.txt")
	err = os.WriteFile(textFile, []byte("This is a readme file"), 0644)
	require.NoError(t, err)

	// Create codebase with no Python files
	codebase := Codebase{
		RootPath: tempDir,
		Files:    []string{"README.txt"},
	}

	// Test archive creation should fail
	archiveData, err := analyzer.createCodebaseArchive(codebase)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no Python files found")
	assert.Nil(t, archiveData)
}

func TestCHTTPPylintAnalyzer_AnalyzeWithRealArchive(t *testing.T) {
	// Create mock CHTTP server that validates archive structure
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/analyze", r.URL.Path)
		assert.Equal(t, "application/gzip", r.Header.Get("Content-Type"))
		assert.NotEmpty(t, r.Header.Get("X-Client-ID"))
		assert.NotEmpty(t, r.Header.Get("X-Signature"))

		// Return mock analysis result
		result := &chttp.AnalysisResult{
			ID:        "test-analysis-123",
			Status:    "success",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Result: chttp.Result{
				Issues: []chttp.Issue{
					{
						File:     "main.py",
						Line:     2,
						Column:   1,
						Severity: "warning",
						Rule:     "unused-import",
						Message:  "Unused import: os",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer mockServer.Close()

	// Create analyzer with mock server
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	analyzer := NewCHTTPPylintAnalyzer(mockServer.URL, "test-client", privateKey)

	// Create temporary test directory
	tempDir := t.TempDir()
	
	// Create Python files with real content
	pythonFile := filepath.Join(tempDir, "main.py")
	err = os.WriteFile(pythonFile, []byte(`
import os  # unused import
def hello():
    print("Hello, world!")
    return "success"
`), 0644)
	require.NoError(t, err)

	// Create codebase
	codebase := Codebase{
		RootPath: tempDir,
		Files:    []string{"main.py"},
	}

	// Test analysis with real archive creation
	ctx := context.Background()
	result, err := analyzer.Analyze(ctx, codebase)
	
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "python", result.Language)
	assert.Equal(t, "pylint-chttp", result.Analyzer)
	assert.True(t, result.Success)
	assert.Len(t, result.Issues, 1)

	// Validate issue details
	issue := result.Issues[0]
	assert.Equal(t, "main.py", issue.File)
	assert.Equal(t, 2, issue.Line)
	assert.Equal(t, "unused-import", issue.RuleName)
	assert.Equal(t, SeverityMedium, issue.Severity)
	assert.True(t, issue.ARFCompatible)
}

func TestCHTTPPylintAnalyzer_ErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectError    bool
		errorContains  string
	}{
		{
			name: "server error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal server error"))
			},
			expectError:   true,
			errorContains: "server returned status 500",
		},
		{
			name: "invalid JSON response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("invalid json"))
			},
			expectError:   true,
			errorContains: "failed to parse response",
		},
		{
			name: "analysis failed status",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				response := &chttp.AnalysisResult{
					ID:     "test-123",
					Status: "failed",
					Error:  "Analysis service error",
				}
				json.NewEncoder(w).Encode(response)
			},
			expectError:   false, // We don't error on failed analysis, just return the result
			errorContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			mockServer := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer mockServer.Close()

			// Create analyzer
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			require.NoError(t, err)
			analyzer := NewCHTTPPylintAnalyzer(mockServer.URL, "test-client", privateKey)

			// Create test codebase
			tempDir := t.TempDir()
			pythonFile := filepath.Join(tempDir, "main.py")
			err = os.WriteFile(pythonFile, []byte("print('hello')"), 0644)
			require.NoError(t, err)

			codebase := Codebase{
				RootPath: tempDir,
				Files:    []string{"main.py"},
			}

			// Test analysis
			ctx := context.Background()
			result, err := analyzer.Analyze(ctx, codebase)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestCHTTPPylintAnalyzer_isARFCompatible(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	analyzer := NewCHTTPPylintAnalyzer("https://test.com", "test-client", privateKey)

	tests := []struct {
		ruleName string
		expected bool
	}{
		{"unused-import", true},
		{"unused-variable", true},
		{"missing-docstring", true},
		{"trailing-whitespace", true},
		{"line-too-long", true},
		{"bad-indentation", true},
		{"unknown-rule", false},
		{"complex-rule", false},
		{"syntax-error", false},
	}

	for _, tt := range tests {
		t.Run(tt.ruleName, func(t *testing.T) {
			result := analyzer.isARFCompatible(tt.ruleName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Benchmark for archive creation performance
func BenchmarkCHTTPPylintAnalyzer_createCodebaseArchive(b *testing.B) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	analyzer := NewCHTTPPylintAnalyzer("https://test.com", "test-client", privateKey)

	// Create test project
	tempDir := b.TempDir()
	
	// Create multiple Python files
	for i := 0; i < 10; i++ {
		filename := filepath.Join(tempDir, fmt.Sprintf("module_%d.py", i))
		content := fmt.Sprintf(`
import os
import sys

def function_%d():
    """Test function %d."""
    return "result_%d"

class Class_%d:
    def __init__(self):
        self.value = %d
    
    def method_%d(self):
        return self.value * 2
`, i, i, i, i, i, i)
		
		err := os.WriteFile(filename, []byte(content), 0644)
		if err != nil {
			b.Fatal(err)
		}
	}

	// Create file list
	files := make([]string, 10)
	for i := 0; i < 10; i++ {
		files[i] = fmt.Sprintf("module_%d.py", i)
	}

	codebase := Codebase{
		RootPath: tempDir,
		Files:    files,
	}

	// Benchmark archive creation
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := analyzer.createCodebaseArchive(codebase)
		if err != nil {
			b.Fatal(err)
		}
	}
}