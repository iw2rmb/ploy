package arf

import (
	"time"

	"github.com/gofiber/fiber/v2"
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
	recipeExecutor   *RecipeExecutor
	recipeStorage    RecipeStorage
	recipeIndex      RecipeIndexStore
	recipeValidator  RecipeValidatorInterface
	recipeRegistry   *RecipeRegistry // Unified recipe registry
	sandboxMgr       SandboxManager
	llmGenerator     LLMRecipeGenerator
	hybridPipeline   HybridPipeline
	multiLangEngine  MultiLanguageEngine
	strategySelector StrategySelector
	// Phase 4 components
	securityEngine *SecurityEngine
	sbomAnalyzer   *SyftSBOMAnalyzer
	// Consul store for transformation status
	consulStore ConsulStoreInterface
	// Metrics for catalog operations
	metrics CatalogMetrics
}

// NewHandler creates a new ARF HTTP handler
func NewHandler(executor *RecipeExecutor, sandboxMgr SandboxManager) *Handler {
	return &Handler{
		recipeExecutor: executor,
		recipeRegistry: nil, // Will be initialized when needed
		sandboxMgr:     sandboxMgr,
		// Initialize Phase 4 components
		securityEngine: NewSecurityEngine(),
		sbomAnalyzer:   NewSyftSBOMAnalyzer(),
	}
}

// SetConsulStore sets the Consul store
func (h *Handler) SetConsulStore(store ConsulStoreInterface) { h.consulStore = store }

// RegisterPrometheusMetrics removed

// NewHandlerWithStorage creates a new ARF HTTP handler with storage backend
func NewHandlerWithStorage(
	executor *RecipeExecutor,
	recipeStorage RecipeStorage,
	recipeIndex RecipeIndexStore,
	recipeValidator RecipeValidatorInterface,
	sandboxMgr SandboxManager,
	storageProvider internalStorage.StorageProvider, // Storage provider for recipe registry
) *Handler {
	// Initialize recipe registry if we have storage provider
	var recipeRegistry *RecipeRegistry
	if storageProvider != nil {
		// Create recipe registry with the storage provider
		recipeRegistry = NewRecipeRegistry(storageProvider)
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
		sbomAnalyzer:   NewSyftSBOMAnalyzer(),
	}
}

// NewHandlerWithPhase3 creates a new ARF HTTP handler with Phase 3 components
func NewHandlerWithPhase3(
	executor *RecipeExecutor,
	sandboxMgr SandboxManager,
	llmGen LLMRecipeGenerator,
	hybrid HybridPipeline,
	multiLang MultiLanguageEngine,
	strategy StrategySelector,
) *Handler {
	return &Handler{
		recipeExecutor:   executor,
		recipeRegistry:   nil, // Will be initialized when needed
		sandboxMgr:       sandboxMgr,
		llmGenerator:     llmGen,
		hybridPipeline:   hybrid,
		multiLangEngine:  multiLang,
		strategySelector: strategy,
		// Initialize Phase 4 components
		securityEngine: NewSecurityEngine(),
		sbomAnalyzer:   NewSyftSBOMAnalyzer(),
	}
}

// RegisterRoutes registers ARF routes with the Fiber app
func (h *Handler) RegisterRoutes(app *fiber.App) {
	arf := app.Group("/v1/arf")

	// Recipe management
	arf.Get("/recipes", h.ListRecipes)
	arf.Get("/recipes/:id", h.GetRecipe)
	arf.Post("/recipes", h.CreateRecipe)
	arf.Put("/recipes/:id", h.UpdateRecipe)
	arf.Delete("/recipes/:id", h.DeleteRecipe)
	arf.Get("/recipes/search", h.SearchRecipes)
	arf.Post("/recipes/upload", h.UploadRecipe)
	arf.Post("/recipes/validate", h.ValidateRecipe)
	arf.Get("/recipes/:id/download", h.DownloadRecipe)

	// Recipe metadata and stats
	arf.Get("/recipes/:id/metadata", h.GetRecipeMetadata)
	arf.Get("/recipes/:id/stats", h.GetRecipeStats)

	// Recipe registration from OpenRewrite JVM runner
	arf.Post("/recipes/register", h.RegisterRecipeFromRunner)

	// Model registry moved to LLMS: /v1/llms/models/*

	// Legacy ARF transform HTTP endpoints have been removed in favor of Transflow

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

	// Phase 4: SBOM
	arf.Post("/sbom/generate", h.GenerateSBOM)
	arf.Post("/sbom/analyze", h.AnalyzeSBOM)
	arf.Get("/sbom/compliance", h.GetSBOMCompliance)
	arf.Get("/sbom/report", h.GetSBOMReport)
	arf.Get("/sbom/:id", h.GetSBOMReport) // Support route param

	// Healing coordinator monitoring removed
}
