package analysis

import (
	"time"
)

// AnalysisRequest represents a request to analyze code
type AnalysisRequest struct {
	// Code to analyze
	Code string `json:"code"`
	
	// Language of the code (e.g., "java", "python")
	Language string `json:"language"`
	
	// ErrorContext contains compilation or runtime errors if available
	ErrorContext string `json:"error_context,omitempty"`
	
	// FilePath is the original file path if known
	FilePath string `json:"file_path,omitempty"`
	
	// Options for analysis
	Options AnalysisOptions `json:"options,omitempty"`
}

// AnalysisOptions contains options for code analysis
type AnalysisOptions struct {
	// IncludeMetrics includes code quality metrics
	IncludeMetrics bool `json:"include_metrics"`
	
	// IncludePatterns includes pattern matching results
	IncludePatterns bool `json:"include_patterns"`
	
	// MaxDepth for structure analysis
	MaxDepth int `json:"max_depth"`
	
	// Timeout for analysis
	Timeout time.Duration `json:"timeout"`
}

// AnalysisResult represents the result of code analysis
type AnalysisResult struct {
	// Structure of the code
	Structure *CodeStructure `json:"structure,omitempty"`
	
	// Patterns found in the code
	Patterns []PatternMatch `json:"patterns,omitempty"`
	
	// Metrics about the code
	Metrics *CodeMetrics `json:"metrics,omitempty"`
	
	// Issues found during analysis
	Issues []Issue `json:"issues,omitempty"`
	
	// Context for LLM prompting
	Context *LLMContext `json:"llm_context,omitempty"`
	
	// Processing time
	ProcessingTime time.Duration `json:"processing_time"`
}

// CodeStructure represents the structure of analyzed code
type CodeStructure struct {
	// Package or module name
	Package string `json:"package,omitempty"`
	
	// Imports used in the code
	Imports []Import `json:"imports,omitempty"`
	
	// Classes defined in the code
	Classes []ClassInfo `json:"classes,omitempty"`
	
	// Methods defined at the file level
	Methods []MethodInfo `json:"methods,omitempty"`
	
	// Fields or global variables
	Fields []FieldInfo `json:"fields,omitempty"`
}

// Import represents an import statement
type Import struct {
	// Path of the import (e.g., "java.util.List")
	Path string `json:"path"`
	
	// Alias if applicable
	Alias string `json:"alias,omitempty"`
	
	// IsStatic for static imports (Java)
	IsStatic bool `json:"is_static,omitempty"`
	
	// Line number where import appears
	Line int `json:"line"`
}

// ClassInfo represents information about a class
type ClassInfo struct {
	// Name of the class
	Name string `json:"name"`
	
	// Type (class, interface, enum, etc.)
	Type string `json:"type"`
	
	// Modifiers (public, private, static, etc.)
	Modifiers []string `json:"modifiers,omitempty"`
	
	// Extends clause
	Extends string `json:"extends,omitempty"`
	
	// Implements clauses
	Implements []string `json:"implements,omitempty"`
	
	// Methods in the class
	Methods []MethodInfo `json:"methods,omitempty"`
	
	// Fields in the class
	Fields []FieldInfo `json:"fields,omitempty"`
	
	// Line range where class is defined
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}

// MethodInfo represents information about a method
type MethodInfo struct {
	// Name of the method
	Name string `json:"name"`
	
	// Return type
	ReturnType string `json:"return_type,omitempty"`
	
	// Parameters
	Parameters []ParameterInfo `json:"parameters,omitempty"`
	
	// Modifiers (public, private, static, etc.)
	Modifiers []string `json:"modifiers,omitempty"`
	
	// Throws clauses (for Java)
	Throws []string `json:"throws,omitempty"`
	
	// Line range where method is defined
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
	
	// Cyclomatic complexity if calculated
	Complexity int `json:"complexity,omitempty"`
}

// ParameterInfo represents a method parameter
type ParameterInfo struct {
	// Name of the parameter
	Name string `json:"name"`
	
	// Type of the parameter
	Type string `json:"type"`
	
	// IsVarargs for variable argument parameters
	IsVarargs bool `json:"is_varargs,omitempty"`
}

// FieldInfo represents a field or variable
type FieldInfo struct {
	// Name of the field
	Name string `json:"name"`
	
	// Type of the field
	Type string `json:"type"`
	
	// Modifiers (public, private, static, final, etc.)
	Modifiers []string `json:"modifiers,omitempty"`
	
	// Initial value if present
	InitialValue string `json:"initial_value,omitempty"`
	
	// Line where field is defined
	Line int `json:"line"`
}

// PatternMatch represents a matched pattern in the code
type PatternMatch struct {
	// PatternID is the identifier of the matched pattern
	PatternID string `json:"pattern_id"`
	
	// Name of the pattern
	Name string `json:"name"`
	
	// Description of what was found
	Description string `json:"description"`
	
	// Category of the pattern (error, warning, suggestion)
	Category string `json:"category"`
	
	// Severity level (critical, high, medium, low)
	Severity string `json:"severity"`
	
	// Confidence in the match (0-1)
	Confidence float64 `json:"confidence"`
	
	// Location where pattern was found
	Location Location `json:"location"`
	
	// Suggestion for fixing the issue
	Suggestion string `json:"suggestion,omitempty"`
	
	// Context around the match
	Context string `json:"context,omitempty"`
}

