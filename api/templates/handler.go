package templates

import (
	"fmt"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
	platformnomad "github.com/iw2rmb/ploy/platform/nomad"
)

// Handler manages template operations
type Handler struct {
	consulClient *orchestration.ConsulTemplateClient
}

// NewHandler creates a new template handler
func NewHandler() (*Handler, error) {
	consulClient, err := orchestration.NewConsulTemplateClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create consul template client: %w", err)
	}

	return &Handler{
		consulClient: consulClient,
	}, nil
}

// TemplateStatus represents the status of a template sync operation
type TemplateStatus struct {
	Name      string `json:"name"`
	Status    string `json:"status"` // "synced", "skipped", "error"
	Message   string `json:"message,omitempty"`
	SizeBytes int    `json:"size_bytes,omitempty"`
}

// GetTemplateStatus returns the status of templates in both platform files and Consul KV
func (h *Handler) GetTemplateStatus(c *fiber.Ctx) error {
	// Use embedded templates only and compare with Consul KV
	var statuses []TemplateStatus
	for _, fullPath := range platformnomad.ListEmbeddedTemplatePaths() {
		templateName := filepath.Base(fullPath)
		status := TemplateStatus{Name: templateName}

		// Embedded content
		platformContent := platformnomad.GetEmbeddedTemplate(fullPath)
		status.SizeBytes = len(platformContent)

		// Check Consul KV
		consulContent, err := h.consulClient.GetTemplate(templateName)
		if err == nil && len(consulContent) > 0 {
			if len(platformContent) == len(consulContent) {
				status.Status = "synced"
				status.Message = "Available in embedded set and Consul KV"
			} else {
				status.Status = "different"
				status.Message = fmt.Sprintf("Size mismatch - Embedded: %d bytes, Consul: %d bytes", len(platformContent), len(consulContent))
			}
		} else {
			status.Status = "embedded_only"
			status.Message = "Available in embedded set only"
		}

		statuses = append(statuses, status)
	}

	return c.JSON(fiber.Map{"templates": statuses})
}

// SetupRoutes registers template management routes
func SetupRoutes(app *fiber.App, handler *Handler) {
	api := app.Group("/v1")

	// Template management endpoints
	api.Get("/templates/status", handler.GetTemplateStatus)
}
