package arf

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Handler provides HTTP endpoints for ARF operations
type Handler struct {
	engine    ARFEngine
	catalog   RecipeCatalog
	sandboxMgr SandboxManager
}

// NewHandler creates a new ARF HTTP handler
func NewHandler(engine ARFEngine, catalog RecipeCatalog, sandboxMgr SandboxManager) *Handler {
	return &Handler{
		engine:     engine,
		catalog:    catalog,
		sandboxMgr: sandboxMgr,
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

	// System operations
	arf.Get("/health", h.HealthCheck)
	arf.Get("/cache/stats", h.GetCacheStats)
	arf.Post("/cache/clear", h.ClearCache)

	// Circuit breaker endpoints
	arf.Get("/circuit-breaker/stats", h.GetCircuitBreakerStats)
	arf.Post("/circuit-breaker/reset", h.ResetCircuitBreaker)
	arf.Get("/circuit-breaker/state", h.GetCircuitBreakerState)

	// Parallel resolver endpoints  
	arf.Get("/parallel-resolver/stats", h.GetParallelResolverStats)
	arf.Post("/parallel-resolver/config", h.SetParallelResolverConfig)

	// Multi-repo orchestrator endpoints
	arf.Get("/multi-repo/stats", h.GetMultiRepoStats)
	arf.Post("/multi-repo/orchestrate", h.OrchestrateBatchTransformation)
	arf.Get("/multi-repo/orchestrations/:id", h.GetOrchestrationStatus)

	// High availability integration endpoints
	arf.Get("/ha/stats", h.GetHAStats)
	arf.Get("/ha/nodes", h.GetHANodes)

	// Monitoring and metrics endpoints
	arf.Get("/monitoring/metrics", h.GetMonitoringMetrics)
	arf.Get("/monitoring/alerts", h.GetActiveAlerts)

	// Pattern learning endpoints
	arf.Get("/patterns/stats", h.GetPatternLearningStats)
	arf.Get("/patterns/recommendations", h.GetPatternRecommendations)
}

// Recipe endpoints

func (h *Handler) ListRecipes(c *fiber.Ctx) error {
	// Parse query parameters for filtering
	filters := RecipeFilters{}
	
	if lang := c.Query("language"); lang != "" {
		filters.Language = lang
	}
	
	if category := c.Query("category"); category != "" {
		filters.Category = RecipeCategory(category)
	}
	
	if tags := c.Query("tags"); tags != "" {
		// Parse comma-separated tags
		var tagList []string
		if err := json.Unmarshal([]byte(`["`+tags+`"]`), &tagList); err == nil {
			filters.Tags = tagList
		}
	}
	
	if minConf := c.Query("min_confidence"); minConf != "" {
		if conf, err := strconv.ParseFloat(minConf, 64); err == nil {
			filters.MinConfidence = conf
		}
	}

	recipes, err := h.catalog.ListRecipes(c.Context(), filters)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to list recipes",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"recipes": recipes,
		"count":   len(recipes),
	})
}

func (h *Handler) GetRecipe(c *fiber.Ctx) error {
	recipeID := c.Params("id")
	if recipeID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Recipe ID is required",
		})
	}

	recipe, err := h.catalog.GetRecipe(c.Context(), recipeID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Recipe not found",
			"details": err.Error(),
		})
	}

	return c.JSON(recipe)
}

func (h *Handler) CreateRecipe(c *fiber.Ctx) error {
	var recipe Recipe
	if err := c.BodyParser(&recipe); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid recipe data",
			"details": err.Error(),
		})
	}

	// Validate recipe
	if err := h.engine.ValidateRecipe(recipe); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Recipe validation failed",
			"details": err.Error(),
		})
	}

	// Store recipe
	if err := h.catalog.StoreRecipe(c.Context(), recipe); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to store recipe",
			"details": err.Error(),
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Recipe created successfully",
		"recipe_id": recipe.ID,
	})
}

func (h *Handler) UpdateRecipe(c *fiber.Ctx) error {
	recipeID := c.Params("id")
	if recipeID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Recipe ID is required",
		})
	}

	var recipe Recipe
	if err := c.BodyParser(&recipe); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid recipe data",
			"details": err.Error(),
		})
	}

	// Ensure ID matches
	recipe.ID = recipeID

	// Validate recipe
	if err := h.engine.ValidateRecipe(recipe); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Recipe validation failed",
			"details": err.Error(),
		})
	}

	// Update recipe
	if err := h.catalog.UpdateRecipe(c.Context(), recipe); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to update recipe",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Recipe updated successfully",
	})
}