// Location represents a location in code
type Location struct {
	// File path if known
	File string `json:"file,omitempty"`
	
	// Line number (1-indexed)
	Line int `json:"line"`
	
	// Column number (1-indexed)
	Column int `json:"column,omitempty"`
	
	// End line for ranges
	EndLine int `json:"end_line,omitempty"`
	
	// End column for ranges
	EndColumn int `json:"end_column,omitempty"`
}

// Issue represents an issue found during analysis
type Issue struct {
	// Type of issue (syntax_error, compilation_error, etc.)
	Type string `json:"type"`
	
	// Message describing the issue
	Message string `json:"message"`
	
	// Severity level
	Severity string `json:"severity"`
	
	// Location of the issue
	Location Location `json:"location"`
	
	// Rule that triggered the issue
	Rule string `json:"rule,omitempty"`
}

// CodeMetrics contains various code quality metrics
type CodeMetrics struct {
	// Lines of code (excluding comments and blank lines)
	LinesOfCode int `json:"lines_of_code"`
	
	// Total lines including everything
	TotalLines int `json:"total_lines"`
	
	// Number of comments
	CommentLines int `json:"comment_lines"`
	
	// Number of classes
	ClassCount int `json:"class_count"`
	
	// Number of methods
	MethodCount int `json:"method_count"`
	
	// Average cyclomatic complexity
	AverageComplexity float64 `json:"average_complexity"`
	
	// Maximum cyclomatic complexity
	MaxComplexity int `json:"max_complexity"`
	
	// Code duplication percentage
	DuplicationRatio float64 `json:"duplication_ratio,omitempty"`
}

// LLMContext represents context prepared for LLM processing
type LLMContext struct {
	// Summary of the code structure
	Summary string `json:"summary"`
	
	// Relevant code snippets
	CodeSnippets []CodeSnippet `json:"code_snippets"`
	
	// Error context if applicable
	ErrorContext string `json:"error_context,omitempty"`
	
	// Suggested focus areas for LLM
	FocusAreas []string `json:"focus_areas"`
	
	// Token count for context
	TokenCount int `json:"token_count"`
	
	// Truncated indicates if context was truncated to fit limits
	Truncated bool `json:"truncated"`
}

// CodeSnippet represents a relevant code snippet
type CodeSnippet struct {
	// Description of why this snippet is relevant
	Description string `json:"description"`
	
	// The code snippet
	Code string `json:"code"`
	
	// Location in the original code
	Location Location `json:"location"`
	
	// Relevance score (0-1)
	Relevance float64 `json:"relevance"`
}

// ErrorPattern represents a pattern for matching errors
type ErrorPattern struct {
	// ID is a unique identifier for the pattern
	ID string `json:"id"`
	
	// Name of the pattern
	Name string `json:"name"`
	
	// Description of what this pattern matches
	Description string `json:"description"`
	
	// Category (compilation, runtime, logic, etc.)
	Category string `json:"category"`
	
	// Language this pattern applies to
	Language string `json:"language"`
	
	// Regex pattern to match
	Pattern string `json:"pattern"`
	
	// Keywords to look for
	Keywords []string `json:"keywords"`
	
	// Severity when matched
	Severity string `json:"severity"`
	
	// Suggestion template for fixes
	SuggestionTemplate string `json:"suggestion_template"`
	
	// Example of the error
	Example string `json:"example,omitempty"`
}

// ContextConfig contains configuration for context building
type ContextConfig struct {
	// MaxTokens is the maximum number of tokens for context
	MaxTokens int `json:"max_tokens"`
	
	// IncludeImports whether to include import statements
	IncludeImports bool `json:"include_imports"`
	
	// IncludeComments whether to include comments
	IncludeComments bool `json:"include_comments"`
	
	// FocusOnErrors whether to prioritize error-related code
	FocusOnErrors bool `json:"focus_on_errors"`
	
	// ContextRadius lines of context around important sections
	ContextRadius int `json:"context_radius"`
}

// ValidationResult represents the result of code validation
type ValidationResult struct {
	// Valid indicates if the code is valid
	Valid bool `json:"valid"`
	
	// Errors found during validation
	Errors []ValidationError `json:"errors,omitempty"`
	
	// Warnings found during validation
	Warnings []ValidationWarning `json:"warnings,omitempty"`
	
	// Suggestions for improvement
	Suggestions []string `json:"suggestions,omitempty"`
}

// ValidationError represents a validation error
type ValidationError struct {
	// Type of error
	Type string `json:"type"`
	
	// Message describing the error
	Message string `json:"message"`
	
	// Location of the error
	Location Location `json:"location"`
	
	// CanAutoFix indicates if this can be automatically fixed
	CanAutoFix bool `json:"can_auto_fix"`
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
	// Type of warning
	Type string `json:"type"`
	
	// Message describing the warning
	Message string `json:"message"`
	
	// Location of the warning
	Location Location `json:"location"`
}