package arf

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/services/cllm/internal/providers"
)

// Analyzer handles ARF-specific error analysis and code suggestions
type Analyzer struct {
	llmProvider providers.Provider
	logger      *slog.Logger
	patterns    *PatternMatcher
}

// NewAnalyzer creates a new ARF analyzer
func NewAnalyzer(llmProvider providers.Provider, logger *slog.Logger) *Analyzer {
	return &Analyzer{
		llmProvider: llmProvider,
		logger:      logger,
		patterns:    NewPatternMatcher(logger),
	}
}

// AnalyzeErrors performs comprehensive error analysis for ARF workflows
func (a *Analyzer) AnalyzeErrors(ctx context.Context, req *ARFAnalysisRequest, options ARFAnalysisOptions) (*ARFAnalysisResponse, error) {
	startTime := time.Now()
	
	a.logger.Debug("Starting ARF error analysis",
		"project_id", req.ProjectID,
		"errors_count", len(req.Errors),
		"attempt_number", req.AttemptNumber)
	
	response := &ARFAnalysisResponse{
		Suggestions:    make([]CodeSuggestion, 0),
		PatternMatches: make([]PatternMatch, 0),
		Metadata: ResponseMetadata{
			ProcessingSteps: make([]ProcessingStep, 0),
			Timestamp:      time.Now(),
			Environment:    "production",
		},
	}
	
	// Step 1: Pattern matching
	stepStart := time.Now()
	patterns, err := a.patterns.FindPatterns(req.Errors, req.CodeContext)
	if err != nil {
		a.logger.Warn("Pattern matching failed", "error", err)
	} else {
		response.PatternMatches = patterns
		a.addProcessingStep(response, "pattern_matching", stepStart, "success", 
			fmt.Sprintf("Found %d patterns", len(patterns)))
	}
	
	// Step 2: Build analysis context
	stepStart = time.Now()
	analysisContext, err := a.buildAnalysisContext(req, patterns)
	if err != nil {
		return nil, fmt.Errorf("failed to build analysis context: %w", err)
	}
	a.addProcessingStep(response, "context_building", stepStart, "success",
		fmt.Sprintf("Built context with %d tokens", len(analysisContext)))
	
	// Step 3: LLM analysis
	stepStart = time.Now()
	analysis, suggestions, err := a.performLLMAnalysis(ctx, analysisContext, options)
	if err != nil {
		return nil, fmt.Errorf("LLM analysis failed: %w", err)
	}
	
	response.Analysis = analysis
	response.Suggestions = suggestions
	a.addProcessingStep(response, "llm_analysis", stepStart, "success",
		fmt.Sprintf("Generated %d suggestions", len(suggestions)))
	
	// Step 4: Quality assessment
	stepStart = time.Now()
	confidence := a.calculateConfidence(analysis, suggestions, patterns)
	response.Confidence = confidence
	a.addProcessingStep(response, "quality_assessment", stepStart, "success",
		fmt.Sprintf("Confidence: %.2f", confidence))
	
	// Step 5: Response enhancement
	stepStart = time.Now()
	a.enhanceResponse(response, req, options)
	a.addProcessingStep(response, "response_enhancement", stepStart, "success", "Enhanced response")
	
	// Set metadata
	response.Metadata.ModelUsed = a.getModelName(options)
	response.Metadata.LLMProvider = a.llmProvider.Name()
	response.Metadata.QualityScore = a.calculateQualityScore(response)
	
	totalDuration := time.Since(startTime)
	a.logger.Info("ARF error analysis completed",
		"project_id", req.ProjectID,
		"duration", totalDuration,
		"confidence", confidence,
		"suggestions_count", len(suggestions),
		"patterns_count", len(patterns))
	
	return response, nil
}