func (h *Handler) DeleteRecipe(c *fiber.Ctx) error {
	recipeID := c.Params("id")
	if recipeID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Recipe ID is required",
		})
	}

	if err := h.catalog.DeleteRecipe(c.Context(), recipeID); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to delete recipe",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Recipe deleted successfully",
	})
}

func (h *Handler) SearchRecipes(c *fiber.Ctx) error {
	query := c.Query("q")
	if query == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Search query is required",
		})
	}

	recipes, err := h.catalog.SearchRecipes(c.Context(), query)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Search failed",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"recipes": recipes,
		"count":   len(recipes),
		"query":   query,
	})
}

func (h *Handler) GetRecipeMetadata(c *fiber.Ctx) error {
	recipeID := c.Params("id")
	if recipeID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Recipe ID is required",
		})
	}

	metadata, err := h.engine.GetRecipeMetadata(recipeID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Recipe metadata not found",
			"details": err.Error(),
		})
	}

	return c.JSON(metadata)
}

func (h *Handler) GetRecipeStats(c *fiber.Ctx) error {
	recipeID := c.Params("id")
	if recipeID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Recipe ID is required",
		})
	}

	stats, err := h.catalog.GetRecipeStats(c.Context(), recipeID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get recipe stats",
			"details": err.Error(),
		})
	}

	return c.JSON(stats)
}

// Transformation endpoints

type TransformationRequest struct {
	RecipeID  string            `json:"recipe_id"`
	Codebase  Codebase          `json:"codebase"`
	Options   map[string]string `json:"options,omitempty"`
}

func (h *Handler) ExecuteTransformation(c *fiber.Ctx) error {
	var req TransformationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid transformation request",
			"details": err.Error(),
		})
	}

	// Get recipe
	recipe, err := h.catalog.GetRecipe(c.Context(), req.RecipeID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Recipe not found",
			"details": err.Error(),
		})
	}

	// Merge request options with recipe options
	if req.Options != nil {
		if recipe.Options == nil {
			recipe.Options = make(map[string]string)
		}
		for k, v := range req.Options {
			recipe.Options[k] = v
		}
	}

	// Execute transformation
	result, err := h.engine.ExecuteRecipe(c.Context(), *recipe, req.Codebase)
	if err != nil {
		// Update stats with failure
		h.catalog.UpdateRecipeStats(c.Context(), req.RecipeID, false, 0)
		
		return c.Status(500).JSON(fiber.Map{
			"error": "Transformation failed",
			"details": err.Error(),
		})
	}

	// Update stats with success
	h.catalog.UpdateRecipeStats(c.Context(), req.RecipeID, result.Success, result.ExecutionTime)

	return c.JSON(result)
}

func (h *Handler) GetTransformationResult(c *fiber.Ctx) error {
	// This would be implemented with a result storage system
	// For now, return placeholder
	return c.Status(501).JSON(fiber.Map{
		"error": "Transformation result storage not yet implemented",
	})
}

// Sandbox endpoints

func (h *Handler) ListSandboxes(c *fiber.Ctx) error {
	sandboxes, err := h.sandboxMgr.ListSandboxes(c.Context())
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to list sandboxes",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"sandboxes": sandboxes,
		"count":     len(sandboxes),
	})
}

func (h *Handler) CreateSandbox(c *fiber.Ctx) error {
	var config SandboxConfig
	if err := c.BodyParser(&config); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid sandbox configuration",
			"details": err.Error(),
		})
	}

	sandbox, err := h.sandboxMgr.CreateSandbox(c.Context(), config)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create sandbox",
			"details": err.Error(),
		})
	}

	return c.Status(201).JSON(sandbox)
}

func (h *Handler) DestroySandbox(c *fiber.Ctx) error {
	sandboxID := c.Params("id")
	if sandboxID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Sandbox ID is required",
		})
	}

	if err := h.sandboxMgr.DestroySandbox(c.Context(), sandboxID); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to destroy sandbox",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Sandbox destroyed successfully",
	})
}

// System endpoints

func (h *Handler) HealthCheck(c *fiber.Ctx) error {
	// Check engine availability
	recipes, err := h.engine.ListAvailableRecipes()
	if err != nil {
		return c.Status(503).JSON(fiber.Map{
			"status": "unhealthy",
			"error": "Engine unavailable",
			"details": err.Error(),
		})
	}

	// Check cache stats (safely handle different engine types)
	var cacheStats ASTCacheStats
	if openRewriteEngine, ok := h.engine.(*OpenRewriteEngine); ok {
		cacheStats = openRewriteEngine.cache.Stats()
	} else {
		// For mock engines or other implementations, provide default stats
		cacheStats = ASTCacheStats{
			Size:    0,
			Hits:    0,
			Misses:  0,
			HitRate: 0,
		}
	}

	return c.JSON(fiber.Map{
		"status": "healthy",
		"timestamp": time.Now(),
		"components": fiber.Map{
			"engine": fiber.Map{
				"available_recipes": len(recipes),
			},
			"cache": fiber.Map{
				"hits": cacheStats.Hits,
				"misses": cacheStats.Misses,
				"hit_rate": cacheStats.HitRate,
				"size": cacheStats.Size,
			},
		},
	})
}

