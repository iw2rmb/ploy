package llms

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/validation"
)

// Handler handles LLM model registry operations
type Handler struct {
	storage   storage.Storage
	validator *validation.LLMModelValidator
}

// NewHandler creates a new LLM handler
func NewHandler(storage storage.Storage) *Handler {
	return &Handler{
		storage:   storage,
		validator: validation.NewLLMModelValidator(),
	}
}

// RegisterRoutes registers all LLM model routes
func (h *Handler) RegisterRoutes(app *fiber.App) {
	// Create v1 API group
	v1 := app.Group("/v1")
	llms := v1.Group("/llms")
	models := llms.Group("/models")

	// Model management routes
	models.Get("/", h.ListModels)             // GET /v1/llms/models
	models.Get("/:id", h.GetModel)            // GET /v1/llms/models/{id}
	models.Post("/", h.CreateModel)           // POST /v1/llms/models
	models.Put("/:id", h.UpdateModel)         // PUT /v1/llms/models/{id}
	models.Delete("/:id", h.DeleteModel)      // DELETE /v1/llms/models/{id}
	models.Get("/:id/stats", h.GetModelStats) // GET /v1/llms/models/{id}/stats
	// Handle missing ID edge-case (double slash collapsing): /v1/llms/models//stats -> /v1/llms/models/stats
	models.Get("/stats", func(c *fiber.Ctx) error { return c.Status(400).JSON(fiber.Map{"error": "model ID is required"}) })
	// Default model operations
	models.Get("/default", h.GetDefaultModel) // GET /v1/llms/models/default
	models.Put("/default", h.SetDefaultModel) // PUT /v1/llms/models/default { id }

	// Fallback route to handle edge-case of double-slash: /v1/llms/models//stats
	models.Get("/*", func(c *fiber.Ctx) error {
		p := c.Path()
		if strings.HasSuffix(p, "//stats") {
			return c.Status(400).JSON(fiber.Map{"error": "model ID is required"})
		}
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	})
}

// Note: Handler method implementations are in split files:
// - list.go
// - model_crud.go
// - default.go
// - stats.go