// buildAnalysisContext creates optimized context for LLM analysis with token budgeting
func (a *Analyzer) buildAnalysisContext(req *ARFAnalysisRequest, patterns []PatternMatch) (string, error) {
	const maxTokenBudget = 3000 // Target token limit
	contextBuilder := NewContextBuilder(maxTokenBudget)
	
	// Essential project context (always included)
	contextBuilder.AddSection("project", 50, func() string {
		var section strings.Builder
		section.WriteString(fmt.Sprintf("Project: %s\n", req.ProjectID))
		section.WriteString(fmt.Sprintf("Language: %s\n", req.CodeContext.Language))
		section.WriteString(fmt.Sprintf("Transform Goal: %s\n", req.TransformGoal))
		section.WriteString(fmt.Sprintf("Attempt: %d\n", req.AttemptNumber))
		
		if req.CodeContext.FrameworkVersion != "" {
			section.WriteString(fmt.Sprintf("Framework Version: %s\n", req.CodeContext.FrameworkVersion))
		}
		if req.CodeContext.BuildTool != "" {
			section.WriteString(fmt.Sprintf("Build Tool: %s\n", req.CodeContext.BuildTool))
		}
		return section.String()
	})
	
	// High priority: Error details
	contextBuilder.AddSection("errors", 800, func() string {
		var section strings.Builder
		section.WriteString("\nErrors to Analyze:\n")
		
		// Prioritize errors by severity and type
		prioritizedErrors := a.prioritizeErrors(req.Errors)
		
		for i, err := range prioritizedErrors {
			if i >= 10 { // Limit to top 10 errors
				section.WriteString(fmt.Sprintf("... and %d more errors\n", len(req.Errors)-i))
				break
			}
			
			section.WriteString(fmt.Sprintf("\n%d. [%s] %s\n", i+1, strings.ToUpper(err.Type), err.Message))
			section.WriteString(fmt.Sprintf("   File: %s:%d\n", err.File, err.Line))
			if err.Context != "" && len(err.Context) < 200 { // Limit context size
				section.WriteString(fmt.Sprintf("   Context: %s\n", err.Context))
			}
		}
		return section.String()
	})
	
	// Medium priority: Pattern matches
	contextBuilder.AddSection("patterns", 300, func() string {
		if len(patterns) == 0 {
			return ""
		}
		
		var section strings.Builder
		section.WriteString("\nDetected Patterns:\n")
		
		// Sort patterns by confidence and show top 5
		sortedPatterns := a.sortPatternsByConfidence(patterns)
		limit := min(5, len(sortedPatterns))
		
		for i := 0; i < limit; i++ {
			pattern := sortedPatterns[i]
			section.WriteString(fmt.Sprintf("- %s (confidence: %.2f)\n", pattern.PatternName, pattern.Confidence))
		}
		
		return section.String()
	})
	
	// Medium priority: Relevant dependencies
	contextBuilder.AddSection("dependencies", 400, func() string {
		if len(req.CodeContext.Dependencies) == 0 {
			return ""
		}
		
		var section strings.Builder
		section.WriteString("\nKey Dependencies:\n")
		
		// Prioritize dependencies based on relevance to errors
		relevantDeps := a.prioritizeDependencies(req.CodeContext.Dependencies, req.Errors, patterns)
		limit := min(8, len(relevantDeps))
		
		for i := 0; i < limit; i++ {
			dep := relevantDeps[i]
			section.WriteString(fmt.Sprintf("- %s:%s:%s\n", dep.GroupID, dep.ArtifactID, dep.Version))
		}
		
		if len(req.CodeContext.Dependencies) > limit {
			section.WriteString(fmt.Sprintf("... and %d more dependencies\n", len(req.CodeContext.Dependencies)-limit))
		}
		
		return section.String()
	})
	
	// Lower priority: Source code (most token-consuming)
	contextBuilder.AddSection("source", 1200, func() string {
		var section strings.Builder
		section.WriteString("\nRelevant Source Code:\n")
		
		// Prioritize files by relevance
		relevantFiles := a.prioritizeSourceFiles(req.CodeContext.SourceFiles, req.Errors)
		
		for i, file := range relevantFiles {
			if i >= 3 { // Limit to top 3 files
				break
			}
			
			section.WriteString(fmt.Sprintf("\n--- %s ---\n", file.Path))
			relevantContent := a.extractRelevantContent(file.Content, req.Errors)
			section.WriteString(relevantContent)
			section.WriteString("\n")
		}
		
		return section.String()
	})
	
	// Low priority: Attempt history
	contextBuilder.AddSection("history", 200, func() string {
		if len(req.History) == 0 {
			return ""
		}
		
		var section strings.Builder
		section.WriteString("\nPrevious Attempts:\n")
		
		// Show only recent attempts
		start := max(0, len(req.History)-3)
		for i := start; i < len(req.History); i++ {
			attempt := req.History[i]
			section.WriteString(fmt.Sprintf("Attempt %d: %s (fixed: %d, new: %d)\n",
				attempt.AttemptNumber, attempt.Status, attempt.ErrorsFixed, attempt.NewErrors))
		}
		
		return section.String()
	})
	
	// Build the final context within token budget
	return contextBuilder.Build(), nil
}

