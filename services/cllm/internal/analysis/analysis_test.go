package analysis

import (
	"strings"
	"testing"
	"time"
)

// Test data
const testJavaCode = `package com.example;

import java.util.List;
import java.util.ArrayList;

public class TestClass {
    private String name;
    private int value;
    
    public TestClass(String name, int value) {
        this.name = name;
        this.value = value;
    }
    
    public String getName() {
        return name;
    }
    
    public void setValue(int value) {
        this.value = value;
    }
    
    public void processData(List<String> data) {
        for (String item : data) {
            if (item != null) {
                System.out.println(item);
            }
        }
    }
}`

const testJavaWithErrors = `public class ErrorClass {
    public void method() {
        String text = "unclosed
        int x = 10  // missing semicolon
        if (x > 5 {  // unbalanced parenthesis
            System.out.println(x)  // missing semicolon
        }
    }
}`

const testPythonCode = `import os
import sys

class TestClass:
    def __init__(self, name):
        self.name = name
        
    def process_data(self, data):
        for item in data:
            if item:
                print(item)
                
def main():
    test = TestClass("example")
    test.process_data([1, 2, 3])
    
if __name__ == "__main__":
    main()`

// TestPatternDatabase tests the pattern database
func TestPatternDatabase(t *testing.T) {
	db := NewPatternDatabase()
	
	t.Run("GetPatterns", func(t *testing.T) {
		patterns := db.GetPatterns("java")
		if len(patterns) == 0 {
			t.Error("Expected Java patterns, got none")
		}
		
		// Check for specific pattern categories
		hasCompilation := false
		hasRuntime := false
		hasMigration := false
		
		for _, p := range patterns {
			switch p.Category {
			case "compilation":
				hasCompilation = true
			case "runtime":
				hasRuntime = true
			case "migration":
				hasMigration = true
			}
		}
		
		if !hasCompilation {
			t.Error("Expected compilation patterns")
		}
		if !hasRuntime {
			t.Error("Expected runtime patterns")
		}
		if !hasMigration {
			t.Error("Expected migration patterns")
		}
	})
	
	t.Run("AddPattern", func(t *testing.T) {
		customPattern := &ErrorPattern{
			ID:          "custom_test",
			Name:        "Custom Test Pattern",
			Description: "Test pattern",
			Category:    "test",
			Language:    "java",
			Pattern:     "test_pattern",
			Severity:    "low",
		}
		
		db.AddPattern(customPattern)
		patterns := db.GetPatterns("java")
		
		found := false
		for _, p := range patterns {
			if p.ID == "custom_test" {
				found = true
				break
			}
		}
		
		if !found {
			t.Error("Custom pattern not added")
		}
	})
	
	t.Run("RemovePattern", func(t *testing.T) {
		removed := db.RemovePattern("java", "custom_test")
		if !removed {
			t.Error("Failed to remove pattern")
		}
		
		patterns := db.GetPatterns("java")
		for _, p := range patterns {
			if p.ID == "custom_test" {
				t.Error("Pattern was not removed")
			}
		}
	})
}

// TestPatternMatcher tests pattern matching
func TestPatternMatcher(t *testing.T) {
	matcher := NewPatternMatcher(nil)
	
	t.Run("MatchCompilationErrors", func(t *testing.T) {
		errorContext := "Error: ';' expected at line 4"
		matches := matcher.MatchPatterns(testJavaWithErrors, errorContext, "java")
		
		if len(matches) == 0 {
			t.Error("Expected pattern matches, got none")
		}
		
		// Check for semicolon error pattern
		foundSemicolon := false
		for _, m := range matches {
			if strings.Contains(m.PatternID, "semicolon") {
				foundSemicolon = true
				break
			}
		}
		
		if !foundSemicolon {
			t.Error("Expected to find semicolon error pattern")
		}
	})
	
	t.Run("MatchRuntimeErrors", func(t *testing.T) {
		errorContext := "NullPointerException at line 10"
		code := `String value = null;
value.length(); // This will cause NPE`
		
		matches := matcher.MatchPatterns(code, errorContext, "java")
		
		foundNPE := false
		for _, m := range matches {
			if strings.Contains(m.PatternID, "null") {
				foundNPE = true
				if m.Severity != "critical" {
					t.Error("NPE should be critical severity")
				}
				break
			}
		}
		
		if !foundNPE {
			t.Error("Expected to find null pointer pattern")
		}
	})
	
	t.Run("CalculateConfidence", func(t *testing.T) {
		pattern := &ErrorPattern{
			Keywords: []string{"null", "pointer", "exception"},
			Category: "runtime",
		}
		
		confidence := matcher.calculateConfidence("NullPointerException occurred", pattern)
		if confidence < 0.7 {
			t.Errorf("Expected confidence > 0.7, got %f", confidence)
		}
		
		confidence = matcher.calculateConfidence("some random text", pattern)
		// Should be 0.5 base + 0.15 for runtime category = 0.65
		if confidence > 0.7 {
			t.Errorf("Expected moderate confidence, got %f", confidence)
		}
	})
}

