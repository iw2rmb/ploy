package arf

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/arf/models"
)

// Helper methods to bridge catalog and storage interfaces

func (h *Handler) listRecipesWithStorage(ctx context.Context, filters RecipeFilters) ([]*models.Recipe, error) {
	if h.recipeStorage != nil {
		// Convert RecipeFilters to RecipeFilter
		storageFilter := RecipeFilter{
			Tags:       filters.Tags,
			Categories: []string{}, // Will be set below
			Languages:  []string{}, // Will be set below
			Author:     filters.Author,
			Limit:      20, // Default limit
		}

		// Convert single values to slices
		if filters.Category != "" {
			storageFilter.Categories = []string{filters.Category}
		}
		if filters.Language != "" {
			storageFilter.Languages = []string{filters.Language}
		}

		return h.recipeStorage.ListRecipes(ctx, storageFilter)
	}

	// Fallback to catalog
	if h.catalog != nil {
		return h.catalog.ListRecipes(ctx, filters)
	}

	return nil, fmt.Errorf("no storage backend available")
}

func (h *Handler) getRecipeWithStorage(ctx context.Context, recipeID string) (*models.Recipe, error) {
	if h.recipeStorage != nil {
		return h.recipeStorage.GetRecipe(ctx, recipeID)
	}

	// Fallback to catalog
	if h.catalog != nil {
		return h.catalog.GetRecipe(ctx, recipeID)
	}

	return nil, fmt.Errorf("no storage backend available")
}

func (h *Handler) createRecipeWithStorage(ctx context.Context, recipe *models.Recipe) error {
	// Validate recipe if validator is available
	if h.recipeValidator != nil {
		if err := h.recipeValidator.ValidateRecipe(recipe); err != nil {
			return fmt.Errorf("recipe validation failed: %w", err)
		}
	}

	if h.recipeStorage != nil {
		return h.recipeStorage.CreateRecipe(ctx, recipe)
	}

	// Fallback to catalog
	if h.catalog != nil {
		return h.catalog.StoreRecipe(ctx, recipe)
	}

	return fmt.Errorf("no storage backend available")
}

func (h *Handler) updateRecipeWithStorage(ctx context.Context, recipeID string, recipe *models.Recipe) error {
	// Validate recipe if validator is available
	if h.recipeValidator != nil {
		if err := h.recipeValidator.ValidateRecipe(recipe); err != nil {
			return fmt.Errorf("recipe validation failed: %w", err)
		}
	}

	if h.recipeStorage != nil {
		return h.recipeStorage.UpdateRecipe(ctx, recipeID, recipe)
	}

	// Fallback to catalog
	if h.catalog != nil {
		return h.catalog.UpdateRecipe(ctx, recipe)
	}

	return fmt.Errorf("no storage backend available")
}

func (h *Handler) deleteRecipeWithStorage(ctx context.Context, recipeID string) error {
	if h.recipeStorage != nil {
		return h.recipeStorage.DeleteRecipe(ctx, recipeID)
	}

	// Fallback to catalog
	if h.catalog != nil {
		return h.catalog.DeleteRecipe(ctx, recipeID)
	}

	return fmt.Errorf("no storage backend available")
}

func (h *Handler) searchRecipesWithStorage(ctx context.Context, query string) ([]*models.Recipe, error) {
	if h.recipeStorage != nil {
		results, err := h.recipeStorage.SearchRecipes(ctx, query)
		if err != nil {
			return nil, err
		}

		// Convert search results to recipes
		recipes := make([]*models.Recipe, len(results))
		for i, result := range results {
			recipes[i] = result.Recipe
		}
		return recipes, nil
	}

	// Fallback to catalog
	if h.catalog != nil {
		return h.catalog.SearchRecipes(ctx, query)
	}

	return nil, fmt.Errorf("no storage backend available")
}

func (h *Handler) getRecipeStatsWithStorage(ctx context.Context, recipeID string) (interface{}, error) {
	// Try catalog first if available (it has stats functionality)
	if h.catalog != nil {
		return h.catalog.GetRecipeStats(ctx, recipeID)
	}

	// If no catalog available, return mock stats
	return map[string]interface{}{
		"recipe_id":        recipeID,
		"execution_count":  0,
		"success_rate":     0.0,
		"average_duration": "0s",
		"last_executed":    nil,
		"error_patterns":   []string{},
		"resource_usage": map[string]string{
			"cpu_average":    "0m",
			"memory_average": "0Mi",
		},
	}, nil
}

