package llms

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/arf/models"
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
	// Default model operations
	models.Get("/default", h.GetDefaultModel) // GET /v1/llms/models/default
	models.Put("/default", h.SetDefaultModel) // PUT /v1/llms/models/default { id }
}

// ListModels returns a list of LLM models
func (h *Handler) ListModels(c *fiber.Ctx) error {
	ctx := context.Background()

	// Parse query parameters
	provider := c.Query("provider", "")
	capability := c.Query("capability", "")
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	// Create filter options
	filterOptions := storage.ListOptions{
		Prefix:  "llms/models/",
		MaxKeys: limit,
	}

	// List objects from storage
	objects, err := h.storage.List(ctx, filterOptions)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to list models: %v", err),
		})
	}

	// Load and filter models
	modelsList := make([]*models.LLMModel, 0)
	for i, obj := range objects {
		// Apply offset
		if i < offset {
			continue
		}

		// Apply limit
		if len(modelsList) >= limit {
			break
		}

		// Get model data
		reader, err := h.storage.Get(ctx, obj.Key)
		if err != nil {
			continue // Skip invalid models
		}

		var model models.LLMModel
		decoder := json.NewDecoder(reader)
		if err := decoder.Decode(&model); err != nil {
			_ = reader.Close()
			continue // Skip invalid models
		}
		_ = reader.Close()

		// Apply filters
		if provider != "" && model.Provider != provider {
			continue
		}

		if capability != "" && !model.HasCapability(capability) {
			continue
		}

		modelsList = append(modelsList, &model)
	}

	return c.JSON(fiber.Map{
		"models": modelsList,
		"count":  len(modelsList),
		"total":  len(objects), // Total before filtering
	})
}

// GetModel returns a specific LLM model
func (h *Handler) GetModel(c *fiber.Ctx) error {
	ctx := context.Background()
	modelID := c.Params("id")

	if modelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "model ID is required",
		})
	}

	// Get model from storage
	key := fmt.Sprintf("llms/models/%s", modelID)
	reader, err := h.storage.Get(ctx, key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": fmt.Sprintf("model not found: %s", modelID),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to get model: %v", err),
		})
	}
	defer func() { _ = reader.Close() }()

	var model models.LLMModel
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&model); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to parse model data",
		})
	}

	return c.JSON(model)
}

// CreateModel creates a new LLM model
func (h *Handler) CreateModel(c *fiber.Ctx) error {
	ctx := context.Background()

	var model models.LLMModel
	if err := c.BodyParser(&model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	// Validate the model
	if err := h.validator.ValidateLLMModel(&model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("validation failed: %v", err),
		})
	}

	// Check if model already exists
	key := fmt.Sprintf("llms/models/%s", model.ID)
	exists, err := h.storage.Exists(ctx, key)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to check model existence",
		})
	}
	if exists {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": fmt.Sprintf("model already exists: %s", model.ID),
		})
	}

	// Set system fields
	model.SetSystemFields()

	// Serialize model
	modelData, err := json.Marshal(model)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to serialize model",
		})
	}

	// Store in storage
	reader := strings.NewReader(string(modelData))
	if err := h.storage.Put(ctx, key, reader, storage.WithContentType("application/json")); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to store model: %v", err),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":      model.ID,
		"message": "model created successfully",
	})
}

// UpdateModel updates an existing LLM model
func (h *Handler) UpdateModel(c *fiber.Ctx) error {
	ctx := context.Background()
	modelID := c.Params("id")

	if modelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "model ID is required",
		})
	}

	var updatedModel models.LLMModel
	if err := c.BodyParser(&updatedModel); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	// Ensure the ID in the URL matches the model ID
	if updatedModel.ID != modelID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "model ID in URL does not match model ID in body",
		})
	}

	// Get existing model
	key := fmt.Sprintf("llms/models/%s", modelID)
	reader, err := h.storage.Get(ctx, key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": fmt.Sprintf("model not found: %s", modelID),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to get existing model: %v", err),
		})
	}

	var existingModel models.LLMModel
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&existingModel); err != nil {
		_ = reader.Close()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to parse existing model data",
		})
	}
	_ = reader.Close()

	// Validate update
	if err := h.validator.ValidateModelUpdate(&existingModel, &updatedModel); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("validation failed: %v", err),
		})
	}

	// Preserve creation time
	updatedModel.Created = existingModel.Created
	updatedModel.SetSystemFields()

	// Serialize updated model
	modelData, err := json.Marshal(updatedModel)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to serialize updated model",
		})
	}

	// Update in storage
	updateReader := strings.NewReader(string(modelData))
	if err := h.storage.Put(ctx, key, updateReader, storage.WithContentType("application/json")); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to update model: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"id":      modelID,
		"message": "model updated successfully",
	})
}

