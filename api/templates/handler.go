package templates

import (
	"fmt"
	"net/http"
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

// SyncTemplatesRequest represents the request to sync templates
type SyncTemplatesRequest struct {
	Force bool `json:"force,omitempty"` // Force overwrite existing templates
}

// SyncTemplatesResponse represents the response from template sync
type SyncTemplatesResponse struct {
	Success      bool             `json:"success"`
	Message      string           `json:"message"`
	SyncedCount  int              `json:"synced_count"`
	SkippedCount int              `json:"skipped_count"`
	Templates    []TemplateStatus `json:"templates"`
}

// TemplateStatus represents the status of a template sync operation
type TemplateStatus struct {
	Name      string `json:"name"`
	Status    string `json:"status"` // "synced", "skipped", "error"
	Message   string `json:"message,omitempty"`
	SizeBytes int    `json:"size_bytes,omitempty"`
}

// SyncTemplates synchronizes platform templates to Consul KV
func (h *Handler) SyncTemplates(c *fiber.Ctx) error {
	var req SyncTemplatesRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Use embedded templates as the sole source of truth
	embeddedPaths := platformnomad.ListEmbeddedTemplatePaths()

	var response SyncTemplatesResponse
	response.Templates = make([]TemplateStatus, 0, len(embeddedPaths))

	for _, fullPath := range embeddedPaths {
		templateName := filepath.Base(fullPath)
		status := TemplateStatus{Name: templateName}

		// Read template content from embedded bytes
		content := platformnomad.GetEmbeddedTemplate(fullPath)
		if len(content) == 0 {
			status.Status = "error"
			status.Message = "Embedded template missing"
			response.Templates = append(response.Templates, status)
			continue
		}

		status.SizeBytes = len(content)

		// Check if template already exists in Consul (unless force is true)
		if !req.Force {
			existing, err := h.consulClient.GetTemplate(templateName)
			if err == nil && len(existing) > 0 {
				status.Status = "skipped"
				status.Message = "Template already exists in Consul KV (use force=true to overwrite)"
				response.SkippedCount++
				response.Templates = append(response.Templates, status)
				continue
			}
		}

		// Store template in Consul KV
		err := h.consulClient.PutTemplate(templateName, content)
		if err != nil {
			status.Status = "error"
			status.Message = fmt.Sprintf("Failed to store in Consul KV: %v", err)
			response.Templates = append(response.Templates, status)
			continue
		}

		status.Status = "synced"
		status.Message = "Successfully synchronized to Consul KV"
		response.SyncedCount++
		response.Templates = append(response.Templates, status)
	}

	response.Success = true
	response.Message = fmt.Sprintf("Synchronized %d templates, skipped %d", response.SyncedCount, response.SkippedCount)

	return c.JSON(response)
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
	api.Post("/templates/sync", handler.SyncTemplates)
	api.Get("/templates/status", handler.GetTemplateStatus)
}