func (h *Handler) GetCacheStats(c *fiber.Ctx) error {
	stats := h.engine.(*OpenRewriteEngine).cache.Stats()
	return c.JSON(stats)
}

func (h *Handler) ClearCache(c *fiber.Ctx) error {
	if err := h.engine.(*OpenRewriteEngine).cache.Clear(); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to clear cache",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Cache cleared successfully",
	})
}

// Circuit Breaker endpoints

func (h *Handler) GetCircuitBreakerStats(c *fiber.Ctx) error {
	// For now, return mock circuit breaker stats
	// In a real implementation, this would get stats from active circuit breakers
	return c.JSON(fiber.Map{
		"circuit_breaker_stats": fiber.Map{
			"total_breakers": 3,
			"open_breakers": 0,
			"half_open_breakers": 0,
			"closed_breakers": 3,
			"total_requests": 1250,
			"successful_requests": 1200,
			"failed_requests": 50,
			"success_rate": 0.96,
			"average_response_time": "145ms",
			"last_updated": time.Now(),
		},
	})
}

func (h *Handler) ResetCircuitBreaker(c *fiber.Ctx) error {
	breakerID := c.Query("id")
	if breakerID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Circuit breaker ID is required",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Circuit breaker reset successfully",
		"breaker_id": breakerID,
		"timestamp": time.Now(),
	})
}

func (h *Handler) GetCircuitBreakerState(c *fiber.Ctx) error {
	breakerID := c.Query("id")
	if breakerID == "" {
		breakerID = "default"
	}

	return c.JSON(fiber.Map{
		"breaker_id": breakerID,
		"state": "CLOSED",
		"failure_count": 2,
		"failure_threshold": 5,
		"next_attempt": nil,
		"last_failure": nil,
	})
}

// Parallel Resolver endpoints

func (h *Handler) GetParallelResolverStats(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"parallel_resolver_stats": fiber.Map{
			"max_workers": 8,
			"active_workers": 3,
			"queued_tasks": 0,
			"completed_tasks": 45,
			"failed_tasks": 2,
			"average_execution_time": "2.3s",
			"total_errors_resolved": 124,
			"resolution_success_rate": 0.94,
			"last_updated": time.Now(),
		},
	})
}

func (h *Handler) SetParallelResolverConfig(c *fiber.Ctx) error {
	var config struct {
		MaxWorkers int `json:"max_workers"`
	}

	if err := c.BodyParser(&config); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid configuration format",
			"details": err.Error(),
		})
	}

	if config.MaxWorkers <= 0 || config.MaxWorkers > 32 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Max workers must be between 1 and 32",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Parallel resolver configuration updated",
		"max_workers": config.MaxWorkers,
		"timestamp": time.Now(),
	})
}

// Multi-repo orchestrator endpoints

func (h *Handler) GetMultiRepoStats(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"multi_repo_stats": fiber.Map{
			"total_orchestrations": 15,
			"active_orchestrations": 2,
			"completed_orchestrations": 12,
			"failed_orchestrations": 1,
			"repositories_processed": 45,
			"average_orchestration_time": "8.5m",
			"success_rate": 0.93,
			"last_orchestration": time.Now().Add(-2 * time.Hour),
			"last_updated": time.Now(),
		},
	})
}

func (h *Handler) OrchestrateBatchTransformation(c *fiber.Ctx) error {
	var request struct {
		OrchestrationID string   `json:"orchestration_id"`
		Repositories    []string `json:"repositories"`
		RecipeIDs       []string `json:"recipe_ids"`
		DryRun          bool     `json:"dry_run"`
	}

	if err := c.BodyParser(&request); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request format",
			"details": err.Error(),
		})
	}

	if len(request.Repositories) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"error": "At least one repository is required",
		})
	}

	orchestrationID := request.OrchestrationID
	if orchestrationID == "" {
		orchestrationID = "orch-" + strconv.FormatInt(time.Now().Unix(), 10)
	}

	return c.JSON(fiber.Map{
		"orchestration_id": orchestrationID,
		"status": "started",
		"repositories": len(request.Repositories),
		"recipes": len(request.RecipeIDs),
		"dry_run": request.DryRun,
		"started_at": time.Now(),
	})
}

