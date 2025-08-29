package handlers

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/arf"
	"github.com/iw2rmb/ploy/internal/storage"
)

// ARFOpenRewriteHandler handles OpenRewrite-related ARF endpoints
type ARFOpenRewriteHandler struct {
	dispatcher   *arf.OpenRewriteDispatcher
}

// NewARFOpenRewriteHandler creates a new OpenRewrite handler
func NewARFOpenRewriteHandler(storageClient *storage.StorageClient) (*ARFOpenRewriteHandler, error) {
	// Get Nomad and Consul configuration from environment
	nomadAddr := getEnvOrDefault("NOMAD_ADDR", "http://localhost:4646")
	consulAddr := getEnvOrDefault("CONSUL_HTTP_ADDR", "http://localhost:8500")
	
	// Create dispatcher
	dispatcher, err := arf.NewOpenRewriteDispatcher(nomadAddr, consulAddr, storageClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create dispatcher: %w", err)
	}
	
	return &ARFOpenRewriteHandler{
		dispatcher:   dispatcher,
	}, nil
}



// TransformRequest represents a transformation request
type TransformRequest struct {
	ProjectURL     string   `json:"project_url" validate:"required"`
	Recipes        []string `json:"recipes" validate:"required,min=1"`
	PackageManager string   `json:"package_manager"`
	BaseJDK        string   `json:"base_jdk"`
	Branch         string   `json:"branch"`
}

// Transform handles POST /v1/arf/openrewrite/transform
func (h *ARFOpenRewriteHandler) Transform(c *fiber.Ctx) error {
	var req TransformRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
			"details": err.Error(),
		})
	}
	
	// Submit transformation job directly without image building
	jobID := fmt.Sprintf("openrewrite-%d", time.Now().Unix())
	
	// Create transformation job parameters
	jobParams := map[string]string{
		"job_id":     jobID,
		"recipes":    strings.Join(req.Recipes, ","),
		"project_url": req.ProjectURL,
		"branch":     req.Branch,
	}
	
	// Submit to dispatcher
	if err := h.dispatcher.SubmitTransformation(jobID, jobParams); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to submit transformation job",
			"details": err.Error(),
		})
	}
	
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"job_id":     jobID,
		"recipes":    req.Recipes,
		"status":     "submitted",
		"message":    "Transformation job submitted successfully",
	})
}

// JobStatus handles GET /v1/arf/openrewrite/status/:jobId
func (h *ARFOpenRewriteHandler) JobStatus(c *fiber.Ctx) error {
	jobID := c.Params("jobId")
	if jobID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Job ID is required",
		})
	}
	
	// Get job status from dispatcher
	status, err := h.dispatcher.GetJobStatus(jobID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Job not found",
			"job_id": jobID,
		})
	}
	
	return c.JSON(status)
}

// RegisterRoutes registers all OpenRewrite ARF routes
func (h *ARFOpenRewriteHandler) RegisterRoutes(app *fiber.App) {
	arf := app.Group("/v1/arf/openrewrite")
	
	// Transformation
	arf.Post("/transform", h.Transform)
	arf.Get("/status/:jobId", h.JobStatus)
}

// getEnvOrDefault gets environment variable or returns default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}