package recipes

import (
	"context"
	"time"

	"github.com/iw2rmb/ploy/api/recipes/models"
)

// RecipeEvolution provides automatic recipe improvement based on failure analysis
type RecipeEvolution interface {
	AnalyzeFailure(ctx context.Context, failure TransformationFailure) (*FailureAnalysis, error)
	EvolveRecipe(ctx context.Context, recipe *models.Recipe, analysis FailureAnalysis) (*models.Recipe, error)
	ValidateEvolution(ctx context.Context, original, evolved *models.Recipe) (*EvolutionValidationResult, error)
	RollbackRecipe(ctx context.Context, recipeID string, version int) error
}

// ErrorType categorizes different types of transformation failures
type ErrorType string

const (
	ErrorRecipeMismatch      ErrorType = "recipe_mismatch"
	ErrorCompilationFailure  ErrorType = "compilation_failure"
	ErrorSemanticChange      ErrorType = "semantic_change"
	ErrorIncompleteTransform ErrorType = "incomplete_transformation"
	ErrorResourceExhaustion  ErrorType = "resource_exhaustion"
	ErrorTimeoutFailure      ErrorType = "timeout_failure"
	ErrorDependencyIssue     ErrorType = "dependency_issue"
	ErrorUnknown             ErrorType = "unknown"
)

// FailurePattern represents a pattern of transformation failures
type FailurePattern struct {
	Signature   string   `json:"signature"`
	Frequency   int      `json:"frequency"`
	Mitigations []string `json:"mitigations"`
}

// FailureAnalysis contains the analysis of a transformation failure
type FailureAnalysis struct {
	ErrorType       ErrorType              `json:"error_type"`
	RootCause       string                 `json:"root_cause"`
	SuggestedFixes  []RecipeModification   `json:"suggested_fixes"`
	Confidence      float64                `json:"confidence"`
	SimilarPatterns []FailurePattern       `json:"similar_patterns"`
	AffectedFiles   []string               `json:"affected_files"`
	ContextInfo     map[string]interface{} `json:"context_info"`
	AnalysisTime    time.Time              `json:"analysis_time"`
}

// RecipeModification describes a specific change to make to a recipe
type RecipeModification struct {
	Type          ModificationType `json:"type"`
	Target        string           `json:"target"`
	Change        string           `json:"change"`
	Justification string           `json:"justification"`
	Priority      int              `json:"priority"`
	RiskLevel     RiskLevel        `json:"risk_level"`
}

// ModificationType defines the kind of modification to apply
type ModificationType string

const (
	ModificationAddRule       ModificationType = "add_rule"
	ModificationModifyRule    ModificationType = "modify_rule"
	ModificationRemoveRule    ModificationType = "remove_rule"
	ModificationAddCondition  ModificationType = "add_condition"
	ModificationAddException  ModificationType = "add_exception"
	ModificationAdjustPattern ModificationType = "adjust_pattern"
	ModificationExtendScope   ModificationType = "extend_scope"
	ModificationReduceScope   ModificationType = "reduce_scope"
)

// TransformationFailure contains details about a failed transformation
type TransformationFailure struct {
	RecipeID      string                 `json:"recipe_id"`
	ErrorMessage  string                 `json:"error_message"`
	StackTrace    string                 `json:"stack_trace,omitempty"`
	FailedFiles   []string               `json:"failed_files"`
	Codebase      Codebase               `json:"codebase"`
	Context       map[string]interface{} `json:"context"`
	FailureTime   time.Time              `json:"failure_time"`
	OperationLogs []string               `json:"operation_logs,omitempty"`
}

// EvolutionValidationResult contains the results of recipe evolution validation
type EvolutionValidationResult struct {
	Valid           bool                      `json:"valid"`
	SafetyScore     float64                   `json:"safety_score"`
	Warnings        []string                  `json:"warnings"`
	CriticalIssues  []string                  `json:"critical_issues"`
	TestResults     []EvolutionValidationTest `json:"test_results"`
	RecommendAction ValidationAction          `json:"recommend_action"`
}

// EvolutionValidationTest represents a specific validation check
type EvolutionValidationTest struct {
	Name        string        `json:"name"`
	Status      string        `json:"status"`
	Description string        `json:"description"`
	Details     string        `json:"details,omitempty"`
	Runtime     time.Duration `json:"runtime"`
}

// ValidationAction recommends what to do with the evolved recipe
type ValidationAction string

const (
	ActionApprove       ValidationAction = "approve"
	ActionReject        ValidationAction = "reject"
	ActionRequireReview ValidationAction = "require_review"
	ActionRunTests      ValidationAction = "run_tests"
)

// RecipeVersion tracks recipe evolution history
type RecipeVersion struct {
	Version      int                        `json:"version"`
	Recipe       *models.Recipe             `json:"recipe"`
	Changes      []RecipeModification       `json:"changes"`
	Reason       string                     `json:"reason"`
	CreatedAt    time.Time                  `json:"created_at"`
	CreatedBy    string                     `json:"created_by"`
	Rollbackable bool                       `json:"rollbackable"`
	TestResults  *EvolutionValidationResult `json:"test_results,omitempty"`
}

// RecipeEvolutionConfig configures the recipe evolution behavior
type RecipeEvolutionConfig struct {
	MaxEvolutionDepth     int     `yaml:"max_evolution_depth"`
	MinConfidenceRequired float64 `yaml:"min_confidence_required"`
	EnableAutoApproval    bool    `yaml:"enable_auto_approval"`
	AutoApprovalThreshold float64 `yaml:"auto_approval_threshold"`
	RetainVersionHistory  int     `yaml:"retain_version_history"`
}

// DefaultRecipeEvolution implements the RecipeEvolution interface
type DefaultRecipeEvolution struct {
	registry   *RecipeRegistry
	validator  RecipeValidator
	versioning RecipeVersioning
	config     RecipeEvolutionConfig
}

// ErrorPattern represents a stored error pattern in the database
type ErrorPattern struct {
	ID            string       `json:"id"`
	Signature     string       `json:"signature"`
	ErrorType     string       `json:"error_type"`
	Context       ErrorContext `json:"context"`
	Solutions     []Solution   `json:"solutions"`
	Effectiveness float64      `json:"effectiveness"`
	Occurrences   int          `json:"occurrences"`
	LastSeen      time.Time    `json:"last_seen"`
	Embedding     []float64    `json:"embedding"`
}

// Solution represents a successful fix for an error pattern
type Solution struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Success     bool      `json:"success"`
	Confidence  float64   `json:"confidence"`
	AppliedAt   time.Time `json:"applied_at"`
}

// RecipeValidator interface for validating recipes
type RecipeValidator interface {
	ValidateRecipe(ctx context.Context, recipe *models.Recipe) error
}

// RecipeVersioning interface for recipe version management
type RecipeVersioning interface {
	StoreVersion(ctx context.Context, version RecipeVersion) error
	GetVersion(ctx context.Context, recipeID string, version int) (*RecipeVersion, error)
	GetNextVersion(ctx context.Context, recipeID string) int
	ListVersions(ctx context.Context, recipeID string) ([]RecipeVersion, error)
}
