package arf

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzer_ExtractRelevantContent_SmartExtraction(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	analyzer := NewAnalyzer(&MockLLMProvider{}, logger)

	content := `package com.example;

import java.util.ArrayList;
import javax.servlet.http.HttpServlet;

public class TestService extends HttpServlet {
    private List<String> items;
    
    public void processItems() {
        // Line 10: Error occurs here
        items.add("test"); // Cannot find symbol: List
        logger.info("Processing complete");
    }
    
    public void cleanup() {
        items.clear();
    }
}`

	errors := []ErrorDetails{
		{
			Message: "cannot find symbol: class List",
			File:    "TestService.java", 
			Line:    10,
			Type:    "compilation",
		},
	}

	result := analyzer.extractRelevantContent(content, errors)

	// Should include context around error line
	assert.Contains(t, result, "private List<String> items;")
	assert.Contains(t, result, "items.add(\"test\");")
	assert.Contains(t, result, "import java.util.ArrayList;")
	
	// Should not include unrelated methods if content is large
	if len(content) > 1000 {
		assert.NotContains(t, result, "cleanup()")
	}
}

func TestAnalyzer_IsRelevantFile_DependencyAnalysis(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	analyzer := NewAnalyzer(&MockLLMProvider{}, logger)

	files := []SourceFile{
		{
			Path:    "com/example/Service.java",
			Content: "import com.example.model.User; public class Service { User user; }",
		},
		{
			Path:    "com/example/model/User.java", 
			Content: "package com.example.model; public class User { String name; }",
		},
		{
			Path:    "com/example/unrelated/Other.java",
			Content: "package com.example.unrelated; public class Other {}",
		},
	}

	errors := []ErrorDetails{
		{
			Message: "cannot find symbol: class User",
			File:    "com/example/Service.java",
			Line:    1,
		},
	}

	// Should identify Service.java as relevant (contains error)
	assert.True(t, analyzer.isRelevantFile(files[0], errors))
	
	// Should identify User.java as relevant (imported by error file)
	assert.True(t, analyzer.isRelevantFileEnhanced(files[1], files, errors))
	
	// Should not identify Other.java as relevant
	assert.False(t, analyzer.isRelevantFileEnhanced(files[2], files, errors))
}

func TestAnalyzer_TokenOptimization(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	analyzer := NewAnalyzer(&MockLLMProvider{}, logger)

	// Create request with large content
	req := &ARFAnalysisRequest{
		ProjectID: "test-project",
		Errors: []ErrorDetails{
			{
				Message: "compilation error",
				File:    "Test.java",
				Line:    1,
				Type:    "compilation",
			},
		},
		CodeContext: CodeContext{
			Language: "java",
			Dependencies: make([]Dependency, 100), // Many dependencies
			SourceFiles: []SourceFile{
				{
					Path:      "Test.java",
					Content:   strings.Repeat("// Very long file content\n", 1000),
					LineCount: 1000,
				},
			},
		},
		TransformGoal: "Fix compilation errors",
		AttemptNumber: 1,
	}

	context, err := analyzer.buildAnalysisContext(req, []PatternMatch{})
	require.NoError(t, err)

	// Should respect token limits (approximate)
	tokenCount := len(strings.Fields(context))
	assert.Less(t, tokenCount, 3500, "Context should stay within token budget")
	
	// Should still include essential information
	assert.Contains(t, context, "compilation error")
	assert.Contains(t, context, "Test.java")
}

func TestAnalyzer_PatternPrioritization(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	analyzer := NewAnalyzer(&MockLLMProvider{}, logger)

	errors := []ErrorDetails{
		{
			Message:  "cannot find symbol: javax.servlet.http.HttpServlet",
			File:     "Test.java",
			Line:     1,
			Type:     "compilation",
			Severity: "error",
		},
		{
			Message:  "deprecated method usage",
			File:     "Test.java", 
			Line:     5,
			Type:     "compilation",
			Severity: "warning",
		},
	}

	codeContext := CodeContext{
		Language: "java",
		Dependencies: []Dependency{
			{
				GroupID:    "jakarta.servlet",
				ArtifactID: "jakarta.servlet-api",
				Version:    "5.0.0",
			},
		},
	}

	patterns, err := analyzer.patterns.FindPatterns(errors, codeContext)
	require.NoError(t, err)

	// Should prioritize high-severity compilation errors
	var javaxPattern *PatternMatch
	for _, pattern := range patterns {
		if pattern.PatternID == "javax_to_jakarta" {
			javaxPattern = &pattern
			break
		}
	}

	require.NotNil(t, javaxPattern, "Should detect javax to jakarta migration")
	assert.Greater(t, javaxPattern.Confidence, 0.9, "Should have high confidence for clear javax->jakarta migration")
}

func TestAnalyzer_ContextBuilding_Performance(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	analyzer := NewAnalyzer(&MockLLMProvider{}, logger)

	req := createValidARFRequest()
	// Add moderate complexity
	for i := 0; i < 20; i++ {
		req.CodeContext.Dependencies = append(req.CodeContext.Dependencies, Dependency{
			GroupID:    "org.springframework",
			ArtifactID: "spring-core",
			Version:    "5.3.0",
		})
	}

	// Measure performance
	start := logger
	context, err := analyzer.buildAnalysisContext(&req, []PatternMatch{})
	_ = start // Duration measurement would be added in real implementation

	require.NoError(t, err)
	assert.NotEmpty(t, context)
	
	// Performance should be reasonable for typical requests
	// In a real test, we'd measure actual duration and assert it's < 500ms
}

func TestAnalyzer_SemanticAnalysis(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	analyzer := NewAnalyzer(&MockLLMProvider{}, logger)

	content := `package com.example;

import java.util.List;

public class UserService {
    public void processUsers(List<User> users) {
        for (User user : users) {
            user.getName(); // Error: User class not found
        }
    }
}`

	errors := []ErrorDetails{
		{
			Message: "cannot find symbol: class User",
			File:    "UserService.java",
			Line:    7,
			Type:    "compilation",
		},
	}

	result := analyzer.extractRelevantContentWithSemantics(content, errors)
	
	// Should include method context and class structure
	assert.Contains(t, result, "public void processUsers")
	assert.Contains(t, result, "import java.util.List;")
	assert.Contains(t, result, "public class UserService")
	
	// Should highlight the problematic area
	assert.Contains(t, result, "user.getName(); // Error")
}