// performLLMAnalysis executes the actual LLM analysis
func (a *Analyzer) performLLMAnalysis(ctx context.Context, analysisContext string, options ARFAnalysisOptions) (string, []CodeSuggestion, error) {
	prompt := a.buildOptimizedPrompt(analysisContext, options)
	
	// Prepare LLM request
	llmRequest := providers.CompletionRequest{
		Messages: []providers.Message{
			{
				Role:    "system",
				Content: a.getSystemPrompt(),
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature:   float32(options.Temperature),
		MaxTokens:     4096, // Limit response size
		StopSequences: []string{"<END_ANALYSIS>"},
	}
	
	// Call LLM provider
	response, err := a.llmProvider.Complete(ctx, llmRequest)
	if err != nil {
		return "", nil, fmt.Errorf("LLM provider error: %w", err)
	}
	
	// Parse LLM response into analysis and suggestions
	analysis, suggestions, err := a.parseLLMResponse(response.Content)
	if err != nil {
		a.logger.Warn("Failed to parse LLM response", "error", err)
		// Return raw analysis if parsing fails
		return response.Content, []CodeSuggestion{}, nil
	}
	
	return analysis, suggestions, nil
}

// buildOptimizedPrompt creates an optimized prompt for ARF error analysis
func (a *Analyzer) buildOptimizedPrompt(context string, options ARFAnalysisOptions) string {
	var prompt strings.Builder
	
	prompt.WriteString("Please analyze the following Java transformation errors and provide actionable solutions.\n\n")
	prompt.WriteString("Context:\n")
	prompt.WriteString(context)
	prompt.WriteString("\n\n")
	
	prompt.WriteString("Please provide:\n")
	prompt.WriteString("1. A clear analysis of what's causing each error\n")
	prompt.WriteString("2. Specific code suggestions to fix each error\n")
	prompt.WriteString("3. Explanations for why these changes are needed\n")
	prompt.WriteString("4. Any dependency or configuration changes required\n\n")
	
	prompt.WriteString(fmt.Sprintf("Provide up to %d suggestions, ordered by importance.\n", options.MaxSuggestions))
	
	if options.IncludePatterns {
		prompt.WriteString("Include pattern matching information where relevant.\n")
	}
	
	if options.IncludeExamples {
		prompt.WriteString("Include code examples in your suggestions.\n")
	}
	
	prompt.WriteString("\nFormat your response as structured analysis followed by numbered suggestions.\n")
	
	return prompt.String()
}

// getSystemPrompt returns the system prompt for ARF analysis
func (a *Analyzer) getSystemPrompt() string {
	return `You are an expert Java developer specializing in code migration and transformation error analysis. 
Your role is to analyze compilation and runtime errors that occur during automated code transformations 
and provide precise, actionable solutions.

Focus on:
- Clear identification of root causes
- Specific code changes needed
- Dependency version conflicts
- Framework migration patterns
- Build configuration issues

Provide concise, practical solutions that can be directly applied to fix the errors.`
}

// parseLLMResponse parses the LLM response into structured format
func (a *Analyzer) parseLLMResponse(content string) (string, []CodeSuggestion, error) {
	// This is a simplified parser - in production, implement robust parsing
	lines := strings.Split(content, "\n")
	
	var analysis strings.Builder
	var suggestions []CodeSuggestion
	
	inSuggestions := false
	currentSuggestion := CodeSuggestion{}
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		if strings.HasPrefix(line, "SUGGESTIONS:") || strings.Contains(strings.ToLower(line), "suggestions:") {
			inSuggestions = true
			continue
		}
		
		if inSuggestions {
			// Parse numbered suggestions
			if strings.HasPrefix(line, "1.") || strings.HasPrefix(line, "2.") || 
			   strings.HasPrefix(line, "3.") || strings.HasPrefix(line, "4.") ||
			   strings.HasPrefix(line, "5.") {
				if currentSuggestion.Title != "" {
					suggestions = append(suggestions, currentSuggestion)
				}
				currentSuggestion = CodeSuggestion{
					ID:         fmt.Sprintf("suggestion_%d", len(suggestions)+1),
					Type:       "fix",
					Title:      strings.TrimPrefix(line, strings.Split(line, ".")[0]+"."),
					Confidence: 0.8,
					Impact:     "medium",
					Category:   "compilation",
				}
			} else if currentSuggestion.Title != "" {
				currentSuggestion.Description += line + "\n"
			}
		} else {
			analysis.WriteString(line + "\n")
		}
	}
	
	// Add last suggestion
	if currentSuggestion.Title != "" {
		suggestions = append(suggestions, currentSuggestion)
	}
	
	return analysis.String(), suggestions, nil
}

// Helper methods

// isRelevantFile checks if a source file is relevant to the errors
func (a *Analyzer) isRelevantFile(file SourceFile, errors []ErrorDetails) bool {
	for _, err := range errors {
		if strings.Contains(err.File, file.Path) || strings.Contains(file.Path, err.File) {
			return true
		}
	}
	return false
}

// isRelevantFileEnhanced checks if a file is relevant considering dependencies and imports
func (a *Analyzer) isRelevantFileEnhanced(file SourceFile, allFiles []SourceFile, errors []ErrorDetails) bool {
	// First check if file directly contains errors
	if a.isRelevantFile(file, errors) {
		return true
	}

	// Check if file is imported by error-containing files
	for _, err := range errors {
		for _, errorFile := range allFiles {
			if strings.Contains(err.File, errorFile.Path) {
				// Check if errorFile imports this file
				if a.fileImportsFile(errorFile, file) {
					return true
				}
			}
		}
	}

	// Check if file contains classes/symbols mentioned in errors
	for _, err := range errors {
		if a.fileContainsSymbolFromError(file, err) {
			return true
		}
	}

	return false
}

// fileImportsFile checks if sourceFile imports targetFile
func (a *Analyzer) fileImportsFile(sourceFile, targetFile SourceFile) bool {
	// Extract package and class name from target file path
	targetClass := a.extractClassName(targetFile.Path)
	if targetClass == "" {
		return false
	}

	// Check if source file imports the target class
	lines := strings.Split(sourceFile.Content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "import ") && strings.Contains(line, targetClass) {
			return true
		}
	}

	return false
}

