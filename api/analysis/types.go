package analysis

import (
	"context"
	"time"
)

// SeverityLevel represents the severity of an issue
type SeverityLevel string

const (
	SeverityCritical SeverityLevel = "critical"
	SeverityHigh     SeverityLevel = "high"
	SeverityMedium   SeverityLevel = "medium"
	SeverityLow      SeverityLevel = "low"
	SeverityInfo     SeverityLevel = "info"
)

// IssueCategory represents the category of an issue
type IssueCategory string

const (
	CategoryBug         IssueCategory = "bug"
	CategorySecurity    IssueCategory = "security"
	CategoryPerformance IssueCategory = "performance"
	CategoryMaintenance IssueCategory = "maintenance"
	CategoryStyle       IssueCategory = "style"
	CategoryComplexity  IssueCategory = "complexity"
	CategoryDeprecation IssueCategory = "deprecation"
)

// Repository represents a code repository to analyze
type Repository struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	URL      string            `json:"url"`
	Branch   string            `json:"branch"`
	Commit   string            `json:"commit"`
	Language string            `json:"language"`
	Metadata map[string]string `json:"metadata"`
}

// Codebase represents the code to be analyzed
type Codebase struct {
	Repository Repository        `json:"repository"`
	RootPath   string            `json:"root_path"`
	Files      []string          `json:"files"`
	Languages  map[string]int    `json:"languages"` // language -> file count
	Size       int64             `json:"size"`      // total size in bytes
	Metadata   map[string]string `json:"metadata"`
}

// AnalyzerInfo provides information about an analyzer
type AnalyzerInfo struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Language     string   `json:"language"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
}

// FixSuggestion represents a suggested fix for an issue
type FixSuggestion struct {
	Description string                 `json:"description"`
	Diff        string                 `json:"diff"`
	Confidence  float64                `json:"confidence"`
	ARFRecipe   string                 `json:"arf_recipe,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Issue represents a detected issue in the code
type Issue struct {
	ID             string          `json:"id"`
	Severity       SeverityLevel   `json:"severity"`
	Category       IssueCategory   `json:"category"`
	RuleName       string          `json:"rule_name"`
	Message        string          `json:"message"`
	File           string          `json:"file"`
	Line           int             `json:"line"`
	Column         int             `json:"column"`
	EndLine        int             `json:"end_line,omitempty"`
	EndColumn      int             `json:"end_column,omitempty"`
	CodeSnippet    string          `json:"code_snippet,omitempty"`
	FixSuggestions []FixSuggestion `json:"fix_suggestions,omitempty"`
	ARFCompatible  bool            `json:"arf_compatible"`
	Documentation  string          `json:"documentation,omitempty"`
	Tags           []string        `json:"tags,omitempty"`
}