// DeleteModel deletes an LLM model
func (h *Handler) DeleteModel(c *fiber.Ctx) error {
	ctx := context.Background()
	modelID := c.Params("id")

	if modelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "model ID is required",
		})
	}

	// Check if model exists
	key := fmt.Sprintf("llms/models/%s", modelID)
	exists, err := h.storage.Exists(ctx, key)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to check model existence",
		})
	}
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fmt.Sprintf("model not found: %s", modelID),
		})
	}

	// Delete from storage
	if err := h.storage.Delete(ctx, key); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to delete model: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"id":      modelID,
		"message": "model deleted successfully",
	})
}

// GetModelStats returns statistics for a specific model
func (h *Handler) GetModelStats(c *fiber.Ctx) error {
	modelID := c.Params("id")

	if modelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "model ID is required",
		})
	}

	// For now, return mock stats
	// TODO: Implement actual usage statistics collection
	stats := map[string]interface{}{
		"model_id":            modelID,
		"usage_count":         0,
		"last_used":           nil,
		"average_tokens":      0,
		"total_requests":      0,
		"successful_requests": 0,
		"failed_requests":     0,
		"success_rate":        0.0,
		"cost_metrics": map[string]interface{}{
			"total_cost":   0.0,
			"average_cost": 0.0,
			"cost_per_day": 0.0,
		},
	}

	return c.JSON(stats)
}

// GetDefaultModel returns the default LLM model if configured, otherwise a best-effort selection.
func (h *Handler) GetDefaultModel(c *fiber.Ctx) error {
	ctx := context.Background()
	// Resolve default id
	defKey := "llms/models/__default"
	var modelID string
	if r, err := h.storage.Get(ctx, defKey); err == nil {
		var obj struct {
			ID string `json:"id"`
		}
		if json.NewDecoder(r).Decode(&obj) == nil && obj.ID != "" {
			modelID = obj.ID
		}
		_ = r.Close()
	}
	if modelID != "" {
		// Attempt to fetch by id
		key := fmt.Sprintf("llms/models/%s", modelID)
		if r, err := h.storage.Get(ctx, key); err == nil {
			defer func() { _ = r.Close() }()
			var m models.LLMModel
			if json.NewDecoder(r).Decode(&m) == nil {
				return c.JSON(m)
			}
		}
	}
	// Fallback: list models and pick one with 'code' capability, else first
	objects, err := h.storage.List(ctx, storage.ListOptions{Prefix: "llms/models/"})
	if err != nil || len(objects) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "no models available"})
	}
	var first *models.LLMModel
	for _, obj := range objects {
		if obj.Key == defKey {
			continue
		}
		r, err := h.storage.Get(ctx, obj.Key)
		if err != nil {
			continue
		}
		var m models.LLMModel
		if json.NewDecoder(r).Decode(&m) == nil {
			if first == nil {
				mm := m
				first = &mm
			}
			if m.HasCapability("code") {
				_ = r.Close()
				return c.JSON(m)
			}
		}
		_ = r.Close()
	}
	if first != nil {
		return c.JSON(first)
	}
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "no models available"})
}

// SetDefaultModel sets the default LLM model by ID.
func (h *Handler) SetDefaultModel(c *fiber.Ctx) error {
	ctx := context.Background()
	var req struct {
		ID string `json:"id"`
	}
	if err := c.BodyParser(&req); err != nil || req.ID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body: expected {id}"})
	}
	// Validate it exists
	key := fmt.Sprintf("llms/models/%s", req.ID)
	if exists, err := h.storage.Exists(ctx, key); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "existence check failed"})
	} else if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": fmt.Sprintf("model not found: %s", req.ID)})
	}
	// Write default pointer
	body, _ := json.Marshal(req)
	if err := h.storage.Put(ctx, "llms/models/__default", strings.NewReader(string(body)), storage.WithContentType("application/json")); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to set default"})
	}
	return c.JSON(fiber.Map{"message": "default set", "id": req.ID})
}