// extractClassName extracts class name from file path
func (a *Analyzer) extractClassName(filePath string) string {
	// Extract filename without extension
	filename := filePath
	if lastSlash := strings.LastIndex(filename, "/"); lastSlash >= 0 {
		filename = filename[lastSlash+1:]
	}
	if lastDot := strings.LastIndex(filename, "."); lastDot >= 0 {
		filename = filename[:lastDot]
	}
	return filename
}

// fileContainsSymbolFromError checks if file defines symbols mentioned in error
func (a *Analyzer) fileContainsSymbolFromError(file SourceFile, err ErrorDetails) bool {
	// Extract class name from error message
	if strings.Contains(err.Message, "cannot find symbol: class ") {
		className := a.extractClassNameFromError(err.Message)
		if className != "" && strings.Contains(file.Content, "class "+className) {
			return true
		}
	}
	return false
}

// extractClassNameFromError extracts class name from error message
func (a *Analyzer) extractClassNameFromError(message string) string {
	// Handle "cannot find symbol: class ClassName"
	if idx := strings.Index(message, "cannot find symbol: class "); idx >= 0 {
		className := message[idx+len("cannot find symbol: class "):]
		if spaceIdx := strings.Index(className, " "); spaceIdx >= 0 {
			className = className[:spaceIdx]
		}
		return className
	}
	return ""
}

// extractRelevantContent extracts relevant portions of source code
func (a *Analyzer) extractRelevantContent(content string, errors []ErrorDetails) string {
	// For small content, return as-is
	if len(content) <= 5000 {
		return content
	}
	
	return a.extractRelevantContentWithSemantics(content, errors)
}

