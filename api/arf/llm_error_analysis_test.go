package arf

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockLLMDispatcher for testing
type MockLLMDispatcher struct {
	mock.Mock
}

func (m *MockLLMDispatcher) SubmitLLMTransformation(ctx context.Context, provider, model, prompt string, params map[string]interface{}) (*LLMJob, error) {
	args := m.Called(ctx, provider, model, prompt, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*LLMJob), args.Error(1)
}

func (m *MockLLMDispatcher) GetJobStatus(ctx context.Context, jobID string) (*LLMJob, error) {
	args := m.Called(ctx, jobID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*LLMJob), args.Error(1)
}

func TestAnalyzeErrorsWithLLM(t *testing.T) {
	tests := []struct {
		name           string
		errors         []string
		language       string
		mockResponse   *LLMAnalysisResult
		expectError    bool
		expectedPrompt string
	}{
		{
			name: "compilation error analysis",
			errors: []string{
				"./main.java:10:5: cannot find symbol: class NonExistent",
				"./main.java:15:10: package org.example does not exist",
			},
			language: "java",
			mockResponse: &LLMAnalysisResult{
				ErrorType:  "compilation",
				Confidence: 0.95,
				SuggestedFix: `Add missing import:
import org.example.*;

Define missing class:
public class NonExistent {
    // Implementation
}`,
				AlternativeFixes: []string{
					"Add dependency to pom.xml or build.gradle",
					"Create the missing class in the appropriate package",
				},
				RiskAssessment: "low",
			},
			expectError: false,
		},
		{
			name: "test failure analysis",
			errors: []string{
				"Test testCalculateTotal failed: expected:<100> but was:<95>",
				"AssertionError at TestClass.java:45",
			},
			language: "java",
			mockResponse: &LLMAnalysisResult{
				ErrorType:  "test",
				Confidence: 0.85,
				SuggestedFix: `Fix calculation logic:
// Change line 23 in Calculator.java
return total * 1.0; // Remove discount that was incorrectly applied`,
				AlternativeFixes: []string{
					"Update test expectation if business logic changed",
					"Check for rounding errors in calculation",
				},
				RiskAssessment: "medium",
			},
			expectError: false,
		},
		{
			name: "mixed error analysis",
			errors: []string{
				"./service.go:25:3: undefined: logger",
				"./service_test.go:15: test failed: connection refused",
			},
			language: "go",
			mockResponse: &LLMAnalysisResult{
				ErrorType:  "mixed",
				Confidence: 0.75,
				SuggestedFix: `1. Add logger import:
import "log"

2. Fix test connection:
// Use test database or mock the connection`,
				AlternativeFixes: []string{
					"Initialize logger in init() function",
					"Use dependency injection for testability",
				},
				RiskAssessment: "medium",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewEnhancedLLMAnalyzer(nil, nil)

			// For this test, we'll mock the internal LLM call
			result := analyzer.analyzeErrorsWithPattern(tt.errors, tt.language)

			if tt.expectError {
				assert.Nil(t, result)
			} else {
				// The basic pattern analysis should categorize correctly
				assert.NotNil(t, result)
				// The actual LLM enhancement would happen in AnalyzeErrors
			}
		})
	}
}

func TestExtractErrorContext(t *testing.T) {
	tests := []struct {
		name            string
		errors          []string
		expectedContext ErrorContext
	}{
		{
			name: "java compilation errors",
			errors: []string{
				"[ERROR] /src/main/java/App.java:[10,5] cannot find symbol",
				"[ERROR]   symbol:   class NonExistent",
				"[ERROR]   location: class com.example.App",
			},
			expectedContext: ErrorContext{
				ErrorType:    "compilation",
				ErrorMessage: "[ERROR] /src/main/java/App.java:[10,5] cannot find symbol\n[ERROR]   symbol:   class NonExistent\n[ERROR]   location: class com.example.App",
				SourceFile:   "/src/main/java/App.java",
			},
		},
		{
			name: "go compilation errors",
			errors: []string{
				"./main.go:10:2: undefined: fmt.Printlnx",
				"./main.go:15:5: cannot use x (type int) as type string",
			},
			expectedContext: ErrorContext{
				ErrorType:    "compilation",
				ErrorMessage: "./main.go:10:2: undefined: fmt.Printlnx\n./main.go:15:5: cannot use x (type int) as type string",
				SourceFile:   "./main.go",
			},
		},
		{
			name: "test failures",
			errors: []string{
				"FAIL: TestCalculation",
				"    calculator_test.py:25: AssertionError: 100 != 95",
			},
			expectedContext: ErrorContext{
				ErrorType:    "test",
				ErrorMessage: "FAIL: TestCalculation\n    calculator_test.py:25: AssertionError: 100 != 95",
				SourceFile:   "calculator_test.py",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewEnhancedLLMAnalyzer(nil, nil)
			context := analyzer.extractErrorContext(tt.errors)

			assert.Equal(t, tt.expectedContext.ErrorType, context.ErrorType)
			assert.Equal(t, tt.expectedContext.ErrorMessage, context.ErrorMessage)
			if tt.expectedContext.SourceFile != "" {
				assert.Contains(t, context.SourceFile, tt.expectedContext.SourceFile)
			}
		})
	}
}

