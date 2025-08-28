package analysis

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ContextBuilder builds optimized context for LLM processing
type ContextBuilder struct {
	tokenEstimator TokenEstimator
}

// NewContextBuilder creates a new context builder
func NewContextBuilder() *ContextBuilder {
	return &ContextBuilder{
		tokenEstimator: NewSimpleTokenEstimator(),
	}
}

// BuildContext builds an optimized LLM context from analysis results
func (cb *ContextBuilder) BuildContext(
	code string,
	errorContext string,
	structure *CodeStructure,
	patterns []PatternMatch,
	config ContextConfig,
) *LLMContext {
	
	context := &LLMContext{
		CodeSnippets: []CodeSnippet{},
		FocusAreas:   []string{},
		TokenCount:   0,
		Truncated:    false,
	}
	
	// Build summary
	context.Summary = cb.buildSummary(structure, patterns)
	
	// Add error context if present
	if errorContext != "" {
		context.ErrorContext = errorContext
		context.FocusAreas = append(context.FocusAreas, "error_resolution")
	}
	
	// Extract relevant code snippets
	snippets := cb.extractRelevantSnippets(code, errorContext, patterns, config)
	
	// Sort snippets by relevance
	sort.Slice(snippets, func(i, j int) bool {
		return snippets[i].Relevance > snippets[j].Relevance
	})
	
	// Add snippets up to token limit
	currentTokens := cb.tokenEstimator.EstimateTokens(context.Summary + context.ErrorContext)
	
	for _, snippet := range snippets {
		snippetTokens := cb.tokenEstimator.EstimateTokens(snippet.Code)
		
		if currentTokens+snippetTokens > config.MaxTokens {
			context.Truncated = true
			break
		}
		
		context.CodeSnippets = append(context.CodeSnippets, snippet)
		currentTokens += snippetTokens
	}
	
	context.TokenCount = currentTokens
	
	// Identify focus areas based on patterns
	context.FocusAreas = append(context.FocusAreas, cb.identifyFocusAreas(patterns)...)
	
	return context
}

// buildSummary builds a summary of the code structure
func (cb *ContextBuilder) buildSummary(structure *CodeStructure, patterns []PatternMatch) string {
	var parts []string
	
	// Package/module information
	if structure != nil {
		if structure.Package != "" {
			parts = append(parts, fmt.Sprintf("Package: %s", structure.Package))
		}
		
		// Class summary
		if len(structure.Classes) > 0 {
			classNames := []string{}
			for _, class := range structure.Classes {
				classNames = append(classNames, class.Name)
			}
			parts = append(parts, fmt.Sprintf("Classes: %s", strings.Join(classNames, ", ")))
		}
		
		// Method count
		methodCount := len(structure.Methods)
		for _, class := range structure.Classes {
			methodCount += len(class.Methods)
		}
		if methodCount > 0 {
			parts = append(parts, fmt.Sprintf("Methods: %d", methodCount))
		}
		
		// Import summary
		if len(structure.Imports) > 0 {
			parts = append(parts, fmt.Sprintf("Imports: %d dependencies", len(structure.Imports)))
		}
	}
	
	// Pattern summary
	if len(patterns) > 0 {
		criticalCount := 0
		highCount := 0
		for _, pattern := range patterns {
			switch pattern.Severity {
			case "critical":
				criticalCount++
			case "high":
				highCount++
			}
		}
		
		if criticalCount > 0 || highCount > 0 {
			parts = append(parts, fmt.Sprintf("Issues: %d critical, %d high severity", criticalCount, highCount))
		}
	}
	
	if len(parts) == 0 {
		return "Code analysis summary"
	}
	
	return strings.Join(parts, "\n")
}

