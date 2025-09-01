package arf

import (
	"context"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
)

// RecipeFilter represents filter criteria for recipe queries
type RecipeFilter struct {
	Tags       []string
	Language   string
	Framework  string
	Name       string
	Version    string
	Author     string
	IsPublic   bool
	SortBy     string
	Limit      int
	Offset     int
}

// RecipeSearchResult represents a search result with relevance score
type RecipeSearchResult struct {
	Recipe *models.Recipe
	Score  float64
}

// BundleIntegrityResult represents the result of bundle integrity verification
type BundleIntegrityResult struct {
	Valid        bool
	RecipeHash   string
	ExpectedHash string
	Errors       []string
}

// IndexAction defines the type of index update
type IndexAction string

const (
	IndexActionAdd    IndexAction = "add"
	IndexActionUpdate IndexAction = "update"
	IndexActionRemove IndexAction = "remove"
)

// RecipeStorage handles persistent recipe management
type RecipeStorage interface {
	// CRUD Operations
	CreateRecipe(ctx context.Context, recipe *models.Recipe) error
	GetRecipe(ctx context.Context, id string) (*models.Recipe, error)
	GetRecipeByNameAndVersion(ctx context.Context, name, version string) (*models.Recipe, error)
	UpdateRecipe(ctx context.Context, id string, recipe *models.Recipe) error
	DeleteRecipe(ctx context.Context, id string) error

	// Query Operations
	ListRecipes(ctx context.Context, filter RecipeFilter) ([]*models.Recipe, error)
	SearchRecipes(ctx context.Context, query string) ([]*RecipeSearchResult, error)
	GetRecipeVersions(ctx context.Context, name string) ([]*models.Recipe, error)
	GetLatestRecipe(ctx context.Context, name string) (*models.Recipe, error)

	// Bulk Operations
	ImportRecipes(ctx context.Context, recipes []*models.Recipe) error
	ExportRecipes(ctx context.Context, filter RecipeFilter) ([]*models.Recipe, error)

	// Integrity Operations
	ValidateRecipe(ctx context.Context, recipe *models.Recipe) error
	CheckRecipeIntegrity(ctx context.Context, id string) error
	VerifyRecipeHash(ctx context.Context, id string, expectedHash string) (bool, error)

	// Index Operations
	RebuildIndex(ctx context.Context) error
	UpdateIndex(ctx context.Context, recipe *models.Recipe, action IndexAction) error
}

// RecipeIndexStore provides fast query capabilities
type RecipeIndexStore interface {
	// Index Management
	BuildIndex(ctx context.Context) error
	UpdateIndex(ctx context.Context, recipe *models.Recipe, action IndexAction) error
	RefreshIndex(ctx context.Context) error
	ClearIndex(ctx context.Context) error

	// Query Operations
	SearchByTags(ctx context.Context, tags []string) ([]*models.Recipe, error)
	SearchByLanguage(ctx context.Context, language string) ([]*models.Recipe, error)
	SearchByFramework(ctx context.Context, framework string) ([]*models.Recipe, error)
	SearchByName(ctx context.Context, name string) ([]*models.Recipe, error)
	FullTextSearch(ctx context.Context, query string) ([]*RecipeSearchResult, error)

	// Aggregation Operations
	GetTagCloud(ctx context.Context) (map[string]int, error)
	GetLanguageStats(ctx context.Context) (map[string]int, error)
	GetFrameworkStats(ctx context.Context) (map[string]int, error)
}

// RecipeValidatorInterface validates recipe definitions
type RecipeValidatorInterface interface {
	// Basic validation
	ValidateRecipe(ctx context.Context, recipe *models.Recipe) error
	ValidateStructure(recipe *models.Recipe) error
	ValidateTransformations(recipe *models.Recipe) error
	ValidateSecurityRules(recipe *models.Recipe) error
	ValidateDependencies(recipe *models.Recipe) error

	// Advanced validation
	ValidateAgainstSchema(recipe *models.Recipe, schema interface{}) error
	ValidateCompatibility(recipe *models.Recipe, targetVersion string) error
	ValidateSyntax(recipe *models.Recipe) error

	// Schema validation
	ValidateSchema(recipe *models.Recipe) error
	GetSchemaVersion() string
}


// RecipeStorageConfig holds configuration for recipe storage
type RecipeStorageConfig struct {
	// Storage backend configuration
	StorageType   string // "seaweedfs", "s3", "local"
	BucketName    string
	KeyPrefix     string
	CacheEnabled  bool
	CacheTTL      time.Duration

	// Index configuration
	IndexType     string // "consul", "etcd", "inmemory"
	IndexEndpoint string
	IndexPrefix   string

	// Validation configuration
	StrictMode    bool
	SchemaVersion string

	// Performance configuration
	MaxConcurrent int
	BatchSize     int
	RetryAttempts int
	RetryDelay    time.Duration
}