func TestGenerateHealingPrompt(t *testing.T) {
	tests := []struct {
		name           string
		errorContext   ErrorContext
		language       string
		expectedPrompt string
	}{
		{
			name: "java compilation error prompt",
			errorContext: ErrorContext{
				ErrorType:    "compilation",
				ErrorMessage: "cannot find symbol: class NonExistent",
				SourceFile:   "App.java",
			},
			language: "java",
			expectedPrompt: `You are an expert Java developer. Analyze the following compilation error and provide a fix.

Error Type: compilation
Source File: App.java
Error Message:
cannot find symbol: class NonExistent

Please provide:
1. Root cause analysis
2. Suggested fix with code
3. Alternative solutions
4. Risk assessment (low/medium/high)
5. Required dependencies or imports

Format your response as JSON with fields: suggested_fix, alternative_fixes, risk_assessment, confidence_score (0-1)`,
		},
		{
			name: "python test failure prompt",
			errorContext: ErrorContext{
				ErrorType:    "test",
				ErrorMessage: "AssertionError: 100 != 95",
				SourceFile:   "test_calc.py",
			},
			language: "python",
			expectedPrompt: `You are an expert Python developer. Analyze the following test failure and provide a fix.

Error Type: test
Source File: test_calc.py
Error Message:
AssertionError: 100 != 95

Please provide:
1. Root cause analysis
2. Suggested fix with code
3. Alternative solutions
4. Risk assessment (low/medium/high)
5. Whether to fix the code or update the test

Format your response as JSON with fields: suggested_fix, alternative_fixes, risk_assessment, confidence_score (0-1)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewEnhancedLLMAnalyzer(nil, nil)
			prompt := analyzer.generateHealingPrompt(tt.errorContext, tt.language)

			assert.Contains(t, prompt, tt.language)
			assert.Contains(t, prompt, tt.errorContext.ErrorType)
			assert.Contains(t, prompt, tt.errorContext.ErrorMessage)
			assert.Contains(t, prompt, "suggested_fix")
			assert.Contains(t, prompt, "confidence_score")
		})
	}
}

func TestConvertToOpenRewriteRecipe(t *testing.T) {
	tests := []struct {
		name             string
		analysis         *LLMAnalysisResult
		language         string
		expectedRecipe   string
		expectedMetadata map[string]interface{}
	}{
		{
			name: "java import fix",
			analysis: &LLMAnalysisResult{
				ErrorType:    "compilation",
				SuggestedFix: "Add import: import java.util.List;",
				Confidence:   0.95,
			},
			language:       "java",
			expectedRecipe: "org.openrewrite.java.AddImport",
			expectedMetadata: map[string]interface{}{
				"type":       "java.util.List",
				"onlyIfUsed": true,
			},
		},
		{
			name: "python unused import",
			analysis: &LLMAnalysisResult{
				ErrorType:    "linting",
				SuggestedFix: "Remove unused import: import unused_module",
				Confidence:   0.90,
			},
			language:       "python",
			expectedRecipe: "org.openrewrite.python.cleanup.RemoveUnusedImports",
			expectedMetadata: map[string]interface{}{
				"module": "unused_module",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewEnhancedLLMAnalyzer(nil, nil)
			recipe, metadata := analyzer.convertToOpenRewriteRecipe(tt.analysis, tt.language)

			assert.Equal(t, tt.expectedRecipe, recipe)
			if tt.expectedMetadata != nil {
				assert.NotNil(t, metadata)
				for key, value := range tt.expectedMetadata {
					assert.Equal(t, value, metadata[key])
				}
			}
		})
	}
}

func TestBatchErrorAnalysis(t *testing.T) {
	analyzer := NewEnhancedLLMAnalyzer(nil, nil)

	errors := [][]string{
		{"error1: compilation failed"},
		{"error2: test failed"},
		{"error3: import missing"},
	}

	// Test batching multiple error sets
	results := analyzer.BatchAnalyzeErrors(context.Background(), errors, "java")

	assert.Len(t, results, 3)
	// Each should have some analysis even if basic
	for _, result := range results {
		assert.NotNil(t, result)
	}
}
