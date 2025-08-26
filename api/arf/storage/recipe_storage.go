package storage

import (
	"context"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
)

// RecipeFilter defines filtering criteria for recipe queries
type RecipeFilter struct {
	Tags       []string
	Categories []string
	Languages  []string
	Frameworks []string
	Author     string
	MinRating  float64
	MaxAge     time.Duration
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

// IndexAction defines the type of index update
type IndexAction string

const (
	IndexActionAdd    IndexAction = "add"
	IndexActionUpdate IndexAction = "update"
	IndexActionRemove IndexAction = "remove"
)

// RecipeIndexStore provides fast query capabilities
type RecipeIndexStore interface {
	// Index Management
	BuildIndex(ctx context.Context) error
	UpdateIndex(ctx context.Context, recipe *models.Recipe, action IndexAction) error
	ClearIndex(ctx context.Context) error

	// Query Interface
	QueryByTags(ctx context.Context, tags []string) ([]string, error)
	QueryByLanguage(ctx context.Context, language string) ([]string, error)
	QueryByCategory(ctx context.Context, category string) ([]string, error)
	QueryByAuthor(ctx context.Context, author string) ([]string, error)
	FullTextSearch(ctx context.Context, query string) ([]string, error)

	// Statistics
	GetIndexStats(ctx context.Context) (*IndexStats, error)
}

// IndexStats represents index statistics
type IndexStats struct {
	TotalRecipes   int
	TotalTags      int
	TotalCategories int
	TotalLanguages int
	TotalAuthors   int
	LastUpdated    time.Time
	IndexSize      int64
}

// RecipeValidator ensures recipe safety and correctness
type RecipeValidator interface {
	// Validation methods
	ValidateRecipe(recipe *models.Recipe) error
	ValidateRecipeYAML(yamlContent []byte) (*models.Recipe, error)
	ValidateRecipeJSON(jsonContent []byte) (*models.Recipe, error)
	
	// Security validation
	ValidateSecurityRules(recipe *models.Recipe) error
	ValidateStepSecurity(step *models.RecipeStep) error
	
	// Schema validation
	ValidateSchema(recipe *models.Recipe) error
	GetSchemaVersion() string
}

// SecurityRuleSet defines security constraints
type SecurityRuleSet struct {
	AllowedCommands      []string
	ForbiddenCommands    []string
	MaxExecutionTime     time.Duration
	AllowNetworkAccess   bool
	AllowFileSystemWrite bool
	SandboxRequired      bool
	MaxMemoryUsage       int64
	MaxCPUUsage          float64
}

// RecipeStorageConfig holds configuration for recipe storage
type RecipeStorageConfig struct {
	// Storage backend configuration
	StorageType    string // "seaweedfs", "s3", "local"
	StorageURL     string
	StorageBucket  string
	StoragePrefix  string

	// Index backend configuration  
	IndexType      string // "consul", "etcd", "memory"
	IndexURL       string
	IndexPrefix    string

	// Validation configuration
	EnableValidation     bool
	EnforceSecurityRules bool
	SecurityRules        *SecurityRuleSet

	// Cache configuration
	EnableCache      bool
	CacheTTL         time.Duration
	MaxCacheSize     int64

	// Performance configuration
	MaxConcurrent    int
	RequestTimeout   time.Duration
	RetryAttempts    int
	RetryBackoff     time.Duration
}