package recipes

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/recipes/models"
)

// ListRecipes lists all recipes from the registry.
func (h *HTTPHandler) ListRecipes(c *fiber.Ctx) error {
	if h.recipeRegistry == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "RecipeRegistry not available - check SeaweedFS connectivity",
		})
	}

	recipeType := c.Query("type", "")
	var (
		recipesList []*UnifiedRecipeMetadata
		err         error
	)

	if recipeType != "" {
		recipesList, err = h.recipeRegistry.QueryByType(c.Context(), recipeType)
	} else {
		recipesList, err = h.recipeRegistry.ListAllRecipes(c.Context())
	}

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to list recipes",
			"details": err.Error(),
		})
	}

	response := make([]map[string]interface{}, 0, len(recipesList))
	for _, recipe := range recipesList {
		item := map[string]interface{}{
			"id":         recipe.Metadata.ID,
			"name":       recipe.Metadata.Name,
			"version":    recipe.Metadata.Version,
			"type":       recipe.Metadata.Type,
			"source":     recipe.Metadata.Source,
			"author":     recipe.Metadata.Author,
			"tags":       recipe.Metadata.Tags,
			"categories": recipe.Metadata.Categories,
		}

		if recipe.Maven != nil {
			item["maven"] = map[string]string{
				"group":    recipe.Maven.Group,
				"artifact": recipe.Maven.Artifact,
				"version":  recipe.Maven.Version,
				"class":    recipe.Maven.Class,
			}
		}

		if recipe.Cache != nil {
			item["cached"] = true
			item["cached_at"] = recipe.Cache.StoredAt
		}

		response = append(response, item)
	}

	return c.JSON(fiber.Map{
		"recipes": response,
		"total":   len(response),
	})
}

// GetRecipe gets a specific recipe from the registry.
func (h *HTTPHandler) GetRecipe(c *fiber.Ctx) error {
	recipeID := c.Params("id")
	if recipeID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Recipe ID is required",
		})
	}

	if h.recipeRegistry == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":   "RecipeRegistry not available",
			"details": "RecipeRegistry requires SeaweedFS storage - check storage connectivity",
		})
	}

	recipe, err := h.recipeRegistry.GetRecipe(c.Context(), recipeID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":   "Recipe not found",
			"details": err.Error(),
		})
	}

	return c.JSON(recipe)
}

// CreateRecipe creates a new recipe in the registry.
func (h *HTTPHandler) CreateRecipe(c *fiber.Ctx) error {
	if h.recipeRegistry == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":   "RecipeRegistry not available",
			"details": "RecipeRegistry requires SeaweedFS storage - check storage connectivity",
		})
	}

	var recipe models.Recipe
	if err := c.BodyParser(&recipe); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid recipe data",
			"details": err.Error(),
		})
	}

	if err := recipe.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Recipe validation failed",
			"details": err.Error(),
		})
	}

	recipe.SetSystemFields("api-user")

	if err := h.recipeRegistry.StoreRecipe(c.Context(), &recipe); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to store recipe",
			"details": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":      recipe.ID,
		"message": "Recipe created successfully",
	})
}

// UpdateRecipe updates an existing recipe in the registry.
func (h *HTTPHandler) UpdateRecipe(c *fiber.Ctx) error {
	recipeID := c.Params("id")
	if recipeID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Recipe ID is required",
		})
	}

	if h.recipeRegistry == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":   "RecipeRegistry not available",
			"details": "RecipeRegistry requires SeaweedFS storage - check storage connectivity",
		})
	}

	var recipe models.Recipe
	if err := c.BodyParser(&recipe); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid recipe data",
			"details": err.Error(),
		})
	}

	recipe.SetSystemFields("api-user")
	recipe.ID = recipeID

	if err := h.recipeRegistry.UpdateRecipe(c.Context(), &recipe); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to update recipe",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Recipe updated successfully",
	})
}

// DeleteRecipe removes a recipe from the registry.
func (h *HTTPHandler) DeleteRecipe(c *fiber.Ctx) error {
	recipeID := c.Params("id")
	if recipeID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Recipe ID is required",
		})
	}

	if h.recipeRegistry == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":   "RecipeRegistry not available",
			"details": "RecipeRegistry requires SeaweedFS storage - check storage connectivity",
		})
	}

	if err := h.recipeRegistry.DeleteRecipe(c.Context(), recipeID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":   "Recipe not found",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Recipe deleted successfully",
	})
}

// SearchRecipes searches recipes by query string.
func (h *HTTPHandler) SearchRecipes(c *fiber.Ctx) error {
	query := c.Query("q")
	if strings.TrimSpace(query) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Query parameter 'q' is required",
		})
	}

	recipesList, err := h.searchRecipesWithStorage(c.Context(), query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to search recipes",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"recipes": recipesList,
		"count":   len(recipesList),
	})
}