// extractRelevantContentWithSemantics provides smart content extraction around errors
func (a *Analyzer) extractRelevantContentWithSemantics(content string, errors []ErrorDetails) string {
	lines := strings.Split(content, "\n")
	relevantLines := make(map[int]bool)
	
	// Always include imports and package declaration
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "package ") || strings.HasPrefix(trimmed, "import ") {
			relevantLines[i] = true
		}
	}
	
	// Include context around error lines
	for _, err := range errors {
		errorLine := err.Line - 1 // Convert to 0-based index
		if errorLine >= 0 && errorLine < len(lines) {
			// Include ±10 lines around error
			start := max(0, errorLine-10)
			end := min(len(lines), errorLine+11)
			
			for i := start; i < end; i++ {
				relevantLines[i] = true
			}
			
			// Include entire method/class containing the error
			a.includeMethodContext(lines, errorLine, relevantLines)
		}
	}
	
	// Build result from relevant lines
	var result strings.Builder
	lastIncluded := -2
	
	for i, line := range lines {
		if relevantLines[i] {
			// Add separator if we skipped lines
			if lastIncluded >= 0 && i > lastIncluded+1 {
				result.WriteString("\n// ... (lines omitted) ...\n")
			}
			
			result.WriteString(fmt.Sprintf("%d: %s\n", i+1, line))
			lastIncluded = i
		}
	}
	
	return result.String()
}

// includeMethodContext includes the entire method/class containing an error line
func (a *Analyzer) includeMethodContext(lines []string, errorLine int, relevantLines map[int]bool) {
	// Find method start (look backwards for method signature)
	methodStart := errorLine
	for i := errorLine; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if a.isMethodSignature(line) {
			methodStart = i
			break
		}
		if a.isClassDeclaration(line) {
			methodStart = i
			break
		}
	}
	
	// Find method end (look forward for closing brace at same indentation)
	methodEnd := errorLine
	braceCount := 0
	startIndent := a.getIndentation(lines[methodStart])
	
	for i := methodStart; i < len(lines); i++ {
		line := lines[i]
		
		// Count braces
		braceCount += strings.Count(line, "{")
		braceCount -= strings.Count(line, "}")
		
		// If we're back to the starting indentation level and brace count is 0
		if i > methodStart && braceCount <= 0 && a.getIndentation(line) <= startIndent {
			methodEnd = i
			break
		}
	}
	
	// Include all lines in the method
	for i := methodStart; i <= methodEnd && i < len(lines); i++ {
		relevantLines[i] = true
	}
}

// isMethodSignature checks if a line contains a method signature
func (a *Analyzer) isMethodSignature(line string) bool {
	line = strings.TrimSpace(line)
	// Look for method patterns: visibility + returnType + methodName + (
	return (strings.Contains(line, "public ") || strings.Contains(line, "private ") || 
			strings.Contains(line, "protected ") || strings.Contains(line, "static ")) &&
		   strings.Contains(line, "(") && !strings.Contains(line, "class ")
}

// isClassDeclaration checks if a line contains a class declaration
func (a *Analyzer) isClassDeclaration(line string) bool {
	line = strings.TrimSpace(line)
	return strings.Contains(line, "class ") || strings.Contains(line, "interface ") || 
		   strings.Contains(line, "enum ")
}

// getIndentation returns the number of leading whitespace characters
func (a *Analyzer) getIndentation(line string) int {
	count := 0
	for _, char := range line {
		if char == ' ' || char == '\t' {
			count++
		} else {
			break
		}
	}
	return count
}

// Helper function for min/max
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Prioritization helper methods for token optimization

// prioritizeErrors sorts errors by severity and type for better context building
func (a *Analyzer) prioritizeErrors(errors []ErrorDetails) []ErrorDetails {
	sorted := make([]ErrorDetails, len(errors))
	copy(sorted, errors)
	
	sort.Slice(sorted, func(i, j int) bool {
		// Priority: error > warning > info
		severityPriority := map[string]int{
			"error":   3,
			"warning": 2,
			"info":    1,
		}
		
		// Type priority: compilation > runtime > test
		typePriority := map[string]int{
			"compilation": 3,
			"runtime":     2,
			"test":        1,
		}
		
		iSeverity := severityPriority[sorted[i].Severity]
		jSeverity := severityPriority[sorted[j].Severity]
		
		if iSeverity != jSeverity {
			return iSeverity > jSeverity
		}
		
		iType := typePriority[sorted[i].Type]
		jType := typePriority[sorted[j].Type]
		
		return iType > jType
	})
	
	return sorted
}

// sortPatternsByConfidence sorts patterns by confidence for prioritization
func (a *Analyzer) sortPatternsByConfidence(patterns []PatternMatch) []PatternMatch {
	sorted := make([]PatternMatch, len(patterns))
	copy(sorted, patterns)
	
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Confidence > sorted[j].Confidence
	})
	
	return sorted
}

