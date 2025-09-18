package security

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

// Handler provides HTTP endpoints for Security operations
type Handler struct {
	recipeStorage   recipes.RecipeStorage
	recipeIndex     recipes.RecipeIndexStore
	recipeValidator recipes.RecipeValidatorInterface
	recipeRegistry  *recipes.RecipeRegistry // Unified recipe registry
	// Phase 4 components
	securityEngine *SecurityEngine
	// Metrics for catalog operations
	metrics CatalogMetrics
}

// NewHandler creates a new Security HTTP handler
func NewHandler() *Handler {
	return &Handler{
		recipeRegistry: nil, // Will be initialized when needed
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

// RegisterPrometheusMetrics removed

// NewHandlerWithStorage creates a new Security HTTP handler with storage backend
func NewHandlerWithStorage(
	recipeStorage recipes.RecipeStorage,
	recipeIndex recipes.RecipeIndexStore,
	recipeValidator recipes.RecipeValidatorInterface,
	storageProvider internalStorage.StorageProvider, // Storage provider for recipe registry
) *Handler {
	// Initialize recipe registry if we have storage provider
	var recipeRegistry *recipes.RecipeRegistry
	if storageProvider != nil {
		// Create recipe registry with the storage provider
		recipeRegistry = recipes.NewRecipeRegistry(storageProvider)
	}

	return &Handler{
		recipeStorage:   recipeStorage,
		recipeIndex:     recipeIndex,
		recipeValidator: recipeValidator,
		recipeRegistry:  recipeRegistry,
		// Initialize Phase 4 components
		securityEngine: NewSecurityEngine(),
	}
}

// RegisterRoutes registers Security routes with the Fiber app
func (h *Handler) RegisterRoutes(app *fiber.App) {
	sec := app.Group("/v1/security")
	// Model registry moved to LLMS: /v1/llms/models/*

	// Legacy Security transform HTTP endpoints have been removed in favor of Mods

	// TODO: Phase 3 LLM and Hybrid Intelligence - methods not yet implemented
	// sec.Post("/recipes/generate", h.GenerateLLMRecipe)
	// sec.Post("/transform/hybrid", h.ExecuteHybridTransformation)
	// sec.Post("/strategy/select", h.SelectTransformationStrategy)
	// sec.Post("/complexity/analyze", h.AnalyzeComplexity)

	// Phase 3: A/B Testing removed - functionality deprecated

	// Phase 4: Security
	sec.Post("/scan", h.SecurityScan)
	sec.Post("/remediation", h.GenerateRemediationPlan)
	sec.Post("/remediate", h.GenerateRemediationPlan) // Alternative route for tests
	sec.Get("/report", h.GetSecurityReport)
	sec.Get("/report/:id", h.GetSecurityReport) // Support route param
	sec.Get("/compliance", h.GetComplianceStatus)

	// Healing coordinator monitoring removed
}
