package arf

import (
	"strings"
)

// countLinesOfCode counts non-empty lines in code
func (c *DefaultComplexityAnalyzer) countLinesOfCode(code string) int {
	lines := strings.Split(code, "\n")
	nonEmptyLines := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines++
		}
	}
	return nonEmptyLines
}

// countFunctions counts function/method definitions in code
func (c *DefaultComplexityAnalyzer) countFunctions(code string, language string) int {
	// Simple pattern-based counting - could be enhanced with AST
	patterns := map[string][]string{
		"java":       {"public ", "private ", "protected ", "static "},
		"javascript": {"function ", "const ", "let ", "=> "},
		"python":     {"def "},
		"go":         {"func "},
		"rust":       {"fn "},
	}

	count := 0
	if langPatterns, exists := patterns[language]; exists {
		for _, pattern := range langPatterns {
			count += strings.Count(strings.ToLower(code), pattern)
		}
	}

	return count / 2 // Rough estimate, divide by 2 to account for overcount
}

// countClasses counts class/struct/interface definitions
func (c *DefaultComplexityAnalyzer) countClasses(code string, language string) int {
	// Simple pattern-based counting
	patterns := map[string][]string{
		"java":       {"class ", "interface ", "enum "},
		"javascript": {"class "},
		"typescript": {"class ", "interface "},
		"python":     {"class "},
		"go":         {"type ", "struct "},
		"rust":       {"struct ", "enum ", "trait "},
	}

	count := 0
	if langPatterns, exists := patterns[language]; exists {
		for _, pattern := range langPatterns {
			count += strings.Count(strings.ToLower(code), pattern)
		}
	}

	return count
}

// calculateNestingDepth calculates maximum nesting depth of code blocks
func (c *DefaultComplexityAnalyzer) calculateNestingDepth(code string, language string) int {
	// Simple brace counting for nesting depth
	maxDepth := 0
	currentDepth := 0

	for _, char := range code {
		if char == '{' {
			currentDepth++
			if currentDepth > maxDepth {
				maxDepth = currentDepth
			}
		} else if char == '}' {
			currentDepth--
		}
	}

	return maxDepth
}

// calculateCyclomaticComplexity calculates cyclomatic complexity
func (c *DefaultComplexityAnalyzer) calculateCyclomaticComplexity(code string, language string) int {
	// Simplified cyclomatic complexity
	// Count decision points: if, while, for, switch, catch
	decisionKeywords := []string{"if", "while", "for", "switch", "catch", "elif", "else if"}

	complexity := 1 // Base complexity
	codeLowar := strings.ToLower(code)

	for _, keyword := range decisionKeywords {
		complexity += strings.Count(codeLowar, keyword)
	}

	return complexity
}

// calculateCognitiveComplexity calculates cognitive complexity
func (c *DefaultComplexityAnalyzer) calculateCognitiveComplexity(code string, language string) int {
	// Simplified cognitive complexity
	// Similar to cyclomatic but with nesting penalties
	complexity := c.calculateCyclomaticComplexity(code, language)
	nestingDepth := c.calculateNestingDepth(code, language)

	// Add penalty for deep nesting
	return complexity + nestingDepth
}

// calculateMaintainabilityIndex calculates maintainability index
func (c *DefaultComplexityAnalyzer) calculateMaintainabilityIndex(metrics *CodeComplexityMetrics) float64 {
	// Simplified maintainability index
	// Based on Halstead metrics approximation and cyclomatic complexity
	if metrics.LinesOfCode == 0 {
		return 100.0
	}

	// Approximate calculation
	complexity := float64(metrics.CyclomaticComplexity)
	loc := float64(metrics.LinesOfCode)

	// Higher complexity and more lines = lower maintainability
	maintainability := 100.0 - (complexity * 5) - (loc / 10)

	if maintainability < 0 {
		maintainability = 0
	}
	if maintainability > 100 {
		maintainability = 100
	}

	return maintainability
}

// extractMetricsFromAST enhances metrics using AST data if available
func (c *DefaultComplexityAnalyzer) extractMetricsFromAST(ast *UniversalAST, metrics *CodeComplexityMetrics) *CodeComplexityMetrics {
	// Enhance metrics using AST data
	if ast != nil {
		// More accurate function count from symbols
		functionSymbols := 0
		classSymbols := 0

		for _, symbol := range ast.Symbols {
			switch symbol.Type {
			case "function", "method":
				functionSymbols++
			case "class", "struct":
				classSymbols++
			}
		}

		// Use AST data if it seems more accurate
		if functionSymbols > 0 {
			metrics.FunctionCount = functionSymbols
		}
		if classSymbols > 0 {
			metrics.ClassCount = classSymbols
		}
	}

	return metrics
}
