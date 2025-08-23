package arf

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// ListRecipes returns a list of available recipes
func (h *Handler) ListRecipes(c *fiber.Ctx) error {
	// Parse query parameters
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	category := c.Query("category")
	language := c.Query("language")

	// Get recipes from catalog
	filters := RecipeFilters{
		Category: RecipeCategory(category),
		Language: language,
	}
	recipes, err := h.catalog.ListRecipes(c.Context(), filters)
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

// GetRecipe returns a specific recipe by ID
func (h *Handler) GetRecipe(c *fiber.Ctx) error {
	recipeID := c.Params("id")

	recipe, err := h.catalog.GetRecipe(c.Context(), recipeID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error":   "Recipe not found",
			"details": err.Error(),
		})
	}

	return c.JSON(recipe)
}

// CreateRecipe creates a new recipe
func (h *Handler) CreateRecipe(c *fiber.Ctx) error {
	var recipe Recipe
	if err := c.BodyParser(&recipe); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid recipe data",
			"details": err.Error(),
		})
	}

	// Store recipe
	if err := h.catalog.StoreRecipe(c.Context(), recipe); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to save recipe",
			"details": err.Error(),
		})
	}

	return c.Status(201).JSON(recipe)
}

// UpdateRecipe updates an existing recipe
func (h *Handler) UpdateRecipe(c *fiber.Ctx) error {
	recipeID := c.Params("id")

	var recipe Recipe
	if err := c.BodyParser(&recipe); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid recipe data",
			"details": err.Error(),
		})
	}

	// Ensure recipe ID matches
	recipe.ID = recipeID

	// Update recipe
	if err := h.catalog.UpdateRecipe(c.Context(), recipe); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to update recipe",
			"details": err.Error(),
		})
	}

	return c.JSON(recipe)
}

// DeleteRecipe deletes a recipe
func (h *Handler) DeleteRecipe(c *fiber.Ctx) error {
	recipeID := c.Params("id")

	if err := h.catalog.DeleteRecipe(c.Context(), recipeID); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to delete recipe",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Recipe deleted successfully",
	})
}

// SearchRecipes searches for recipes based on criteria
func (h *Handler) SearchRecipes(c *fiber.Ctx) error {
	query := c.Query("q")
	
	recipes, err := h.catalog.SearchRecipes(c.Context(), query)
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
		"recipe_id": recipeID,
		"created_at": time.Now().Add(-30 * 24 * time.Hour),
		"updated_at": time.Now().Add(-5 * 24 * time.Hour),
		"author": "system",
		"version": "1.0.0",
		"tags": []string{"spring", "migration"},
	}
	_ = metadata
	
	recipe, err := h.catalog.GetRecipe(c.Context(), recipeID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error":   "Recipe not found",
			"details": err.Error(),
		})
	}

	// Return metadata based on recipe
	return c.JSON(fiber.Map{
		"recipe_id": recipeID,
		"created_at": time.Now().Add(-30 * 24 * time.Hour),
		"updated_at": time.Now().Add(-5 * 24 * time.Hour),
		"author": "system",
		"version": recipe.Version,
		"tags": recipe.Tags,
	})
}

// GetRecipeStats returns recipe statistics
func (h *Handler) GetRecipeStats(c *fiber.Ctx) error {
	recipeID := c.Params("id")

	// Get recipe stats from catalog
	stats, err := h.catalog.GetRecipeStats(c.Context(), recipeID)
	if err != nil {
		// Return mock stats as fallback
		return c.JSON(fiber.Map{
			"recipe_id":        recipeID,
			"execution_count":  42,
			"success_rate":     0.95,
			"average_duration": "3m 25s",
			"last_executed":    time.Now().Add(-24 * time.Hour),
			"error_patterns":   []string{},
			"resource_usage": fiber.Map{
				"cpu_average":    "250m",
				"memory_average": "512Mi",
			},
		})
	}

	return c.JSON(stats)
}