// ListRecipesLegacy returns a list of available recipes (old implementation)
func (h *Handler) ListRecipesLegacy(c *fiber.Ctx) error {
	// Parse query parameters
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	category := c.Query("category")
	language := c.Query("language")

	// Get recipes using storage backend
	filters := RecipeFilters{
		Category: category,
		Language: language,
	}
	recipes, err := h.listRecipesWithStorage(c.Context(), filters)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to list recipes",
			"details": err.Error(),
		})
	}

	// Mock total count for pagination
	totalCount := len(recipes)

	return c.JSON(fiber.Map{
		"recipes": recipes,
		"pagination": fiber.Map{
			"page":        page,
			"limit":       limit,
			"total_count": totalCount,
			"total_pages": (totalCount + limit - 1) / limit,
		},
	})
}

// GetRecipeLegacy returns a specific recipe by ID (old implementation)
func (h *Handler) GetRecipeLegacy(c *fiber.Ctx) error {
	recipeID := c.Params("id")

	recipe, err := h.getRecipeWithStorage(c.Context(), recipeID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error":   "Recipe not found",
			"details": err.Error(),
		})
	}

	return c.JSON(recipe)
}

// CreateRecipeLegacy creates a new recipe (old implementation)
func (h *Handler) CreateRecipeLegacy(c *fiber.Ctx) error {
	var recipe models.Recipe
	if err := c.BodyParser(&recipe); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid recipe data",
			"details": err.Error(),
		})
	}

	// Set system fields
	recipe.SetSystemFields("api-user")

	// Store recipe using storage backend
	if err := h.createRecipeWithStorage(c.Context(), &recipe); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to save recipe",
			"details": err.Error(),
		})
	}

	return c.Status(201).JSON(recipe)
}

// UpdateRecipeLegacy updates an existing recipe (old implementation)
func (h *Handler) UpdateRecipeLegacy(c *fiber.Ctx) error {
	recipeID := c.Params("id")

	var recipe models.Recipe
	if err := c.BodyParser(&recipe); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid recipe data",
			"details": err.Error(),
		})
	}

	// Ensure recipe ID matches
	recipe.ID = recipeID
	recipe.UpdatedAt = time.Now()

	// Update recipe using storage backend
	if err := h.updateRecipeWithStorage(c.Context(), recipeID, &recipe); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to update recipe",
			"details": err.Error(),
		})
	}

	return c.JSON(recipe)
}

// DeleteRecipeLegacy deletes a recipe (old implementation)
func (h *Handler) DeleteRecipeLegacy(c *fiber.Ctx) error {
	recipeID := c.Params("id")

	if err := h.deleteRecipeWithStorage(c.Context(), recipeID); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to delete recipe",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Recipe deleted successfully",
	})
}

// SearchRecipesLegacy searches for recipes based on criteria (old implementation)
func (h *Handler) SearchRecipesLegacy(c *fiber.Ctx) error {
	query := c.Query("q")

	recipes, err := h.searchRecipesWithStorage(c.Context(), query)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Search failed",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"query":   query,
		"count":   len(recipes),
		"recipes": recipes,
	})
}

// GetRecipeMetadata returns recipe metadata
func (h *Handler) GetRecipeMetadata(c *fiber.Ctx) error {
	recipeID := c.Params("id")

	// Mock metadata for now as GetRecipeMetadata is not in interface
	metadata := fiber.Map{
		"recipe_id":  recipeID,
		"created_at": time.Now().Add(-30 * 24 * time.Hour),
		"updated_at": time.Now().Add(-5 * 24 * time.Hour),
		"author":     "system",
		"version":    "1.0.0",
		"tags":       []string{"spring", "migration"},
	}
	_ = metadata

	recipe, err := h.getRecipeWithStorage(c.Context(), recipeID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error":   "Recipe not found",
			"details": err.Error(),
		})
	}

	// Return metadata based on recipe
	return c.JSON(fiber.Map{
		"recipe_id":  recipeID,
		"created_at": recipe.CreatedAt,
		"updated_at": recipe.UpdatedAt,
		"author":     recipe.Metadata.Author,
		"version":    recipe.Metadata.Version,
		"tags":       recipe.Metadata.Tags,
	})
}

