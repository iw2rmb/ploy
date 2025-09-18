package templates

import (
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	platformnomad "github.com/iw2rmb/ploy/platform/nomad"
)

// Handler manages template operations
type Handler struct{}

// NewHandler creates a new template handler
func NewHandler() (*Handler, error) {
	return &Handler{}, nil
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

		status.Status = "embedded"
		status.Message = "Available in embedded set"
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
