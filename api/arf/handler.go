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
	catalog          RecipeCatalog
	sandboxMgr       SandboxManager
	llmGenerator     LLMRecipeGenerator
	learningSystem   LearningSystem
	hybridPipeline   HybridPipeline
	multiLangEngine  MultiLanguageEngine
	abTestFramework  ABTestFramework
	strategySelector StrategySelector
	// Phase 4 components
	securityEngine      *SecurityEngine
	sbomAnalyzer        *SyftSBOMAnalyzer
	productionOptimizer *ProductionOptimizer
	// Phase 8 components
	benchmarkManager    *BenchmarkManager
}

// NewHandler creates a new ARF HTTP handler
func NewHandler(executor *RecipeExecutor, catalog RecipeCatalog, sandboxMgr SandboxManager, benchmarkMgr *BenchmarkManager) *Handler {
	return &Handler{
		recipeExecutor: executor,
		catalog:    catalog,
		sandboxMgr: sandboxMgr,
		// Initialize Phase 4 components
		securityEngine:      NewSecurityEngine(),
		sbomAnalyzer:        NewSyftSBOMAnalyzer(),
		productionOptimizer: NewProductionOptimizer(OptimizerConfig{
			EnableCircuitBreaker: true,
			EnableCaching:        true,
		}),
		// Phase 8 components
		benchmarkManager:    benchmarkMgr,
	}
}