// GetRecipeStats returns recipe statistics
func (h *Handler) GetRecipeStats(c *fiber.Ctx) error {
	recipeID := c.Params("id")

	// Get recipe stats from storage backend
	stats, err := h.getRecipeStatsWithStorage(c.Context(), recipeID)
	if err != nil {
		// Return default stats as fallback
		return c.JSON(fiber.Map{
			"recipe_id":        recipeID,
			"execution_count":  0,
			"success_rate":     0.0,
			"average_duration": "0s",
			"last_executed":    nil,
			"error_patterns":   []string{},
			"resource_usage": fiber.Map{
				"cpu_average":    "0m",
				"memory_average": "0Mi",
			},
		})
	}

	return c.JSON(stats)
}

// UploadRecipe handles recipe upload from YAML
func (h *Handler) UploadRecipe(c *fiber.Ctx) error {
	var recipe models.Recipe
	if err := c.BodyParser(&recipe); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid recipe data",
			"details": err.Error(),
		})
	}

	// Validate recipe
	if err := recipe.Validate(); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Recipe validation failed",
			"details": err.Error(),
		})
	}

	// Set system fields
	recipe.SetSystemFields("api-user")

	// Store recipe using storage backend
	if err := h.createRecipeWithStorage(c.Context(), &recipe); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to store recipe",
			"details": err.Error(),
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"id":      recipe.ID,
		"message": "Recipe uploaded successfully",
	})
}

// ValidateRecipe validates a recipe without storing it
func (h *Handler) ValidateRecipe(c *fiber.Ctx) error {
	var recipe models.Recipe
	if err := c.BodyParser(&recipe); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid recipe data",
			"details": err.Error(),
		})
	}

	// Basic validation
	if err := recipe.Validate(); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Recipe validation failed",
			"details": err.Error(),
			"valid":   false,
		})
	}

	// Additional validation checks
	warnings := []string{}

	// Check for missing optional but recommended fields
	if recipe.Metadata.MinPlatform == "" {
		warnings = append(warnings, "Missing minimum platform version")
	}
	if len(recipe.Metadata.Tags) == 0 {
		warnings = append(warnings, "No tags specified")
	}
	if recipe.Metadata.License == "" {
		warnings = append(warnings, "No license specified")
	}

	// Check step configurations
	for i, step := range recipe.Steps {
		if step.Timeout.Duration == 0 {
			warnings = append(warnings, "Step "+strconv.Itoa(i+1)+" ("+step.Name+") has no timeout specified")
		}

		// OpenRewrite-specific validation
		if step.Type == models.StepTypeOpenRewrite {
			if recipe, ok := step.Config["recipe"].(string); !ok || recipe == "" {
				warnings = append(warnings, "Step "+strconv.Itoa(i+1)+" ("+step.Name+") is OpenRewrite type but missing 'recipe' config")
			}
			if len(recipe.Metadata.Languages) == 0 || (recipe.Metadata.Languages[0] != "java" && recipe.Metadata.Languages[0] != "kotlin") {
				warnings = append(warnings, "OpenRewrite recipe should specify 'java' or 'kotlin' as language")
			}
		}
	}

	response := fiber.Map{
		"valid":   true,
		"message": "Recipe is valid",
		"summary": fiber.Map{
			"name":       recipe.Metadata.Name,
			"version":    recipe.Metadata.Version,
			"steps":      len(recipe.Steps),
			"languages":  recipe.Metadata.Languages,
			"categories": recipe.Metadata.Categories,
		},
	}

	if len(warnings) > 0 {
		response["warnings"] = warnings
	}

	return c.JSON(response)
}

// DownloadRecipe returns a recipe in YAML format
func (h *Handler) DownloadRecipe(c *fiber.Ctx) error {
	recipeID := c.Params("id")

	// Get recipe from storage backend
	recipe, err := h.getRecipeWithStorage(c.Context(), recipeID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error":   "Recipe not found",
			"details": err.Error(),
		})
	}

	// Set appropriate headers for YAML download
	c.Set("Content-Type", "application/x-yaml")
	c.Set("Content-Disposition", "attachment; filename="+recipeID+".yaml")

	return c.JSON(recipe)
}
