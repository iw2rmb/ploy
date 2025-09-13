package arf

import (
	"time"

	"github.com/gofiber/fiber/v2"
	recipes "github.com/iw2rmb/ploy/api/recipes"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
)

// CatalogMetrics interface for tracking catalog metrics
type CatalogMetrics interface {
	RecordHit()
	RecordMiss()
	RecordValidationFailure()
	RecordSearch(duration time.Duration)
}

// Handler provides HTTP endpoints for ARF operations
type Handler struct {
	recipeExecutor  *recipes.RecipeExecutor
	recipeStorage   recipes.RecipeStorage
	recipeIndex     recipes.RecipeIndexStore
	recipeValidator recipes.RecipeValidatorInterface
	recipeRegistry  *recipes.RecipeRegistry // Unified recipe registry
	sandboxMgr      SandboxManager
	// Phase 4 components
	securityEngine *SecurityEngine
	// Consul store for transformation status
	consulStore ConsulStoreInterface
	// Metrics for catalog operations
	metrics CatalogMetrics
}

// NewHandler creates a new ARF HTTP handler
func NewHandler(executor *recipes.RecipeExecutor, sandboxMgr SandboxManager) *Handler {
	return &Handler{
		recipeExecutor: executor,
		recipeRegistry: nil, // Will be initialized when needed
		sandboxMgr:     sandboxMgr,
		// Initialize Phase 4 components
		securityEngine: NewSecurityEngine(),
	}
}

// SetCVEDatabase wires a CVE database into the security engine
func (h *Handler) SetCVEDatabase(db CVEDatabase) {
	if h != nil && h.securityEngine != nil {
		h.securityEngine.SetCVEDatabase(db)
	}
}

// SetConsulStore sets the Consul store
func (h *Handler) SetConsulStore(store ConsulStoreInterface) { h.consulStore = store }

// RegisterPrometheusMetrics removed

// NewHandlerWithStorage creates a new ARF HTTP handler with storage backend
func NewHandlerWithStorage(
	executor *recipes.RecipeExecutor,
	recipeStorage recipes.RecipeStorage,
	recipeIndex recipes.RecipeIndexStore,
	recipeValidator recipes.RecipeValidatorInterface,
	sandboxMgr SandboxManager,
	storageProvider internalStorage.StorageProvider, // Storage provider for recipe registry
) *Handler {
	// Initialize recipe registry if we have storage provider
	var recipeRegistry *recipes.RecipeRegistry
	if storageProvider != nil {
		// Create recipe registry with the storage provider
		recipeRegistry = recipes.NewRecipeRegistry(storageProvider)
	}

	return &Handler{
		recipeExecutor:  executor,
		recipeStorage:   recipeStorage,
		recipeIndex:     recipeIndex,
		recipeValidator: recipeValidator,
		recipeRegistry:  recipeRegistry,
		sandboxMgr:      sandboxMgr,
		// Initialize Phase 4 components
		securityEngine: NewSecurityEngine(),
	}
}

// RegisterRoutes registers ARF routes with the Fiber app
func (h *Handler) RegisterRoutes(app *fiber.App) {
	arf := app.Group("/v1/arf")

	// Health
	arf.Get("/health", h.Health)

	// Recipe management
	// Place specific routes before parameterized ones to avoid shadowing
	arf.Get("/recipes", h.ListRecipes)
	arf.Get("/recipes/search", h.SearchRecipes)
	arf.Post("/recipes/upload", h.UploadRecipe)
	arf.Post("/recipes/validate", h.ValidateRecipe)
	arf.Post("/recipes", h.CreateRecipe)
	arf.Get("/recipes/:id", h.GetRecipe)
	arf.Put("/recipes/:id", h.UpdateRecipe)
	arf.Delete("/recipes/:id", h.DeleteRecipe)
	arf.Get("/recipes/:id/download", h.DownloadRecipe)

	// Recipe metadata and stats
	arf.Get("/recipes/:id/metadata", h.GetRecipeMetadata)
	arf.Get("/recipes/:id/stats", h.GetRecipeStats)

	// Recipe registration from OpenRewrite JVM runner
	arf.Post("/recipes/register", h.RegisterRecipeFromRunner)

	// Model registry moved to LLMS: /v1/llms/models/*

	// Legacy ARF transform HTTP endpoints have been removed in favor of Mods

	// Sandbox management
	arf.Get("/sandboxes", h.ListSandboxes)
	arf.Post("/sandboxes", h.CreateSandbox)
	arf.Delete("/sandboxes/:id", h.DestroySandbox)

	// TODO: Phase 3 LLM and Hybrid Intelligence - methods not yet implemented
	// arf.Post("/recipes/generate", h.GenerateLLMRecipe)
	// arf.Post("/transform/hybrid", h.ExecuteHybridTransformation)
	// arf.Post("/strategy/select", h.SelectTransformationStrategy)
	// arf.Post("/complexity/analyze", h.AnalyzeComplexity)

	// Phase 3: A/B Testing removed - functionality deprecated

	// Phase 4: Security
	arf.Post("/security/scan", h.SecurityScan)
	arf.Post("/security/remediation", h.GenerateRemediationPlan)
	arf.Post("/security/remediate", h.GenerateRemediationPlan) // Alternative route for tests
	arf.Get("/security/report", h.GetSecurityReport)
	arf.Get("/security/report/:id", h.GetSecurityReport) // Support route param
	arf.Get("/security/compliance", h.GetComplianceStatus)

	// Healing coordinator monitoring removed
}