// NewHandlerWithStorage creates a new ARF HTTP handler with storage backend
func NewHandlerWithStorage(
	executor *RecipeExecutor,
	recipeStorage storage.RecipeStorage,
	recipeIndex storage.RecipeIndexStore,
	recipeValidator storage.RecipeValidator,
	sandboxMgr SandboxManager,
	benchmarkMgr *BenchmarkManager,
) *Handler {
	return &Handler{
		recipeExecutor:  executor,
		recipeStorage:   recipeStorage,
		recipeIndex:     recipeIndex,
		recipeValidator: recipeValidator,
		catalog:         nil, // Will use storage directly
		sandboxMgr:      sandboxMgr,
		// Initialize Phase 4 components
		securityEngine:      NewSecurityEngine(),
		sbomAnalyzer:        NewSyftSBOMAnalyzer(),
		productionOptimizer: NewProductionOptimizer(OptimizerConfig{
			EnableCircuitBreaker: true,
			EnableCaching:        true,
		}),
		// Phase 8 components
		benchmarkManager: benchmarkMgr,
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
	abTest ABTestFramework,
	strategy StrategySelector,
	benchmarkMgr *BenchmarkManager,
) *Handler {
	return &Handler{
		recipeExecutor:   executor,
		catalog:          catalog,
		sandboxMgr:       sandboxMgr,
		llmGenerator:     llmGen,
		learningSystem:   learning,
		hybridPipeline:   hybrid,
		multiLangEngine:  multiLang,
		abTestFramework:  abTest,
		strategySelector: strategy,
		// Initialize Phase 4 components
		securityEngine:      NewSecurityEngine(),
		sbomAnalyzer:        NewSyftSBOMAnalyzer(),
		productionOptimizer: NewProductionOptimizer(OptimizerConfig{
			EnableCircuitBreaker: true,
			EnableCaching:        true,
		}),
		// Phase 8 components
		benchmarkManager:    benchmarkMgr,
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

	// Transformation execution
	arf.Post("/transform", h.ExecuteTransformation)
	arf.Get("/transforms/:id", h.GetTransformationResult)

	// Sandbox management
	arf.Get("/sandboxes", h.ListSandboxes)
	arf.Post("/sandboxes", h.CreateSandbox)
	arf.Delete("/sandboxes/:id", h.DestroySandbox)

	// Health and monitoring
	arf.Get("/health", h.HealthCheck)
	arf.Get("/stats/cache", h.GetCacheStats)
	arf.Delete("/cache", h.ClearCache)

	// Circuit breaker management
	arf.Get("/circuit-breaker/stats", h.GetCircuitBreakerStats)
	arf.Post("/circuit-breaker/reset", h.ResetCircuitBreaker)
	arf.Get("/circuit-breaker/state", h.GetCircuitBreakerState)

	// Parallel resolver stats
	arf.Get("/parallel-resolver/stats", h.GetParallelResolverStats)
	arf.Put("/parallel-resolver/config", h.SetParallelResolverConfig)

	// Multi-repo orchestration
	arf.Get("/orchestration/stats", h.GetMultiRepoStats)
	arf.Post("/orchestration/batch", h.OrchestrateBatchTransformation)
	arf.Get("/orchestration/:id/status", h.GetOrchestrationStatus)

	// High availability
	arf.Get("/ha/stats", h.GetHAStats)
	arf.Get("/ha/nodes", h.GetHANodes)

	// Monitoring and metrics
	arf.Get("/monitoring/metrics", h.GetMonitoringMetrics)
	arf.Get("/monitoring/alerts", h.GetActiveAlerts)

	// Pattern learning
	arf.Get("/patterns/stats", h.GetPatternLearningStats)
	arf.Get("/patterns/recommendations", h.GetPatternRecommendations)

	// Phase 3: LLM and Hybrid Intelligence
	arf.Post("/recipes/generate", h.GenerateLLMRecipe)
	arf.Post("/recipes/optimize", h.OptimizeRecipe)
	arf.Post("/transform/hybrid", h.ExecuteHybridTransformation)
	arf.Post("/strategy/select", h.SelectTransformationStrategy)
	arf.Post("/complexity/analyze", h.AnalyzeComplexity)

	// Phase 3: Learning System
	arf.Post("/learning/outcome", h.RecordTransformationOutcome)
	arf.Get("/learning/patterns", h.ExtractLearningPatterns)

	// Phase 3: A/B Testing
	arf.Post("/ab-test", h.CreateABTest)
	arf.Get("/ab-test/:id/results", h.GetABTestResults)
	arf.Post("/ab-test/:id/graduate", h.GraduateABTest)

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

	// Phase 4: Production Optimization
	arf.Post("/optimize/execution", h.OptimizeExecution)
	arf.Post("/optimize/system", h.OptimizeSystemPerformance)
	arf.Get("/optimize/metrics", h.GetPerformanceMetrics)
	arf.Get("/optimize/report", h.GetOptimizationReport)
	arf.Get("/optimize/resources", h.GetResourceUtilization)
	arf.Get("/optimize/cost", h.GetCostAnalysis)
	arf.Post("/optimize/benchmark", h.RunBenchmark)
	arf.Get("/optimize/benchmark/:id", h.GetBenchmarkResults)
	
	// Phase 4: Production Metrics (additional endpoint for test compatibility)
	arf.Get("/production/metrics", h.GetProductionMetrics)
	
	// Phase 8: RESTful Benchmark Test Suite API
	arf.Post("/benchmarks", h.RunBenchmarkSuite)                    // Create new benchmark
	arf.Get("/benchmarks", h.ListBenchmarks)                        // List all benchmarks  
	arf.Get("/benchmarks/:id", h.GetBenchmark)                      // Get benchmark details
	arf.Get("/benchmarks/:id/status", h.GetBenchmarkStatus)         // Get benchmark status
	arf.Get("/benchmarks/:id/logs", h.GetBenchmarkLogs)             // Get benchmark logs
	arf.Get("/benchmarks/:id/results", h.GetBenchmarkResults)       // Get benchmark results
	arf.Get("/benchmarks/:id/errors", h.GetBenchmarkErrors)         // Get benchmark errors
	arf.Post("/benchmarks/:id/stop", h.CancelBenchmark)             // Stop benchmark
	arf.Post("/benchmarks/:id/reports", h.GenerateBenchmarkReport)  // Generate report
	arf.Post("/benchmarks/compare", h.CompareBenchmarks)             // Compare benchmarks
}