package arf

import (
	"time"
)

// ARFAnalysisRequest represents an error analysis request specifically optimized for ARF workflows
type ARFAnalysisRequest struct {
	ProjectID     string        `json:"project_id" validate:"required,min=1,max=255"`
	Errors        []ErrorDetails `json:"errors" validate:"required,min=1,dive"`
	CodeContext   CodeContext   `json:"code_context" validate:"required"`
	TransformGoal string        `json:"transform_goal" validate:"required,min=1,max=1000"`
	AttemptNumber int           `json:"attempt_number" validate:"min=1,max=10"`
	History       []AttemptInfo `json:"history,omitempty" validate:"max=5,dive"`
}

// ErrorDetails represents a specific error that occurred during transformation
type ErrorDetails struct {
	Type        string `json:"type" validate:"required,oneof=compilation runtime test"`
	Message     string `json:"message" validate:"required,min=1,max=5000"`
	File        string `json:"file" validate:"required"`
	Line        int    `json:"line" validate:"min=1"`
	Column      int    `json:"column,omitempty" validate:"min=0"`
	Severity    string `json:"severity" validate:"oneof=error warning info"`
	Code        string `json:"code,omitempty" validate:"max=100"`
	Context     string `json:"context,omitempty" validate:"max=2000"`
}

// CodeContext represents the relevant code context for error analysis
type CodeContext struct {
	Language         string            `json:"language" validate:"required,oneof=java kotlin scala groovy"`
	FrameworkVersion string            `json:"framework_version,omitempty"`
	Dependencies     []Dependency      `json:"dependencies,omitempty" validate:"max=50,dive"`
	SourceFiles      []SourceFile      `json:"source_files" validate:"required,min=1,max=20,dive"`
	BuildTool        string            `json:"build_tool,omitempty" validate:"oneof=maven gradle ant"`
	ProjectStructure ProjectStructure  `json:"project_structure,omitempty"`
	Environment      map[string]string `json:"environment,omitempty" validate:"max=20"`
}

// Dependency represents a project dependency
type Dependency struct {
	GroupID    string `json:"group_id" validate:"required"`
	ArtifactID string `json:"artifact_id" validate:"required"`
	Version    string `json:"version" validate:"required"`
	Scope      string `json:"scope,omitempty" validate:"oneof=compile runtime test provided"`
}

// SourceFile represents a source code file with relevant content
type SourceFile struct {
	Path         string `json:"path" validate:"required"`
	Language     string `json:"language" validate:"required"`
	Content      string `json:"content" validate:"required,max=50000"`
	LineCount    int    `json:"line_count" validate:"min=1"`
	ModifiedTime string `json:"modified_time,omitempty"`
	Encoding     string `json:"encoding,omitempty" validate:"oneof=utf-8 ascii iso-8859-1"`
}

// ProjectStructure represents the overall project structure
type ProjectStructure struct {
	Type        string   `json:"type" validate:"oneof=maven gradle ant simple"`
	SourceDirs  []string `json:"source_dirs,omitempty"`
	TestDirs    []string `json:"test_dirs,omitempty"`
	ResourceDirs []string `json:"resource_dirs,omitempty"`
	OutputDir   string   `json:"output_dir,omitempty"`
}

// AttemptInfo represents information about previous transformation attempts
type AttemptInfo struct {
	AttemptNumber  int               `json:"attempt_number" validate:"min=1"`
	Timestamp      time.Time         `json:"timestamp" validate:"required"`
	ErrorsFixed    int               `json:"errors_fixed" validate:"min=0"`
	NewErrors      int               `json:"new_errors" validate:"min=0"`
	Suggestions    []string          `json:"suggestions,omitempty" validate:"max=10"`
	Duration       time.Duration     `json:"duration" validate:"min=0"`
	Status         string            `json:"status" validate:"required,oneof=success partial_success failure timeout"`
	LLMProvider    string            `json:"llm_provider,omitempty"`
	ModelUsed      string            `json:"model_used,omitempty"`
	TokensUsed     int               `json:"tokens_used,omitempty" validate:"min=0"`
	Metadata       map[string]string `json:"metadata,omitempty" validate:"max=10"`
}

// ARFAnalysisResponse represents the response from ARF error analysis
type ARFAnalysisResponse struct {
	Analysis       string            `json:"analysis"`
	Suggestions    []CodeSuggestion  `json:"suggestions"`
	Confidence     float64           `json:"confidence"`
	PatternMatches []PatternMatch    `json:"pattern_matches"`
	Metadata       ResponseMetadata  `json:"metadata"`
	Status         string            `json:"status"`
	ProcessingTime time.Duration     `json:"processing_time"`
}