// ARFTrigger represents a trigger for ARF remediation
type ARFTrigger struct {
	IssueID     string                 `json:"issue_id"`
	RecipeName  string                 `json:"recipe_name"`
	Priority    int                    `json:"priority"`
	AutoApprove bool                   `json:"auto_approve"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// AnalysisMetrics contains metrics about the analysis
type AnalysisMetrics struct {
	TotalFiles       int            `json:"total_files"`
	AnalyzedFiles    int            `json:"analyzed_files"`
	SkippedFiles     int            `json:"skipped_files"`
	TotalIssues      int            `json:"total_issues"`
	IssuesBySeverity map[string]int `json:"issues_by_severity"`
	IssuesByCategory map[string]int `json:"issues_by_category"`
	AnalysisTime     time.Duration  `json:"analysis_time"`
	CacheHits        int            `json:"cache_hits"`
	CacheMisses      int            `json:"cache_misses"`
}

// LanguageAnalysisResult represents analysis results for a specific language
type LanguageAnalysisResult struct {
	Language  string          `json:"language"`
	Analyzer  string          `json:"analyzer"`
	Issues    []Issue         `json:"issues"`
	Metrics   AnalysisMetrics `json:"metrics"`
	Success   bool            `json:"success"`
	Error     string          `json:"error,omitempty"`
	StartTime time.Time       `json:"start_time"`
	EndTime   time.Time       `json:"end_time"`
}

// AnalysisResult represents the complete analysis result
type AnalysisResult struct {
	ID              string                             `json:"id"`
	Repository      Repository                         `json:"repository"`
	Timestamp       time.Time                          `json:"timestamp"`
	OverallScore    float64                            `json:"overall_score"`
	LanguageResults map[string]*LanguageAnalysisResult `json:"language_results"`
	Issues          []Issue                            `json:"issues"`
	Metrics         AnalysisMetrics                    `json:"metrics"`
	ARFTriggers     []ARFTrigger                       `json:"arf_triggers"`
	Success         bool                               `json:"success"`
	Error           string                             `json:"error,omitempty"`
}

// AnalysisConfig represents configuration for analysis
type AnalysisConfig struct {
	Enabled         bool                   `json:"enabled" yaml:"enabled"`
	FailOnCritical  bool                   `json:"fail_on_critical" yaml:"fail_on_critical"`
	ARFIntegration  bool                   `json:"arf_integration" yaml:"arf_integration"`
	MaxIssues       int                    `json:"max_issues" yaml:"max_issues"`
	Timeout         time.Duration          `json:"timeout" yaml:"timeout"`
	CacheEnabled    bool                   `json:"cache_enabled" yaml:"cache_enabled"`
	CacheTTL        time.Duration          `json:"cache_ttl" yaml:"cache_ttl"`
	Languages       map[string]interface{} `json:"languages" yaml:"languages"`
	ExcludePatterns []string               `json:"exclude_patterns" yaml:"exclude_patterns"`
	IncludePatterns []string               `json:"include_patterns" yaml:"include_patterns"`
	CustomRules     []string               `json:"custom_rules" yaml:"custom_rules"`
}

// AnalysisRequest represents a request to analyze code
type AnalysisRequest struct {
	Repository Repository     `json:"repository"`
	Config     AnalysisConfig `json:"config"`
	Languages  []string       `json:"languages,omitempty"`
	FixIssues  bool           `json:"fix_issues"`
	DryRun     bool           `json:"dry_run"`
}

// AnalysisEngine interface defines the core analysis engine
type AnalysisEngine interface {
	// Core analysis operations
	AnalyzeRepository(ctx context.Context, repo Repository) (*AnalysisResult, error)
	AnalyzeCodebase(ctx context.Context, codebase Codebase, config AnalysisConfig) (*AnalysisResult, error)

	// Analyzer management
	RegisterAnalyzer(language string, analyzer LanguageAnalyzer) error
	GetAnalyzer(language string) (LanguageAnalyzer, error)
	GetSupportedLanguages() []string

	// Configuration
	ConfigureAnalysis(config AnalysisConfig) error
	GetConfiguration() AnalysisConfig
	ValidateConfiguration(config AnalysisConfig) error

	// Results and caching
	GetAnalysisResult(id string) (*AnalysisResult, error)
	ListAnalysisResults(repo Repository, limit int) ([]*AnalysisResult, error)
	ClearCache(repo Repository) error
}

// LanguageAnalyzer interface defines a language-specific analyzer
type LanguageAnalyzer interface {
	// Core analysis
	Analyze(ctx context.Context, codebase Codebase) (*LanguageAnalysisResult, error)

	// Configuration
	GetSupportedFileTypes() []string
	GetAnalyzerInfo() AnalyzerInfo
	ValidateConfiguration(config interface{}) error
	Configure(config interface{}) error

	// Fix generation
	GenerateFixSuggestions(issue Issue) ([]FixSuggestion, error)
	CanAutoFix(issue Issue) bool

	// Integration
	GetARFRecipes(issue Issue) []string
}

// CacheManager interface for analysis caching
type CacheManager interface {
	Get(key string) (*AnalysisResult, bool)
	Set(key string, result *AnalysisResult, ttl time.Duration) error
	Delete(key string) error
	Clear() error
	GetMetrics() map[string]int64
}