// extractRelevantSnippets extracts code snippets relevant to the analysis
func (cb *ContextBuilder) extractRelevantSnippets(
	code string,
	errorContext string,
	patterns []PatternMatch,
	config ContextConfig,
) []CodeSnippet {
	
	var snippets []CodeSnippet
	lines := strings.Split(code, "\n")
	
	// Add snippets for error locations
	if config.FocusOnErrors && errorContext != "" {
		errorSnippets := cb.extractErrorSnippets(lines, errorContext, config.ContextRadius)
		snippets = append(snippets, errorSnippets...)
	}
	
	// Add snippets for pattern matches
	for _, pattern := range patterns {
		if pattern.Severity == "critical" || pattern.Severity == "high" {
			snippet := cb.extractSnippetForLocation(lines, pattern.Location, config.ContextRadius)
			snippet.Description = fmt.Sprintf("Pattern: %s", pattern.Name)
			snippet.Relevance = cb.calculateRelevance(pattern.Severity, pattern.Confidence)
			snippets = append(snippets, snippet)
		}
	}
	
	// Add import statements if configured
	if config.IncludeImports {
		importSnippet := cb.extractImports(lines)
		if importSnippet != nil {
			snippets = append(snippets, *importSnippet)
		}
	}
	
	// Deduplicate overlapping snippets
	snippets = cb.deduplicateSnippets(snippets)
	
	return snippets
}

// extractErrorSnippets extracts snippets around error locations
func (cb *ContextBuilder) extractErrorSnippets(lines []string, errorContext string, radius int) []CodeSnippet {
	var snippets []CodeSnippet
	
	// Parse error context for line numbers
	errorLines := cb.parseErrorLines(errorContext)
	
	for _, lineNum := range errorLines {
		if lineNum > 0 && lineNum <= len(lines) {
			snippet := cb.extractSnippetForLocation(lines, Location{Line: lineNum}, radius)
			snippet.Description = "Error location"
			snippet.Relevance = 1.0 // Highest relevance for error locations
			snippets = append(snippets, snippet)
		}
	}
	
	return snippets
}

// parseErrorLines extracts line numbers from error context
func (cb *ContextBuilder) parseErrorLines(errorContext string) []int {
	var lineNumbers []int
	seenLines := make(map[int]bool)
	
	// Look for patterns like "line 42", ":42:", "[42]"
	patterns := []string{
		`line\s+(\d+)`,
		`:(\d+):`,
		`\[(\d+)\]`,
		`at line (\d+)`,
		`^(\d+):`,
	}
	
	for _, pattern := range patterns {
		matches := regexp.MustCompile(pattern).FindAllStringSubmatch(errorContext, -1)
		for _, match := range matches {
			if len(match) > 1 {
				if lineNum := parseInt(match[1]); lineNum > 0 {
					if !seenLines[lineNum] {
						lineNumbers = append(lineNumbers, lineNum)
						seenLines[lineNum] = true
					}
				}
			}
		}
	}
	
	sort.Ints(lineNumbers)
	return lineNumbers
}

// extractSnippetForLocation extracts a code snippet around a location
func (cb *ContextBuilder) extractSnippetForLocation(lines []string, location Location, radius int) CodeSnippet {
	startLine := location.Line - radius - 1 // Convert to 0-indexed
	if startLine < 0 {
		startLine = 0
	}
	
	endLine := location.Line + radius
	if location.EndLine > 0 {
		endLine = location.EndLine + radius
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	
	snippetLines := lines[startLine:endLine]
	
	return CodeSnippet{
		Code: strings.Join(snippetLines, "\n"),
		Location: Location{
			Line:    startLine + 1,
			EndLine: endLine,
		},
		Relevance: 0.5, // Default relevance
	}
}

// extractImports extracts import statements
func (cb *ContextBuilder) extractImports(lines []string) *CodeSnippet {
	var importLines []string
	var startLine, endLine int
	inImports := false
	
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// Check for import statements
		if strings.HasPrefix(trimmed, "import") || strings.HasPrefix(trimmed, "from ") ||
			strings.HasPrefix(trimmed, "using") || strings.HasPrefix(trimmed, "require") {
			if !inImports {
				inImports = true
				startLine = i + 1
			}
			importLines = append(importLines, line)
			endLine = i + 1
		} else if inImports && trimmed != "" && !strings.HasPrefix(trimmed, "//") && 
			!strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "*") {
			// End of import section
			break
		}
	}
	
	if len(importLines) == 0 {
		return nil
	}
	
	return &CodeSnippet{
		Description: "Import statements",
		Code:        strings.Join(importLines, "\n"),
		Location: Location{
			Line:    startLine,
			EndLine: endLine,
		},
		Relevance: 0.3, // Lower relevance for imports
	}
}

