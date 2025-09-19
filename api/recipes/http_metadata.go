package recipes

import (
	"github.com/gofiber/fiber/v2"
)

// GetRecipeMetadata returns recipe metadata.
func (h *HTTPHandler) GetRecipeMetadata(c *fiber.Ctx) error {
	recipeID := c.Params("id")
	if recipeID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Recipe ID is required",
		})
	}

	recipe, err := h.getRecipeWithStorage(c.Context(), recipeID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":   "Recipe not found",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"recipe_id":  recipeID,
		"created_at": recipe.CreatedAt,
		"updated_at": recipe.UpdatedAt,
		"author":     recipe.Metadata.Author,
		"version":    recipe.Metadata.Version,
		"tags":       recipe.Metadata.Tags,
	})
}

// GetRecipeStats returns recipe statistics.
func (h *HTTPHandler) GetRecipeStats(c *fiber.Ctx) error {
	recipeID := c.Params("id")

	stats, err := h.getRecipeStatsWithStorage(c.Context(), recipeID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to load recipe stats",
			"details": err.Error(),
		})
	}

	return c.JSON(stats)
}
