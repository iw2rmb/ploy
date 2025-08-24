package arf

import (
	"github.com/gofiber/fiber/v2"
)

// Handler provides HTTP endpoints for ARF operations
type Handler struct {
	engine           ARFEngine
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
	workflowEngine      *HumanWorkflowEngine
	productionOptimizer *ProductionOptimizer
	// Phase 8 components
	benchmarkManager    *BenchmarkManager
}

// NewHandler creates a new ARF HTTP handler
func NewHandler(engine ARFEngine, catalog RecipeCatalog, sandboxMgr SandboxManager, benchmarkMgr *BenchmarkManager) *Handler {
	return &Handler{
		engine:     engine,
		catalog:    catalog,
		sandboxMgr: sandboxMgr,
		// Initialize Phase 4 components
		securityEngine:      NewSecurityEngine(),
		sbomAnalyzer:        NewSyftSBOMAnalyzer(),
		workflowEngine:      NewHumanWorkflowEngine(nil, nil, nil, nil, nil), // Will be properly initialized with dependencies
		productionOptimizer: NewProductionOptimizer(OptimizerConfig{
			EnableCircuitBreaker: true,
			EnableCaching:        true,
		}),
		// Phase 8 components
		benchmarkManager:    benchmarkMgr,
	}
}

// NewHandlerWithPhase3 creates a new ARF HTTP handler with Phase 3 components
func NewHandlerWithPhase3(
	engine ARFEngine,
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
		engine:           engine,
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
		workflowEngine:      NewHumanWorkflowEngine(nil, nil, nil, nil, nil),
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

	// Phase 4: Human Workflow
	arf.Post("/workflow", h.CreateWorkflow)
	arf.Post("/workflow/create", h.CreateWorkflow) // Alternative route for tests
	arf.Post("/workflow/:id/approve", h.ApproveWorkflow)
	arf.Post("/workflow/:id/reject", h.RejectWorkflow)
	arf.Get("/workflow/:id/history", h.GetApprovalHistory)
	arf.Delete("/workflow/:id", h.CancelWorkflow)
	arf.Get("/workflow/pending", h.GetPendingWorkflows)
	arf.Get("/workflow/:id/status", h.GetWorkflowStatus)
	arf.Put("/workflow/:id/priority", h.UpdateWorkflowPriority)
	arf.Get("/workflow/metrics", h.GetWorkflowMetrics)
	arf.Post("/workflow/:id/escalate", h.EscalateWorkflow)
	arf.Get("/workflow/templates", h.GetWorkflowTemplates)

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
	
	// Phase 8: Benchmark Test Suite
	arf.Post("/benchmark/run", h.RunBenchmarkSuite)
	arf.Get("/benchmark/status/:id", h.GetBenchmarkStatus)
	arf.Get("/benchmark/results/:id", h.GetBenchmarkResults)
	arf.Get("/benchmark/errors/:id", h.GetBenchmarkErrors)
	arf.Post("/benchmark/compare", h.CompareBenchmarks)
	arf.Post("/benchmark/report/:id", h.GenerateBenchmarkReport)
	arf.Get("/benchmark/list", h.ListBenchmarks)
	arf.Get("/benchmark/:id", h.GetBenchmark) // Get full benchmark details
	arf.Delete("/benchmark/:id", h.CancelBenchmark)
}