// calculateRelevance calculates relevance score based on severity and confidence
func (cb *ContextBuilder) calculateRelevance(severity string, confidence float64) float64 {
	var severityScore float64
	
	switch severity {
	case "critical":
		severityScore = 1.0
	case "high":
		severityScore = 0.8
	case "medium":
		severityScore = 0.6
	case "low":
		severityScore = 0.4
	default:
		severityScore = 0.5
	}
	
	// Combine severity and confidence
	return severityScore * confidence
}

// deduplicateSnippets removes overlapping snippets
func (cb *ContextBuilder) deduplicateSnippets(snippets []CodeSnippet) []CodeSnippet {
	if len(snippets) <= 1 {
		return snippets
	}
	
	// Sort by start line
	sort.Slice(snippets, func(i, j int) bool {
		return snippets[i].Location.Line < snippets[j].Location.Line
	})
	
	var result []CodeSnippet
	current := snippets[0]
	
	for i := 1; i < len(snippets); i++ {
		next := snippets[i]
		
		// Check for overlap
		if current.Location.EndLine >= next.Location.Line {
			// Merge snippets
			current = cb.mergeSnippets(current, next)
		} else {
			result = append(result, current)
			current = next
		}
	}
	
	result = append(result, current)
	return result
}

// mergeSnippets merges two overlapping snippets
func (cb *ContextBuilder) mergeSnippets(s1, s2 CodeSnippet) CodeSnippet {
	merged := CodeSnippet{
		Description: cb.mergeDescriptions(s1.Description, s2.Description),
		Location: Location{
			Line: s1.Location.Line,
			EndLine: s2.Location.EndLine,
		},
		Relevance: max(s1.Relevance, s2.Relevance),
	}
	
	// Merge code (assuming they're from the same source)
	// In a real implementation, we'd need to properly merge the actual code lines
	if len(s1.Code) > len(s2.Code) {
		merged.Code = s1.Code
	} else {
		merged.Code = s2.Code
	}
	
	return merged
}

// mergeDescriptions combines two snippet descriptions
func (cb *ContextBuilder) mergeDescriptions(d1, d2 string) string {
	if d1 == "" {
		return d2
	}
	if d2 == "" {
		return d1
	}
	if d1 == d2 {
		return d1
	}
	return d1 + "; " + d2
}

// identifyFocusAreas identifies focus areas based on patterns
func (cb *ContextBuilder) identifyFocusAreas(patterns []PatternMatch) []string {
	focusMap := make(map[string]bool)
	
	for _, pattern := range patterns {
		switch pattern.Category {
		case "compilation":
			focusMap["compilation_errors"] = true
		case "runtime":
			focusMap["runtime_errors"] = true
		case "migration":
			focusMap["code_migration"] = true
		case "quality":
			focusMap["code_quality"] = true
		}
		
		// Add specific focus based on pattern ID
		if strings.Contains(pattern.PatternID, "null") {
			focusMap["null_safety"] = true
		}
		if strings.Contains(pattern.PatternID, "type") {
			focusMap["type_safety"] = true
		}
		if strings.Contains(pattern.PatternID, "memory") {
			focusMap["memory_management"] = true
		}
	}
	
	var focusAreas []string
	for area := range focusMap {
		focusAreas = append(focusAreas, area)
	}
	
	sort.Strings(focusAreas)
	return focusAreas
}

// TokenEstimator estimates token count for text
type TokenEstimator interface {
	EstimateTokens(text string) int
}

// SimpleTokenEstimator provides simple token estimation
type SimpleTokenEstimator struct{}

// NewSimpleTokenEstimator creates a new simple token estimator
func NewSimpleTokenEstimator() *SimpleTokenEstimator {
	return &SimpleTokenEstimator{}
}

// EstimateTokens estimates tokens using simple heuristics
func (e *SimpleTokenEstimator) EstimateTokens(text string) int {
	// Simple estimation: ~1 token per 4 characters or 0.75 tokens per word
	// This is a rough approximation for GPT-style tokenization
	
	charCount := len(text)
	words := strings.Fields(text)
	wordCount := len(words)
	
	// Use average of character and word-based estimates
	charEstimate := charCount / 4
	wordEstimate := int(float64(wordCount) * 0.75)
	
	return (charEstimate + wordEstimate) / 2
}

// Helper functions

func parseInt(s string) int {
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}