package recipes

import (
	"github.com/gofiber/fiber/v2"
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
