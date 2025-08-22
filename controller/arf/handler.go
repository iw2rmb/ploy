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