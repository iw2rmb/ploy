package llms

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	models "github.com/iw2rmb/ploy/internal/llms/models"
	"github.com/iw2rmb/ploy/internal/storage"
)

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