// TestAnalyzer tests the code analyzer
func TestAnalyzer(t *testing.T) {
	analyzer := NewAnalyzer()
	
	t.Run("AnalyzeJavaCode", func(t *testing.T) {
		request := AnalysisRequest{
			Code:     testJavaCode,
			Language: "java",
			Options: AnalysisOptions{
				IncludePatterns: true,
				IncludeMetrics:  true,
			},
		}
		
		result, err := analyzer.Analyze(request)
		if err != nil {
			t.Fatalf("Analysis failed: %v", err)
		}
		
		if result.Structure == nil {
			t.Error("Expected structure analysis")
		} else {
			if result.Structure.Package != "com.example" {
				t.Errorf("Expected package 'com.example', got '%s'", result.Structure.Package)
			}
			if len(result.Structure.Classes) != 1 {
				t.Errorf("Expected 1 class, got %d", len(result.Structure.Classes))
			}
			if len(result.Structure.Imports) != 2 {
				t.Errorf("Expected 2 imports, got %d", len(result.Structure.Imports))
			}
		}
		
		if result.Metrics == nil {
			t.Error("Expected metrics")
		} else {
			if result.Metrics.ClassCount != 1 {
				t.Errorf("Expected 1 class in metrics, got %d", result.Metrics.ClassCount)
			}
		}
		
		if result.Context == nil {
			t.Error("Expected LLM context")
		}
	})
	
	t.Run("ExtractJavaStructure", func(t *testing.T) {
		structure := analyzer.extractJavaStructure(testJavaCode)
		
		if structure.Package != "com.example" {
			t.Errorf("Expected package 'com.example', got '%s'", structure.Package)
		}
		
		if len(structure.Imports) != 2 {
			t.Errorf("Expected 2 imports, got %d", len(structure.Imports))
		}
		
		if len(structure.Classes) != 1 {
			t.Errorf("Expected 1 class, got %d", len(structure.Classes))
		} else {
			class := structure.Classes[0]
			if class.Name != "TestClass" {
				t.Errorf("Expected class name 'TestClass', got '%s'", class.Name)
			}
			if len(class.Methods) < 4 {
				t.Errorf("Expected at least 4 methods, got %d", len(class.Methods))
			}
			if len(class.Fields) != 2 {
				t.Errorf("Expected 2 fields, got %d", len(class.Fields))
				for i, field := range class.Fields {
					t.Logf("Field %d: %s (type: %s)", i, field.Name, field.Type)
				}
			}
		}
	})
	
	t.Run("ExtractPythonStructure", func(t *testing.T) {
		structure := analyzer.extractPythonStructure(testPythonCode)
		
		if len(structure.Imports) != 2 {
			t.Errorf("Expected 2 imports, got %d", len(structure.Imports))
		}
		
		if len(structure.Classes) != 1 {
			t.Errorf("Expected 1 class, got %d", len(structure.Classes))
		}
		
		if len(structure.Methods) != 1 {
			t.Errorf("Expected 1 top-level function, got %d", len(structure.Methods))
		}
	})
	
	t.Run("CalculateMetrics", func(t *testing.T) {
		structure := analyzer.extractJavaStructure(testJavaCode)
		metrics := analyzer.calculateMetrics(testJavaCode, structure)
		
		if metrics.ClassCount != 1 {
			t.Errorf("Expected 1 class, got %d", metrics.ClassCount)
		}
		
		if metrics.LinesOfCode == 0 {
			t.Error("Expected non-zero lines of code")
		}
		
		if metrics.TotalLines == 0 {
			t.Error("Expected non-zero total lines")
		}
	})
}

// TestContextBuilder tests the context builder
func TestContextBuilder(t *testing.T) {
	builder := NewContextBuilder()
	
	t.Run("BuildContext", func(t *testing.T) {
		analyzer := NewAnalyzer()
		structure := analyzer.extractJavaStructure(testJavaCode)
		
		config := ContextConfig{
			MaxTokens:      1000,
			IncludeImports: true,
			FocusOnErrors:  false,
			ContextRadius:  3,
		}
		
		context := builder.BuildContext(
			testJavaCode,
			"",
			structure,
			[]PatternMatch{},
			config,
		)
		
		if context.Summary == "" {
			t.Error("Expected summary")
		}
		
		if context.TokenCount == 0 {
			t.Error("Expected token count")
		}
		
		if context.Truncated {
			t.Error("Context should not be truncated for small code")
		}
	})
	
	t.Run("BuildContextWithErrors", func(t *testing.T) {
		errorContext := "Error at line 5: NullPointerException"
		
		config := ContextConfig{
			MaxTokens:     1000,
			FocusOnErrors: true,
			ContextRadius: 5,
		}
		
		context := builder.BuildContext(
			testJavaCode,
			errorContext,
			nil,
			[]PatternMatch{},
			config,
		)
		
		if context.ErrorContext != errorContext {
			t.Error("Expected error context to be preserved")
		}
		
		found := false
		for _, area := range context.FocusAreas {
			if area == "error_resolution" {
				found = true
				break
			}
		}
		
		if !found {
			t.Error("Expected 'error_resolution' in focus areas")
		}
	})
	
	t.Run("TokenEstimation", func(t *testing.T) {
		estimator := NewSimpleTokenEstimator()
		
		text := "This is a test string with some words"
		tokens := estimator.EstimateTokens(text)
		
		// Should be roughly between 5-15 tokens for this text
		if tokens < 5 || tokens > 15 {
			t.Errorf("Token estimate out of expected range: %d", tokens)
		}
	})
}

