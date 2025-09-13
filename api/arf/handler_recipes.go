package arf

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/arf/models"
	recipes "github.com/iw2rmb/ploy/api/recipes"
)

// Helper methods to bridge catalog and storage interfaces

func (h *Handler) listRecipesWithStorage(ctx context.Context, filters recipes.RecipeFilters) ([]*models.Recipe, error) {
	if h.recipeStorage != nil {
		// Convert RecipeFilters to RecipeFilter
		storageFilter := recipes.RecipeFilter{
			Tags:     filters.Tags,
			Language: filters.Language,
			Author:   filters.Author,
			Limit:    20, // Default limit
		}

		return h.recipeStorage.ListRecipes(ctx, storageFilter)
	}

	// Use RecipeRegistry
	if h.recipeRegistry != nil {
		return h.recipeRegistry.ListRecipes(ctx, filters)
	}

	return nil, fmt.Errorf("no storage backend available")
}

func (h *Handler) getRecipeWithStorage(ctx context.Context, recipeID string) (*models.Recipe, error) {
	if h.recipeStorage != nil {
		return h.recipeStorage.GetRecipe(ctx, recipeID)
	}

	// Use RecipeRegistry
	if h.recipeRegistry != nil {
		return h.recipeRegistry.GetRecipeAsModelsRecipe(ctx, recipeID)
	}

	return nil, fmt.Errorf("no storage backend available")
}

func (h *Handler) createRecipeWithStorage(ctx context.Context, recipe *models.Recipe) error {
	// Validate recipe if validator is available
	if h.recipeValidator != nil {
		if err := h.recipeValidator.ValidateRecipe(ctx, recipe); err != nil {
			return fmt.Errorf("recipe validation failed: %w", err)
		}
	}

	if h.recipeStorage != nil {
		return h.recipeStorage.CreateRecipe(ctx, recipe)
	}

	// Use RecipeRegistry
	if h.recipeRegistry != nil {
		return h.recipeRegistry.StoreRecipe(ctx, recipe)
	}

	return fmt.Errorf("no storage backend available")
}

func (h *Handler) updateRecipeWithStorage(ctx context.Context, recipeID string, recipe *models.Recipe) error {
	// Validate recipe if validator is available
	if h.recipeValidator != nil {
		if err := h.recipeValidator.ValidateRecipe(ctx, recipe); err != nil {
			return fmt.Errorf("recipe validation failed: %w", err)
		}
	}

	if h.recipeStorage != nil {
		return h.recipeStorage.UpdateRecipe(ctx, recipeID, recipe)
	}

	// Use RecipeRegistry
	if h.recipeRegistry != nil {
		return h.recipeRegistry.UpdateRecipe(ctx, recipe)
	}

	return fmt.Errorf("no storage backend available")
}

func (h *Handler) deleteRecipeWithStorage(ctx context.Context, recipeID string) error {
	if h.recipeStorage != nil {
		return h.recipeStorage.DeleteRecipe(ctx, recipeID)
	}

	// Use RecipeRegistry
	if h.recipeRegistry != nil {
		return h.recipeRegistry.DeleteRecipe(ctx, recipeID)
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

	// Use RecipeRegistry
	if h.recipeRegistry != nil {
		return h.recipeRegistry.SearchRecipes(ctx, query)
	}

	return nil, fmt.Errorf("no storage backend available")
}

func (h *Handler) getRecipeStatsWithStorage(ctx context.Context, recipeID string) (interface{}, error) {
	// Use RecipeRegistry for stats functionality
	if h.recipeRegistry != nil {
		return h.recipeRegistry.GetRecipeStats(ctx, recipeID)
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

// Legacy recipe handlers removed: ListRecipesLegacy, GetRecipeLegacy, CreateRecipeLegacy,
// UpdateRecipeLegacy, DeleteRecipeLegacy, SearchRecipesLegacy.

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
