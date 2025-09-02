package arf

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v2"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
	"github.com/prometheus/client_golang/prometheus"
)

// Handler provides HTTP endpoints for ARF operations
type Handler struct {
	recipeExecutor   *RecipeExecutor
	recipeStorage    RecipeStorage
	recipeIndex      RecipeIndexStore
	recipeValidator  RecipeValidatorInterface
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
	// Consul store for transformation status
	consulStore ConsulStoreInterface
	// Healing coordination for parallel attempts
	healingCoordinator *HealingCoordinator
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

// SetConsulStore sets the Consul store and initializes the healing coordinator
func (h *Handler) SetConsulStore(store ConsulStoreInterface) {
	h.consulStore = store

	// Initialize healing coordinator with default config when Consul is available
	config := DefaultHealingConfig()
	h.healingCoordinator = NewHealingCoordinator(config)

	// Start the coordinator
	if err := h.healingCoordinator.Start(context.Background()); err != nil {
		fmt.Printf("Warning: Failed to start healing coordinator: %v\n", err)
	}
}

// RegisterPrometheusMetrics registers the healing metrics with the Prometheus registry
func (h *Handler) RegisterPrometheusMetrics(registry interface{}) error {
	if h.healingCoordinator == nil {
		return fmt.Errorf("healing coordinator not initialized")
	}

	exporter := h.healingCoordinator.GetMetricsExporter()
	if exporter == nil {
		return fmt.Errorf("metrics exporter not available")
	}

	// Check if registry is a Prometheus registry
	if promRegistry, ok := registry.(*prometheus.Registry); ok {
		return exporter.Register(promRegistry)
	}

	return fmt.Errorf("invalid registry type")
}

// GetHealingCoordinatorMetrics returns metrics from the healing coordinator
func (h *Handler) GetHealingCoordinatorMetrics() *HealingCoordinatorMetrics {
	if h.healingCoordinator != nil {
		metrics := h.healingCoordinator.GetMetrics()
		return &metrics
	}
	return nil
}

// GetHealingMetrics provides an HTTP endpoint for healing coordinator metrics
func (h *Handler) GetHealingMetrics(c *fiber.Ctx) error {
	if h.healingCoordinator == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":   "Healing coordinator not initialized",
			"message": "Healing coordinator requires Consul store to be configured",
		})
	}

	if !h.healingCoordinator.IsRunning() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":   "Healing coordinator not running",
			"message": "Healing coordinator is not currently active",
		})
	}

	metrics := h.healingCoordinator.GetMetrics()

	// Get active alerts
	activeAlerts := h.healingCoordinator.GetActiveAlerts()
	alertHistory := h.healingCoordinator.GetAlertHistory()

	// Create enhanced response with additional context
	response := fiber.Map{
		"coordinator_metrics": metrics,
		"status": fiber.Map{
			"running":      h.healingCoordinator.IsRunning(),
			"active_tasks": h.healingCoordinator.GetActiveTaskCount(),
		},
		"configuration": fiber.Map{
			"max_parallel_attempts": 3, // Default from config
			"max_healing_depth":     5,
			"max_total_attempts":    20,
		},
		"alerts": fiber.Map{
			"active":        activeAlerts,
			"active_count":  len(activeAlerts),
			"history_count": len(alertHistory),
		},
	}

	return c.JSON(response)
}

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
	arf.Post("/transforms", h.ExecuteTransformationAsync)
	arf.Get("/transforms/:id", h.GetTransformationResult)
	arf.Get("/transforms/:id/status", h.GetTransformationStatusAsync)

	// Transformation debugging endpoints
	arf.Get("/transforms/:id/hierarchy", h.GetTransformationHierarchy)
	arf.Get("/transforms/:id/active", h.GetActiveHealingAttempts)
	arf.Get("/transforms/:id/timeline", h.GetTransformationTimeline)
	arf.Get("/transforms/:id/analysis", h.GetTransformationAnalysis)
	arf.Get("/transforms/:id/report", h.GetTransformationReport)
	arf.Get("/transforms/orphaned", h.GetOrphanedTransformations)

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

	// Healing coordinator monitoring
	arf.Get("/healing/metrics", h.GetHealingMetrics)
}