// TestValidator tests the code validator
func TestValidator(t *testing.T) {
	validator := NewValidator()
	
	t.Run("ValidateValidJavaCode", func(t *testing.T) {
		result := validator.ValidateCode(testJavaCode, "java")
		
		if !result.Valid {
			t.Error("Expected valid code")
			for _, err := range result.Errors {
				t.Logf("Error: %s at line %d", err.Message, err.Location.Line)
			}
		}
	})
	
	t.Run("ValidateInvalidJavaCode", func(t *testing.T) {
		result := validator.ValidateCode(testJavaWithErrors, "java")
		
		if result.Valid {
			t.Error("Expected invalid code")
		}
		
		if len(result.Errors) == 0 {
			t.Error("Expected validation errors")
		}
		
		// Check for specific error types
		hasUnclosedString := false
		hasMissingSemicolon := false
		
		for _, err := range result.Errors {
			switch err.Type {
			case "unclosed_string":
				hasUnclosedString = true
			case "missing_semicolon":
				hasMissingSemicolon = true
			}
		}
		
		if !hasUnclosedString {
			t.Error("Expected unclosed string error")
		}
		if !hasMissingSemicolon {
			t.Error("Expected missing semicolon error")
		}
	})
	
	t.Run("CheckBalancedBraces", func(t *testing.T) {
		code := "{ { } }"
		errors := validator.checkBalancedBraces(code)
		if len(errors) != 0 {
			t.Error("Expected balanced braces")
		}
		
		code = "{ { }"
		errors = validator.checkBalancedBraces(code)
		if len(errors) == 0 {
			t.Error("Expected unbalanced braces error")
		}
	})
	
	t.Run("CheckBalancedParentheses", func(t *testing.T) {
		code := "((()))"
		errors := validator.checkBalancedParentheses(code)
		if len(errors) != 0 {
			t.Error("Expected balanced parentheses")
		}
		
		code = "((())"
		errors = validator.checkBalancedParentheses(code)
		if len(errors) == 0 {
			t.Error("Expected unbalanced parentheses error")
		}
	})
	
	t.Run("ValidatePythonCode", func(t *testing.T) {
		result := validator.ValidateCode(testPythonCode, "python")
		
		if !result.Valid {
			t.Error("Expected valid Python code")
			for _, err := range result.Errors {
				t.Logf("Error: %s at line %d", err.Message, err.Location.Line)
			}
		}
	})
	
	t.Run("GenerateSuggestions", func(t *testing.T) {
		longLineCode := "public class Test { " + strings.Repeat("a", 150) + " }"
		suggestions := validator.generateSuggestions(longLineCode, "java")
		
		if len(suggestions) == 0 {
			t.Error("Expected suggestions for long lines")
		}
	})
}

// TestIntegration tests the full analysis pipeline
func TestIntegration(t *testing.T) {
	analyzer := NewAnalyzer()
	
	t.Run("FullAnalysisPipeline", func(t *testing.T) {
		request := AnalysisRequest{
			Code:         testJavaWithErrors,
			Language:     "java",
			ErrorContext: "Compilation failed: ';' expected at line 4",
			Options: AnalysisOptions{
				IncludePatterns: true,
				IncludeMetrics:  true,
				Timeout:         5 * time.Second,
			},
		}
		
		result, err := analyzer.Analyze(request)
		if err != nil {
			t.Fatalf("Analysis failed: %v", err)
		}
		
		// Check all components are present
		if result.Structure == nil {
			t.Error("Expected structure analysis")
		}
		
		if len(result.Patterns) == 0 {
			t.Error("Expected pattern matches")
		}
		
		if result.Metrics == nil {
			t.Error("Expected metrics")
		}
		
		if len(result.Issues) == 0 {
			t.Error("Expected issues")
		}
		
		if result.Context == nil {
			t.Error("Expected LLM context")
		}
		
		if result.ProcessingTime == 0 {
			t.Error("Expected processing time")
		}
	})
	
	t.Run("PerformanceTest", func(t *testing.T) {
		// Generate larger code sample
		largeCode := strings.Repeat(testJavaCode+"\n", 10)
		
		request := AnalysisRequest{
			Code:     largeCode,
			Language: "java",
			Options: AnalysisOptions{
				IncludePatterns: true,
				IncludeMetrics:  true,
			},
		}
		
		start := time.Now()
		result, err := analyzer.Analyze(request)
		duration := time.Since(start)
		
		if err != nil {
			t.Fatalf("Analysis failed: %v", err)
		}
		
		// Should complete within 5 seconds for typical code
		if duration > 5*time.Second {
			t.Errorf("Analysis took too long: %v", duration)
		}
		
		t.Logf("Analysis completed in %v", duration)
		t.Logf("Token count: %d", result.Context.TokenCount)
	})
}