// prioritizeDependencies sorts dependencies by relevance to errors and patterns
func (a *Analyzer) prioritizeDependencies(deps []Dependency, errors []ErrorDetails, patterns []PatternMatch) []Dependency {
	if len(deps) == 0 {
		return deps
	}
	
	type DepWithScore struct {
		dep   Dependency
		score int
	}
	
	depScores := make([]DepWithScore, len(deps))
	for i, dep := range deps {
		depScores[i] = DepWithScore{dep: dep, score: 0}
	}
	
	// Score based on error relevance
	for _, err := range errors {
		errorLower := strings.ToLower(err.Message)
		for i := range depScores {
			dep := &depScores[i]
			groupLower := strings.ToLower(dep.dep.GroupID)
			artifactLower := strings.ToLower(dep.dep.ArtifactID)
			
			// High relevance if error mentions this dependency
			if strings.Contains(errorLower, groupLower) || strings.Contains(errorLower, artifactLower) {
				dep.score += 10
			}
			
			// Medium relevance for common framework dependencies
			if a.isFrameworkDependency(dep.dep) {
				dep.score += 5
			}
		}
	}
	
	// Score based on pattern matches
	for _, pattern := range patterns {
		for i := range depScores {
			dep := &depScores[i]
			if a.dependencyMatchesPattern(dep.dep, pattern) {
				dep.score += int(pattern.Confidence * 10)
			}
		}
	}
	
	// Sort by score
	sort.Slice(depScores, func(i, j int) bool {
		return depScores[i].score > depScores[j].score
	})
	
	result := make([]Dependency, len(deps))
	for i, depScore := range depScores {
		result[i] = depScore.dep
	}
	
	return result
}

// prioritizeSourceFiles sorts source files by relevance to errors
func (a *Analyzer) prioritizeSourceFiles(files []SourceFile, errors []ErrorDetails) []SourceFile {
	if len(files) == 0 {
		return files
	}
	
	type FileWithScore struct {
		file  SourceFile
		score int
	}
	
	fileScores := make([]FileWithScore, len(files))
	for i, file := range files {
		fileScores[i] = FileWithScore{file: file, score: 0}
	}
	
	// Score based on error relevance
	for _, err := range errors {
		for i := range fileScores {
			file := &fileScores[i]
			
			// Highest relevance: file directly contains error
			if strings.Contains(err.File, file.file.Path) || strings.Contains(file.file.Path, err.File) {
				file.score += 20
			}
			
			// Medium relevance: file might be related (import analysis)
			if a.fileContainsSymbolFromError(file.file, err) {
				file.score += 10
			}
			
			// Lower relevance: same package/directory
			if a.filesInSamePackage(file.file.Path, err.File) {
				file.score += 3
			}
		}
	}
	
	// Prefer smaller files (easier to include in context)
	for i := range fileScores {
		file := &fileScores[i]
		if file.file.LineCount > 0 && file.file.LineCount < 100 {
			file.score += 2
		} else if file.file.LineCount > 500 {
			file.score -= 3
		}
	}
	
	// Sort by score
	sort.Slice(fileScores, func(i, j int) bool {
		return fileScores[i].score > fileScores[j].score
	})
	
	result := make([]SourceFile, len(files))
	for i, fileScore := range fileScores {
		result[i] = fileScore.file
	}
	
	return result
}

// Helper methods for prioritization

// isFrameworkDependency checks if dependency is a common framework
func (a *Analyzer) isFrameworkDependency(dep Dependency) bool {
	groupLower := strings.ToLower(dep.GroupID)
	artifactLower := strings.ToLower(dep.ArtifactID)
	
	frameworks := []string{
		"spring", "hibernate", "jackson", "junit", "mockito",
		"jakarta", "javax", "apache", "google", "fasterxml",
	}
	
	for _, framework := range frameworks {
		if strings.Contains(groupLower, framework) || strings.Contains(artifactLower, framework) {
			return true
		}
	}
	
	return false
}

// dependencyMatchesPattern checks if dependency is related to a pattern
func (a *Analyzer) dependencyMatchesPattern(dep Dependency, pattern PatternMatch) bool {
	depLower := strings.ToLower(dep.GroupID + " " + dep.ArtifactID)
	patternLower := strings.ToLower(pattern.PatternName)
	
	// Check for common pattern-dependency relationships
	if strings.Contains(patternLower, "javax") && strings.Contains(depLower, "jakarta") {
		return true
	}
	if strings.Contains(patternLower, "spring") && strings.Contains(depLower, "spring") {
		return true
	}
	if strings.Contains(patternLower, "dependency") && a.isFrameworkDependency(dep) {
		return true
	}
	
	return false
}

