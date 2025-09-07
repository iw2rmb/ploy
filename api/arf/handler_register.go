package arf

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/arf/models"
)

// RecipeRegistrationRequest represents the request from OpenRewrite JVM runner
type RecipeRegistrationRequest struct {
	RecipeClass string `json:"recipe_class"`
	MavenCoords string `json:"maven_coords"`
	JarPath     string `json:"jar_path"`
	Source      string `json:"source"`
	Timestamp   string `json:"timestamp"`
}

// RegisterRecipeFromRunner handles recipe registration from OpenRewrite JVM runner
func (h *Handler) RegisterRecipeFromRunner(c *fiber.Ctx) error {
	var req RecipeRegistrationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid request body",
			"details": err.Error(),
		})
	}

	// Validate required fields
	if req.RecipeClass == "" || req.MavenCoords == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Missing required fields",
			"details": "recipe_class and maven_coords are required",
		})
	}

	ctx := context.Background()

	// Ensure recipe registry is available
	if h.recipeRegistry == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":   "Recipe registry not available",
			"details": "RecipeRegistry requires SeaweedFS storage - check storage connectivity",
			"status":  "failed",
		})
	}

	// Register the Maven recipe in the unified registry
	err := h.recipeRegistry.RegisterMavenRecipe(
		ctx,
		req.MavenCoords,
		req.JarPath,
		req.RecipeClass,
	)

	if err != nil {
		log.Printf("[Recipe Registration] Failed to register recipe: %v", err)
		// Don't fail the request as this is non-critical for the runner
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

// ListRecipes lists all recipes from the registry
func (h *Handler) ListRecipes(c *fiber.Ctx) error {
	ctx := context.Background()

	// Get query parameters for filtering
	recipeType := c.Query("type", "")

	var recipes []*UnifiedRecipeMetadata
	var err error

	// Use RecipeRegistry only
	if h.recipeRegistry == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "RecipeRegistry not available - check SeaweedFS connectivity",
		})
	}

	if recipeType != "" {
		recipes, err = h.recipeRegistry.QueryByType(ctx, recipeType)
	} else {
		recipes, err = h.recipeRegistry.ListAllRecipes(ctx)
	}

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to list recipes",
			"details": err.Error(),
		})
	}

	// Transform to response format
	response := make([]map[string]interface{}, 0, len(recipes))
	for _, recipe := range recipes {
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

		// Add Maven info if available
		if recipe.Maven != nil {
			item["maven"] = map[string]string{
				"group":    recipe.Maven.Group,
				"artifact": recipe.Maven.Artifact,
				"version":  recipe.Maven.Version,
				"class":    recipe.Maven.Class,
			}
		}

		// Add cache info if available
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

// GetRecipe gets a specific recipe from the registry
func (h *Handler) GetRecipe(c *fiber.Ctx) error {
	recipeID := c.Params("id")
	if recipeID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Recipe ID is required",
		})
	}

	if h.recipeRegistry == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Recipe registry not available",
		})
	}

	ctx := context.Background()
	recipe, err := h.recipeRegistry.GetRecipe(ctx, recipeID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fmt.Sprintf("Recipe not found: %s", recipeID),
		})
	}

	// Build response
	response := map[string]interface{}{
		"id":         recipe.Metadata.ID,
		"name":       recipe.Metadata.Name,
		"version":    recipe.Metadata.Version,
		"type":       recipe.Metadata.Type,
		"source":     recipe.Metadata.Source,
		"author":     recipe.Metadata.Author,
		"tags":       recipe.Metadata.Tags,
		"categories": recipe.Metadata.Categories,
	}

	// Add Maven info if available
	if recipe.Maven != nil {
		response["maven"] = map[string]string{
			"group":    recipe.Maven.Group,
			"artifact": recipe.Maven.Artifact,
			"version":  recipe.Maven.Version,
			"class":    recipe.Maven.Class,
		}
	}

	// Add steps if available (for custom recipes)
	if len(recipe.Steps) > 0 {
		response["steps"] = recipe.Steps
	}

	// Add cache info if available
	if recipe.Cache != nil {
		response["cache"] = map[string]interface{}{
			"stored_at": recipe.Cache.StoredAt,
			"jar_path":  recipe.Cache.JarPath,
			"size":      recipe.Cache.SizeBytes,
			"hash":      recipe.Cache.Hash,
		}
	}

	return c.JSON(response)
}

// CreateRecipe creates a new recipe in the registry
func (h *Handler) CreateRecipe(c *fiber.Ctx) error {
	var req struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		Version     string              `json:"version"`
		Author      string              `json:"author"`
		Tags        []string            `json:"tags"`
		Categories  []string            `json:"categories"`
		Steps       []models.RecipeStep `json:"steps"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid request body",
			"details": err.Error(),
		})
	}

	// Create custom recipe
	recipe := &models.Recipe{
		Metadata: models.RecipeMetadata{
			Name:        req.Name,
			Description: req.Description,
			Version:     req.Version,
			Author:      req.Author,
			Tags:        req.Tags,
			Categories:  req.Categories,
		},
		Steps: req.Steps,
	}

	ctx := context.Background()

	// Initialize recipe registry if needed
	if h.recipeRegistry == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Recipe registry not available",
		})
	}

	// Register the custom recipe
	err := h.recipeRegistry.RegisterCustomRecipe(ctx, recipe)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to create recipe",
			"details": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"status":    "success",
		"message":   "Recipe created successfully",
		"recipe_id": recipe.Metadata.Name,
	})
}

// UpdateRecipe updates an existing recipe
func (h *Handler) UpdateRecipe(c *fiber.Ctx) error {
	// For now, updating recipes requires deleting and recreating
	// This could be enhanced in the future
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"error":   "Recipe updates not yet implemented",
		"message": "Please delete and recreate the recipe with new content",
	})
}

// DeleteRecipe deletes a recipe from the registry
func (h *Handler) DeleteRecipe(c *fiber.Ctx) error {
	recipeID := c.Params("id")
	if recipeID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Recipe ID is required",
		})
	}

	// For now, deletion is not implemented in the registry
	// This could be enhanced in the future
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"error":   "Recipe deletion not yet implemented",
		"message": "Recipes are currently immutable once created",
	})
}

// SearchRecipes searches for recipes by keyword
func (h *Handler) SearchRecipes(c *fiber.Ctx) error {
	query := c.Query("q")
	if query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Search query is required",
		})
	}

	if h.recipeRegistry == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Recipe registry not available",
		})
	}

	ctx := context.Background()

	// Get all recipes and filter by query
	recipes, err := h.recipeRegistry.ListAllRecipes(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to search recipes",
			"details": err.Error(),
		})
	}

	// Filter recipes by search query
	query = strings.ToLower(query)
	var matches []*UnifiedRecipeMetadata

	for _, recipe := range recipes {
		if strings.Contains(strings.ToLower(recipe.Metadata.ID), query) ||
			strings.Contains(strings.ToLower(recipe.Metadata.Name), query) ||
			containsInTags(recipe.Metadata.Tags, query) ||
			containsInTags(recipe.Metadata.Categories, query) {
			matches = append(matches, recipe)
		}
	}

	return c.JSON(fiber.Map{
		"query":   query,
		"count":   len(matches),
		"recipes": matches,
	})
}

func containsInTags(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}
