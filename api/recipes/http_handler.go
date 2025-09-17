package recipes

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/recipes/models"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
)

// HTTPHandler provides HTTP endpoints for recipe catalog operations.
type HTTPHandler struct {
	storage        RecipeStorage
	index          RecipeIndexStore
	validator      RecipeValidatorInterface
	recipeRegistry *RecipeRegistry
}

// NewHTTPHandlerWithStorage creates a new recipe HTTP handler with storage backend.
func NewHTTPHandlerWithStorage(
	storage RecipeStorage,
	index RecipeIndexStore,
	validator RecipeValidatorInterface,
	provider internalStorage.StorageProvider,
	registry *RecipeRegistry,
) *HTTPHandler {
	if registry == nil && provider != nil {
		registry = NewRecipeRegistry(provider)
	}

	return &HTTPHandler{
		storage:        storage,
		index:          index,
		validator:      validator,
		recipeRegistry: registry,
	}
}

// RegisterRoutes registers recipe routes with the Fiber app.
func (h *HTTPHandler) RegisterRoutes(app *fiber.App) {
	rec := app.Group("/v1/recipes")

	rec.Get("", h.ListRecipes)
	rec.Get("/search", h.SearchRecipes)
	rec.Post("/upload", h.UploadRecipe)
	rec.Post("/validate", h.ValidateRecipe)
	rec.Post("", h.CreateRecipe)
	rec.Get("/:id", h.GetRecipe)
	rec.Put("/:id", h.UpdateRecipe)
	rec.Delete("/:id", h.DeleteRecipe)
	rec.Get("/:id/download", h.DownloadRecipe)
	rec.Get("/:id/metadata", h.GetRecipeMetadata)
	rec.Get("/:id/stats", h.GetRecipeStats)
	rec.Post("/register", h.RegisterRecipeFromRunner)
}

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
	if h.recipeRegistry == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":   "RecipeRegistry not available",
			"details": "RecipeRegistry requires SeaweedFS storage - check storage connectivity",
		})
	}

	query := c.Query("q")
	if strings.TrimSpace(query) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Query parameter 'q' is required",
		})
	}

	recipesList, err := h.recipeRegistry.SearchRecipes(c.Context(), query)
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

// Helper methods bridging storage and registry use cases.
func (h *HTTPHandler) getRecipeWithStorage(ctx context.Context, recipeID string) (*models.Recipe, error) {
	if h.storage != nil {
		if recipe, err := h.storage.GetRecipe(ctx, recipeID); err == nil {
			return recipe, nil
		}
	}

	if h.recipeRegistry != nil {
		return h.recipeRegistry.GetRecipeAsModelsRecipe(ctx, recipeID)
	}

	return nil, fmt.Errorf("no storage backend available")
}

func (h *HTTPHandler) createRecipeWithStorage(ctx context.Context, recipe *models.Recipe) error {
	if h.validator != nil {
		if err := h.validator.ValidateRecipe(ctx, recipe); err != nil {
			return fmt.Errorf("recipe validation failed: %w", err)
		}
	}

	if h.storage != nil {
		return h.storage.CreateRecipe(ctx, recipe)
	}

	if h.recipeRegistry != nil {
		return h.recipeRegistry.StoreRecipe(ctx, recipe)
	}

	return fmt.Errorf("no storage backend available")
}

func (h *HTTPHandler) updateRecipeWithStorage(ctx context.Context, recipeID string, recipe *models.Recipe) error {
	if h.validator != nil {
		if err := h.validator.ValidateRecipe(ctx, recipe); err != nil {
			return fmt.Errorf("recipe validation failed: %w", err)
		}
	}

	if h.storage != nil {
		return h.storage.UpdateRecipe(ctx, recipeID, recipe)
	}

	if h.recipeRegistry != nil {
		return h.recipeRegistry.UpdateRecipe(ctx, recipe)
	}

	return fmt.Errorf("no storage backend available")
}

func (h *HTTPHandler) deleteRecipeWithStorage(ctx context.Context, recipeID string) error {
	if h.storage != nil {
		return h.storage.DeleteRecipe(ctx, recipeID)
	}

	if h.recipeRegistry != nil {
		return h.recipeRegistry.DeleteRecipe(ctx, recipeID)
	}

	return fmt.Errorf("no storage backend available")
}

func (h *HTTPHandler) searchRecipesWithStorage(ctx context.Context, query string) ([]*models.Recipe, error) {
	if h.storage != nil {
		results, err := h.storage.SearchRecipes(ctx, query)
		if err != nil {
			return nil, err
		}
		recipes := make([]*models.Recipe, len(results))
		for i, result := range results {
			recipes[i] = result.Recipe
		}
		return recipes, nil
	}

	if h.recipeRegistry != nil {
		return h.recipeRegistry.SearchRecipes(ctx, query)
	}

	return nil, fmt.Errorf("no storage backend available")
}

func (h *HTTPHandler) getRecipeStatsWithStorage(ctx context.Context, recipeID string) (interface{}, error) {
	if h.recipeRegistry != nil {
		return h.recipeRegistry.GetRecipeStats(ctx, recipeID)
	}

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