// filesInSamePackage checks if two files are in the same package
func (a *Analyzer) filesInSamePackage(file1, file2 string) bool {
	dir1 := strings.LastIndex(file1, "/")
	dir2 := strings.LastIndex(file2, "/")
	
	if dir1 == -1 || dir2 == -1 {
		return false
	}
	
	return file1[:dir1] == file2[:dir2]
}

// calculateConfidence calculates overall confidence in the analysis
func (a *Analyzer) calculateConfidence(analysis string, suggestions []CodeSuggestion, patterns []PatternMatch) float64 {
	confidence := 0.5 // Base confidence
	
	// Increase confidence based on pattern matches
	if len(patterns) > 0 {
		confidence += 0.2
	}
	
	// Increase confidence based on suggestion quality
	if len(suggestions) > 0 {
		avgSuggestionConfidence := 0.0
		for _, s := range suggestions {
			avgSuggestionConfidence += s.Confidence
		}
		avgSuggestionConfidence /= float64(len(suggestions))
		confidence += avgSuggestionConfidence * 0.3
	}
	
	// Cap at maximum confidence
	if confidence > 0.95 {
		confidence = 0.95
	}
	
	return confidence
}

// enhanceResponse adds additional metadata and enhancements
func (a *Analyzer) enhanceResponse(response *ARFAnalysisResponse, req *ARFAnalysisRequest, options ARFAnalysisOptions) {
	// Add suggestion enhancements
	for i := range response.Suggestions {
		suggestion := &response.Suggestions[i]
		
		// Set file context if missing
		if suggestion.File == "" && len(req.CodeContext.SourceFiles) > 0 {
			// Try to match suggestion to relevant file
			for _, file := range req.CodeContext.SourceFiles {
				if a.suggestionRelevantToFile(suggestion, file) {
					suggestion.File = file.Path
					break
				}
			}
		}
		
		// Add reasoning if missing
		if suggestion.Reasoning == "" {
			suggestion.Reasoning = "Based on error pattern analysis and best practices"
		}
		
		// Set default impact if not specified
		if suggestion.Impact == "" {
			suggestion.Impact = "medium"
		}
	}
}

// suggestionRelevantToFile checks if a suggestion is relevant to a file
func (a *Analyzer) suggestionRelevantToFile(suggestion *CodeSuggestion, file SourceFile) bool {
	title := strings.ToLower(suggestion.Title)
	content := strings.ToLower(file.Content)
	
	// Simple relevance check - in production, use more sophisticated matching
	return strings.Contains(content, strings.ToLower(suggestion.Description)) ||
		   strings.Contains(title, strings.ToLower(file.Path))
}

// calculateQualityScore calculates overall response quality score
func (a *Analyzer) calculateQualityScore(response *ARFAnalysisResponse) float64 {
	score := response.Confidence * 0.4
	
	// Factor in suggestion count and quality
	if len(response.Suggestions) > 0 {
		score += float64(len(response.Suggestions)) * 0.1
		
		// Quality based on suggestion completeness
		completeCount := 0
		for _, s := range response.Suggestions {
			if s.Description != "" && s.Reasoning != "" {
				completeCount++
			}
		}
		score += float64(completeCount) / float64(len(response.Suggestions)) * 0.3
	}
	
	// Factor in pattern matches
	if len(response.PatternMatches) > 0 {
		score += float64(len(response.PatternMatches)) * 0.05
	}
	
	// Factor in analysis completeness
	if len(response.Analysis) > 100 {
		score += 0.15
	}
	
	if score > 1.0 {
		score = 1.0
	}
	
	return score
}

// getModelName returns the model name for the given options
func (a *Analyzer) getModelName(options ARFAnalysisOptions) string {
	if options.PreferredModel != "" {
		return options.PreferredModel
	}
	return "default"
}

// addProcessingStep adds a processing step to the response metadata
func (a *Analyzer) addProcessingStep(response *ARFAnalysisResponse, name string, startTime time.Time, status, details string) {
	step := ProcessingStep{
		Name:     name,
		Duration: time.Since(startTime),
		Status:   status,
		Details:  details,
	}
	response.Metadata.ProcessingSteps = append(response.Metadata.ProcessingSteps, step)
}