// CodeSuggestion represents a specific code change suggestion
type CodeSuggestion struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"` // "fix", "improvement", "refactor", "dependency"
	Title       string            `json:"title"`
	Description string            `json:"description"`
	File        string            `json:"file"`
	StartLine   int               `json:"start_line"`
	EndLine     int               `json:"end_line"`
	OldCode     string            `json:"old_code,omitempty"`
	NewCode     string            `json:"new_code,omitempty"`
	Confidence  float64           `json:"confidence"`
	Impact      string            `json:"impact"` // "low", "medium", "high"
	Category    string            `json:"category"` // "syntax", "logic", "dependency", "configuration"
	Reasoning   string            `json:"reasoning"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// PatternMatch represents a detected error pattern
type PatternMatch struct {
	PatternID   string            `json:"pattern_id"`
	PatternName string            `json:"pattern_name"`
	Confidence  float64           `json:"confidence"`
	Description string            `json:"description"`
	Category    string            `json:"category"` // "common", "framework", "migration", "deprecation"
	Frequency   int               `json:"frequency"` // How often this pattern appears
	Resolution  PatternResolution `json:"resolution"`
	Examples    []string          `json:"examples,omitempty"`
	References  []string          `json:"references,omitempty"`
}

// PatternResolution provides resolution information for a detected pattern
type PatternResolution struct {
	Strategy    string   `json:"strategy"` // "replace", "update", "remove", "configure"
	Steps       []string `json:"steps"`
	Complexity  string   `json:"complexity"` // "low", "medium", "high"
	Automated   bool     `json:"automated"`
	RiskLevel   string   `json:"risk_level"` // "low", "medium", "high"
	TestingTips []string `json:"testing_tips,omitempty"`
}

// ResponseMetadata contains metadata about the analysis response
type ResponseMetadata struct {
	RequestID       string            `json:"request_id"`
	ModelUsed       string            `json:"model_used"`
	LLMProvider     string            `json:"llm_provider"`
	TokensUsed      int               `json:"tokens_used"`
	ProcessingSteps []ProcessingStep  `json:"processing_steps"`
	QualityScore    float64           `json:"quality_score"`
	CacheHit        bool              `json:"cache_hit"`
	Version         string            `json:"version"`
	Timestamp       time.Time         `json:"timestamp"`
	Environment     string            `json:"environment"`
	Warnings        []string          `json:"warnings,omitempty"`
	Debug           map[string]string `json:"debug,omitempty"`
}

// ProcessingStep represents a step in the analysis processing pipeline
type ProcessingStep struct {
	Name      string        `json:"name"`
	Duration  time.Duration `json:"duration"`
	Status    string        `json:"status"` // "success", "partial", "failed", "skipped"
	Details   string        `json:"details,omitempty"`
	TokensIn  int           `json:"tokens_in,omitempty"`
	TokensOut int           `json:"tokens_out,omitempty"`
}

// ARFAnalysisOptions represents configuration options for ARF analysis
type ARFAnalysisOptions struct {
	MaxSuggestions    int           `json:"max_suggestions" validate:"min=1,max=20"`
	IncludePatterns   bool          `json:"include_patterns"`
	IncludeExamples   bool          `json:"include_examples"`
	PreferredModel    string        `json:"preferred_model,omitempty"`
	FallbackEnabled   bool          `json:"fallback_enabled"`
	TimeoutSeconds    int           `json:"timeout_seconds" validate:"min=1,max=300"`
	ContextWindow     int           `json:"context_window" validate:"min=1000,max=32000"`
	Temperature       float64       `json:"temperature" validate:"min=0,max=2"`
	CacheEnabled      bool          `json:"cache_enabled"`
	QualityThreshold  float64       `json:"quality_threshold" validate:"min=0,max=1"`
	DebugMode         bool          `json:"debug_mode"`
	IncludeMetadata   bool          `json:"include_metadata"`
}

// ARFError represents specific errors that can occur during ARF analysis
type ARFError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
	Field   string `json:"field,omitempty"`
}

// ARFErrorResponse represents an error response for ARF requests
type ARFErrorResponse struct {
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Errors    []ARFError `json:"errors,omitempty"`
	RequestID string    `json:"request_id"`
	Timestamp time.Time `json:"timestamp"`
}

// Validation constants
const (
	MaxProjectIDLength   = 255
	MaxErrorMessage      = 5000
	MaxErrorContext      = 2000
	MaxTransformGoal     = 1000
	MaxSourceFileContent = 50000
	MaxAttemptHistory    = 5
	MaxDependencies      = 50
	MaxSourceFiles       = 20
	MaxEnvironmentVars   = 20
	MaxSuggestions       = 20
	MaxProcessingTime    = 5 * time.Minute
	DefaultTimeout       = 30 * time.Second
)

// Common error codes for ARF analysis
const (
	ErrorCodeInvalidRequest     = "INVALID_REQUEST"
	ErrorCodeValidationFailed   = "VALIDATION_FAILED"
	ErrorCodeProcessingTimeout  = "PROCESSING_TIMEOUT"
	ErrorCodeLLMProviderError   = "LLM_PROVIDER_ERROR"
	ErrorCodeInternalError      = "INTERNAL_ERROR"
	ErrorCodeRateLimitExceeded  = "RATE_LIMIT_EXCEEDED"
	ErrorCodeInsufficientQuota = "INSUFFICIENT_QUOTA"
	ErrorCodeUnsupportedFormat  = "UNSUPPORTED_FORMAT"
)

// Default options for ARF analysis
var DefaultARFOptions = ARFAnalysisOptions{
	MaxSuggestions:   10,
	IncludePatterns:  true,
	IncludeExamples:  false,
	FallbackEnabled:  true,
	TimeoutSeconds:   30,
	ContextWindow:    8192,
	Temperature:      0.3,
	CacheEnabled:     true,
	QualityThreshold: 0.7,
	DebugMode:        false,
	IncludeMetadata:  true,
}