package arf

import (
	"context"
	"time"
)

// ARFEngine provides the core transformation engine interface
type ARFEngine interface {
	ExecuteRecipe(ctx context.Context, recipe Recipe, codebase Codebase) (*TransformationResult, error)
	ValidateRecipe(recipe Recipe) error
	ListAvailableRecipes() ([]Recipe, error)
	GetRecipeMetadata(recipeID string) (*RecipeMetadata, error)
	CacheAST(key string, ast *AST) error
	GetCachedAST(key string) (*AST, bool)
}

// Recipe represents an OpenRewrite recipe with metadata
type Recipe struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Language    string            `json:"language"`
	Category    RecipeCategory    `json:"category"`
	Confidence  float64           `json:"confidence"`
	Options     map[string]string `json:"options"`
	Source      string            `json:"source"` // OpenRewrite class name
	Version     string            `json:"version"`
	Tags        []string          `json:"tags"`
}

// RecipeCategory defines types of transformations
type RecipeCategory string

const (
	CategoryCleanup      RecipeCategory = "cleanup"
	CategoryModernize    RecipeCategory = "modernize"
	CategorySecurity     RecipeCategory = "security"
	CategoryPerformance  RecipeCategory = "performance"
	CategoryMigration    RecipeCategory = "migration"
	CategoryStyle        RecipeCategory = "style"
	CategoryTesting      RecipeCategory = "testing"
)

// Codebase represents the source code to be transformed
type Codebase struct {
	Repository string            `json:"repository"`
	Branch     string            `json:"branch"`
	Path       string            `json:"path"`
	Language   string            `json:"language"`
	BuildTool  string            `json:"build_tool"` // maven, gradle, etc.
	Metadata   map[string]string `json:"metadata"`
}

// TransformationResult contains the results of a recipe execution
type TransformationResult struct {
	RecipeID        string                 `json:"recipe_id"`
	Success         bool                   `json:"success"`
	ChangesApplied  int                    `json:"changes_applied"`
	FilesModified   []string               `json:"files_modified"`
	Diff            string                 `json:"diff"`
	ValidationScore float64                `json:"validation_score"`
	ExecutionTime   time.Duration          `json:"execution_time"`
	Errors          []TransformationError  `json:"errors,omitempty"`
	Warnings        []TransformationError  `json:"warnings,omitempty"`
	Metadata        map[string]interface{} `json:"metadata"`
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

// RecipeMetadata provides detailed information about a recipe
type RecipeMetadata struct {
	Recipe              Recipe    `json:"recipe"`
	ApplicableLanguages []string  `json:"applicable_languages"`
	RequiredOptions     []string  `json:"required_options"`
	OptionalOptions     []string  `json:"optional_options"`
	Prerequisites       []string  `json:"prerequisites"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	UsageCount          int64     `json:"usage_count"`
	SuccessRate         float64   `json:"success_rate"`
}

// AST represents an abstract syntax tree for caching
type AST struct {
	FilePath    string                 `json:"file_path"`
	Language    string                 `json:"language"`
	Checksum    string                 `json:"checksum"`
	Nodes       map[string]interface{} `json:"nodes"`
	ParsedAt    time.Time              `json:"parsed_at"`
	Size        int64                  `json:"size"`
}

// ASTCacheStats provides cache performance metrics
type ASTCacheStats struct {
	Hits        int64   `json:"hits"`
	Misses      int64   `json:"misses"`
	HitRate     float64 `json:"hit_rate"`
	Size        int64   `json:"size"`
	MemoryUsage int64   `json:"memory_usage"`
}