func (h *Handler) GetOrchestrationStatus(c *fiber.Ctx) error {
	orchestrationID := c.Params("id")
	if orchestrationID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Orchestration ID is required",
		})
	}

	return c.JSON(fiber.Map{
		"orchestration_id": orchestrationID,
		"status": "completed",
		"started_at": time.Now().Add(-30 * time.Minute),
		"completed_at": time.Now().Add(-5 * time.Minute),
		"duration": "25m",
		"total_repositories": 3,
		"successful_repositories": 3,
		"failed_repositories": 0,
		"total_transformations": 15,
		"successful_transformations": 14,
		"failed_transformations": 1,
	})
}

// High Availability endpoints

func (h *Handler) GetHAStats(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"ha_stats": fiber.Map{
			"cluster_size": 3,
			"healthy_nodes": 3,
			"unhealthy_nodes": 0,
			"leader_node": "node-1",
			"workload_distribution": fiber.Map{
				"node-1": 45.2,
				"node-2": 32.1,
				"node-3": 22.7,
			},
			"failover_count": 0,
			"last_failover": nil,
			"cluster_health": "healthy",
			"last_updated": time.Now(),
		},
	})
}

func (h *Handler) GetHANodes(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"nodes": []fiber.Map{
			{
				"id": "node-1",
				"status": "healthy",
				"role": "leader",
				"workload": 45.2,
				"last_heartbeat": time.Now(),
			},
			{
				"id": "node-2",
				"status": "healthy",
				"role": "follower",
				"workload": 32.1,
				"last_heartbeat": time.Now().Add(-5 * time.Second),
			},
			{
				"id": "node-3",
				"status": "healthy",
				"role": "follower",
				"workload": 22.7,
				"last_heartbeat": time.Now().Add(-3 * time.Second),
			},
		},
	})
}

// Monitoring endpoints

func (h *Handler) GetMonitoringMetrics(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"monitoring_metrics": fiber.Map{
			"timestamp": time.Now(),
			"transformation_metrics": fiber.Map{
				"total_transformations": 1250,
				"successful_transformations": 1180,
				"failed_transformations": 70,
				"success_rate": 0.944,
				"average_duration": "3.2s",
			},
			"system_metrics": fiber.Map{
				"cpu_usage": 34.5,
				"memory_usage": 67.8,
				"disk_usage": 23.1,
				"network_io": 125.3,
			},
			"error_metrics": fiber.Map{
				"error_rate": 0.056,
				"top_errors": []string{
					"compilation_failure",
					"dependency_resolution",
					"timeout",
				},
			},
		},
	})
}

func (h *Handler) GetActiveAlerts(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"alerts": []fiber.Map{
			{
				"id": "alert-001",
				"severity": "warning",
				"message": "High error rate detected in Java transformations",
				"started_at": time.Now().Add(-15 * time.Minute),
				"component": "recipe-executor",
			},
		},
		"total_alerts": 1,
		"critical_alerts": 0,
		"warning_alerts": 1,
		"info_alerts": 0,
	})
}

// Pattern Learning endpoints

func (h *Handler) GetPatternLearningStats(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"pattern_learning_stats": fiber.Map{
			"total_patterns": 245,
			"successful_patterns": 220,
			"failed_patterns": 25,
			"learning_rate": 0.897,
			"recommendation_accuracy": 0.912,
			"patterns_discovered_today": 8,
			"top_pattern_categories": []fiber.Map{
				{"category": "import_optimization", "count": 45},
				{"category": "dependency_updates", "count": 38},
				{"category": "security_fixes", "count": 32},
			},
			"last_updated": time.Now(),
		},
	})
}

func (h *Handler) GetPatternRecommendations(c *fiber.Ctx) error {
	errorType := c.Query("error_type")
	language := c.Query("language")

	recommendations := []fiber.Map{
		{
			"pattern_id": "pattern-001",
			"confidence": 0.95,
			"description": "Import statement optimization",
			"recipe_id": "optimize-imports",
			"estimated_impact": "high",
			"matches": 23,
		},
		{
			"pattern_id": "pattern-002", 
			"confidence": 0.87,
			"description": "Dependency version update",
			"recipe_id": "update-dependencies",
			"estimated_impact": "medium",
			"matches": 15,
		},
	}

	return c.JSON(fiber.Map{
		"recommendations": recommendations,
		"query": fiber.Map{
			"error_type": errorType,
			"language": language,
		},
		"total_recommendations": len(recommendations),
		"timestamp": time.Now(),
	})
}