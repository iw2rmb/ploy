package arf

import (
	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/arf/storage"
)

// Handler provides HTTP endpoints for ARF operations
type Handler struct {
	recipeExecutor   *RecipeExecutor
	recipeStorage    storage.RecipeStorage
	recipeIndex      storage.RecipeIndexStore
	recipeValidator  storage.RecipeValidator
	recipeRegistry   *RecipeRegistry // Unified recipe registry
	catalog          RecipeCatalog
	sandboxMgr       SandboxManager
	llmGenerator     LLMRecipeGenerator
	learningSystem   LearningSystem
	hybridPipeline   HybridPipeline
	multiLangEngine  MultiLanguageEngine
	strategySelector StrategySelector
	// Phase 4 components
	securityEngine *SecurityEngine
	sbomAnalyzer   *SyftSBOMAnalyzer
}

// NewHandler creates a new ARF HTTP handler
func NewHandler(executor *RecipeExecutor, catalog RecipeCatalog, sandboxMgr SandboxManager) *Handler {
	return &Handler{
		recipeExecutor: executor,
		recipeRegistry: nil, // Will be initialized when needed
		catalog:        catalog,
		sandboxMgr:     sandboxMgr,
		// Initialize Phase 4 components
		securityEngine: NewSecurityEngine(),
		sbomAnalyzer:   NewSyftSBOMAnalyzer(),
	}
}

// NewHandlerWithStorage creates a new ARF HTTP handler with storage backend
func NewHandlerWithStorage(
	executor *RecipeExecutor,
	recipeStorage storage.RecipeStorage,
	recipeIndex storage.RecipeIndexStore,
	recipeValidator storage.RecipeValidator,
	sandboxMgr SandboxManager,
) *Handler {
	// Initialize recipe registry if we have storage
	var recipeRegistry *RecipeRegistry
	if recipeStorage != nil {
		// Convert storage to storage provider interface
		// For now, we'll use a wrapper or nil since the storage backend is already set
		// The registry can work without being initialized if storage is directly used
		// TODO: Create storage provider wrapper for recipe registry
		recipeRegistry = nil // Will be initialized when needed
	}
	
	return &Handler{
		recipeExecutor:  executor,
		recipeStorage:   recipeStorage,
		recipeIndex:     recipeIndex,
		recipeValidator: recipeValidator,
		recipeRegistry:  recipeRegistry,
		catalog:         nil, // Will use storage directly
		sandboxMgr:      sandboxMgr,
		// Initialize Phase 4 components
		securityEngine: NewSecurityEngine(),
		sbomAnalyzer:   NewSyftSBOMAnalyzer(),
	}
}

// NewHandlerWithPhase3 creates a new ARF HTTP handler with Phase 3 components
func NewHandlerWithPhase3(
	executor *RecipeExecutor,
	catalog RecipeCatalog,
	sandboxMgr SandboxManager,
	llmGen LLMRecipeGenerator,
	learning LearningSystem,
	hybrid HybridPipeline,
	multiLang MultiLanguageEngine,
	strategy StrategySelector,
) *Handler {
	return &Handler{
		recipeExecutor:   executor,
		catalog:          catalog,
		sandboxMgr:       sandboxMgr,
		llmGenerator:     llmGen,
		learningSystem:   learning,
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

	// Model registry management
	arf.Get("/models", h.GetModels)
	arf.Post("/models", h.AddModel)
	arf.Put("/models", h.ImportModels)
	arf.Delete("/models/:name", h.RemoveModel)
	arf.Post("/models/:name/set-default", h.SetDefaultModel)

	// Transformation execution
	arf.Post("/transform", h.ExecuteTransformation)
	arf.Get("/transforms/:id", h.GetTransformationResult)
	arf.Get("/transforms/:id/status", h.GetTransformationStatus)

	// Sandbox management
	arf.Get("/sandboxes", h.ListSandboxes)
	arf.Post("/sandboxes", h.CreateSandbox)
	arf.Delete("/sandboxes/:id", h.DestroySandbox)

	// TODO: Phase 3 LLM and Hybrid Intelligence - methods not yet implemented
	// arf.Post("/recipes/generate", h.GenerateLLMRecipe)
	// arf.Post("/transform/hybrid", h.ExecuteHybridTransformation)
	// arf.Post("/strategy/select", h.SelectTransformationStrategy)
	// arf.Post("/complexity/analyze", h.AnalyzeComplexity)

	// TODO: Phase 3 Learning System - methods not yet implemented
	// arf.Post("/learning/outcome", h.RecordTransformationOutcome)
	// arf.Get("/learning/patterns", h.ExtractLearningPatterns)

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

	// Phase 4: Workflow removed - functionality integrated into transform command
	// Phase 8: Benchmark functionality removed - integrated into transform endpoint
}
