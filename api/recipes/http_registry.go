package recipes

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
)

// RegisterRecipeFromRunner handles recipe registration from OpenRewrite JVM runner.
func (h *HTTPHandler) RegisterRecipeFromRunner(c *fiber.Ctx) error {
	var req RecipeRegistrationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid request body",
			"details": err.Error(),
		})
	}

	if req.RecipeClass == "" || req.MavenCoords == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Missing required fields",
			"details": "recipe_class and maven_coords are required",
		})
	}

	if h.recipeRegistry == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":   "Recipe registry not available",
			"details": "RecipeRegistry requires SeaweedFS storage - check storage connectivity",
			"status":  "failed",
		})
	}

	err := h.recipeRegistry.RegisterMavenRecipe(
		c.Context(),
		req.MavenCoords,
		req.JarPath,
		req.RecipeClass,
	)
	if err != nil {
		log.Printf("[Recipe Registration] Failed to register recipe: %v", err)
		return c.JSON(fiber.Map{
			"status":  "warning",
			"message": "Recipe registration partially failed",
			"error":   err.Error(),
		})
	}

	log.Printf("[Recipe Registration] Successfully registered recipe: %s", req.RecipeClass)

	return c.JSON(fiber.Map{
		"status":        "success",
		"message":       "Recipe registered successfully",
		"recipe_class":  req.RecipeClass,
		"registered_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// RecipeRegistrationRequest represents the request from OpenRewrite JVM runner.
type RecipeRegistrationRequest struct {
	RecipeClass string `json:"recipe_class"`
	MavenCoords string `json:"maven_coords"`
	JarPath     string `json:"jar_path"`
	Source      string `json:"source"`
	Timestamp   string `json:"timestamp"`
}
