package recipes

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/recipes/models"
)

// UploadRecipe handles recipe upload from YAML/JSON payload.
func (h *HTTPHandler) UploadRecipe(c *fiber.Ctx) error {
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

	if err := h.createRecipeWithStorage(c.Context(), &recipe); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to store recipe",
			"details": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":      recipe.ID,
		"message": "Recipe uploaded successfully",
	})
}

// ValidateRecipe validates a recipe without storing it.
func (h *HTTPHandler) ValidateRecipe(c *fiber.Ctx) error {
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
			"valid":   false,
		})
	}

	warnings := []string{}

	if recipe.Metadata.MinPlatform == "" {
		warnings = append(warnings, "Missing minimum platform version")
	}
	if len(recipe.Metadata.Tags) == 0 {
		warnings = append(warnings, "No tags specified")
	}
	if recipe.Metadata.License == "" {
		warnings = append(warnings, "No license specified")
	}

	for i, step := range recipe.Steps {
		if step.Timeout.Duration == 0 {
			warnings = append(warnings, "Step "+strconv.Itoa(i+1)+" ("+step.Name+") has no timeout specified")
		}

		if step.Type == models.StepTypeOpenRewrite {
			if value, ok := step.Config["recipe"].(string); !ok || value == "" {
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

// DownloadRecipe returns a recipe in JSON format.
func (h *HTTPHandler) DownloadRecipe(c *fiber.Ctx) error {
	recipeID := c.Params("id")

	recipe, err := h.getRecipeWithStorage(c.Context(), recipeID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":   "Recipe not found",
			"details": err.Error(),
		})
	}

	c.Set("Content-Type", "application/x-yaml")
	c.Set("Content-Disposition", "attachment; filename="+recipeID+".yaml")

	return c.JSON(recipe)
}
