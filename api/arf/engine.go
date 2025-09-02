package arf

import (
	"time"
)

// Core ARF types for transformation engine

// Codebase represents the source code to be transformed
type Codebase struct {
	Repository string            `json:"repository"`
	Branch     string            `json:"branch"`
	Path       string            `json:"path"`
	RootPath   string            `json:"root_path"` // Root directory of the codebase
	Language   string            `json:"language"`
	BuildTool  string            `json:"build_tool"` // maven, gradle, etc.
	Metadata   map[string]string `json:"metadata"`
}

// TransformationIteration represents a single iteration in the transformation process
type TransformationIteration struct {
	Number    int                    `json:"number"`
	StartTime time.Time              `json:"start_time"`
	EndTime   time.Time              `json:"end_time"`
	Duration  time.Duration          `json:"duration"`
	Status    string                 `json:"status"` // success, partial, failed, timeout
	Stages    []TransformationStage  `json:"stages"`
	Diffs     []DiffCapture          `json:"diffs"`
	Errors    []ErrorCapture         `json:"errors"`
	Metrics   IterationMetrics       `json:"metrics"`
	LLMCalls  []LLMCallMetrics       `json:"llm_calls,omitempty"`
}

// TransformationStage represents a stage within a transformation iteration
type TransformationStage struct {
	Name      string        `json:"name"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	Status    string        `json:"status"`
	Details   interface{}   `json:"details,omitempty"`
}

// DiffCapture captures code changes made during transformation
type DiffCapture struct {
	File         string    `json:"file"`
	Type         string    `json:"type"` // added, modified, deleted
	Before       string    `json:"before,omitempty"`
	After        string    `json:"after,omitempty"`
	UnifiedDiff  string    `json:"unified_diff"`
	LinesAdded   int       `json:"lines_added"`
	LinesRemoved int       `json:"lines_removed"`
	Timestamp    time.Time `json:"timestamp"`
}

// ErrorCapture captures errors during execution
type ErrorCapture struct {
	Stage      string    `json:"stage"`
	Type       string    `json:"type"` // compile, test, validation, runtime
	Message    string    `json:"message"`
	Details    string    `json:"details,omitempty"`
	StackTrace string    `json:"stack_trace,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// IterationMetrics captures metrics for an iteration
type IterationMetrics struct {
	FilesAnalyzed   int     `json:"files_analyzed"`
	FilesModified   int     `json:"files_modified"`
	LinesAdded      int     `json:"lines_added"`
	LinesRemoved    int     `json:"lines_removed"`
	CompileSuccess  bool    `json:"compile_success"`
	TestsRun        int     `json:"tests_run"`
	TestsPassed     int     `json:"tests_passed"`
	CoveragePercent float64 `json:"coverage_percent,omitempty"`
	ComplexityDelta int     `json:"complexity_delta,omitempty"`
}

// LLMCallMetrics tracks LLM usage during transformation
type LLMCallMetrics struct {
	Purpose      string        `json:"purpose"`
	Model        string        `json:"model"`
	InputTokens  int           `json:"input_tokens"`
	OutputTokens int           `json:"output_tokens"`
	Duration     time.Duration `json:"duration"`
	Cost         float64       `json:"cost,omitempty"`
	Success      bool          `json:"success"`
	Error        string        `json:"error,omitempty"`
}

// TransformationSummary provides high-level metrics for the transformation
type TransformationSummary struct {
	TotalIterations      int           `json:"total_iterations"`
	SuccessfulIterations int           `json:"successful_iterations"`
	PartialIterations    int           `json:"partial_iterations"`
	FailedIterations     int           `json:"failed_iterations"`
	AverageIterationTime time.Duration `json:"average_iteration_time"`
	TotalLLMCalls        int           `json:"total_llm_calls"`
	TotalLLMTokens       int           `json:"total_llm_tokens"`
	TotalLLMCost         float64       `json:"total_llm_cost"`
	FinalCompileStatus   bool          `json:"final_compile_status"`
	FinalTestStatus      bool          `json:"final_test_status"`
	TotalFilesModified   int           `json:"total_files_modified"`
	TotalLinesChanged    int           `json:"total_lines_changed"`
}

// ComparisonResult compares transformation runs
type ComparisonResult struct {
	BaselineRun  string                 `json:"baseline_run"`
	ComparedRuns []string               `json:"compared_runs"`
	Metrics      map[string]interface{} `json:"metrics"`
	Winner       string                 `json:"winner"`
	Analysis     string                 `json:"analysis"`
}

// RepositoryInfo contains information about the transformed repository
type RepositoryInfo struct {
	URL        string            `json:"url"`
	Branch     string            `json:"branch"`
	CommitHash string            `json:"commit_hash"`
	Language   string            `json:"language"`
	BuildTool  string            `json:"build_tool"`
	Size       int64             `json:"size_bytes"`
	FileCount  int               `json:"file_count"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// BuildMetrics contains build-related metrics
type BuildMetrics struct {
	BuildTool      string        `json:"build_tool"`
	BuildCommand   string        `json:"build_command"`
	BuildSuccess   bool          `json:"build_success"`
	BuildDuration  time.Duration `json:"build_duration"`
	BuildErrors    []string      `json:"build_errors,omitempty"`
	ArtifactsCount int           `json:"artifacts_count"`
	ArtifactsSize  int64         `json:"artifacts_size_bytes"`
}

// TestMetrics contains test execution metrics
type TestMetrics struct {
	TestFramework   string        `json:"test_framework"`
	TestCommand     string        `json:"test_command"`
	TotalTests      int           `json:"total_tests"`
	PassedTests     int           `json:"passed_tests"`
	FailedTests     int           `json:"failed_tests"`
	SkippedTests    int           `json:"skipped_tests"`
	TestDuration    time.Duration `json:"test_duration"`
	CoveragePercent float64       `json:"coverage_percent,omitempty"`
	TestErrors      []string      `json:"test_errors,omitempty"`
}

// TransformationResult contains the comprehensive results of a recipe execution
type TransformationResult struct {
	// Core identification
	TransformationID string    `json:"transformation_id,omitempty"`
	RecipeID         string    `json:"recipe_id"`
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	Status           string    `json:"status,omitempty"` // pending, running, completed, failed
	
	// Basic results (backward compatible)
	Success         bool          `json:"success"`
	ChangesApplied  int           `json:"changes_applied"`
	TotalFiles      int           `json:"total_files"`
	FilesModified   []string      `json:"files_modified"`
	Diff            string        `json:"diff"` // Simple unified diff for backward compatibility
	ValidationScore float64       `json:"validation_score"`
	ExecutionTime   time.Duration `json:"execution_time"`
	Error           string        `json:"error,omitempty"` // Error message if failed
	
	// Legacy error tracking (backward compatible)
	Errors   []TransformationError  `json:"errors,omitempty"`
	Warnings []TransformationError  `json:"warnings,omitempty"`
	
	// Comprehensive reporting (new fields)
	Iterations   []TransformationIteration `json:"iterations,omitempty"`
	Summary      *TransformationSummary    `json:"summary,omitempty"`
	Repository   *RepositoryInfo           `json:"repository,omitempty"`
	BuildResults *BuildMetrics             `json:"build_results,omitempty"`
	TestResults  *TestMetrics              `json:"test_results,omitempty"`
	LLMUsage     []LLMCallMetrics          `json:"llm_usage,omitempty"`
	DiffCaptures []DiffCapture             `json:"diff_captures,omitempty"`
	ErrorLog     []ErrorCapture            `json:"error_log,omitempty"`
	Comparison   *ComparisonResult         `json:"comparison,omitempty"`
	
	// Metadata
	Metadata map[string]interface{} `json:"metadata"`
}

// TransformationError represents errors and warnings during transformation
type TransformationError struct {
	Type        string `json:"type"`
	Message     string `json:"message"`
	File        string `json:"file,omitempty"`
	Line        int    `json:"line,omitempty"`
	Column      int    `json:"column,omitempty"`
	Recoverable bool   `json:"recoverable"`
}

// AST represents an abstract syntax tree for caching
type AST struct {
	FilePath string                 `json:"file_path"`
	Language string                 `json:"language"`
	Checksum string                 `json:"checksum"`
	Nodes    map[string]interface{} `json:"nodes"`
	ParsedAt time.Time              `json:"parsed_at"`
	Size     int64                  `json:"size"`
}

// ASTCacheStats provides cache performance metrics
type ASTCacheStats struct {
	Hits        int64   `json:"hits"`
	Misses      int64   `json:"misses"`
	HitRate     float64 `json:"hit_rate"`
	Size        int64   `json:"size"`
	MemoryUsage int64   `json:"memory_usage"`
}
