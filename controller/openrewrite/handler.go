package openrewrite

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/openrewrite"
)

// Handler provides HTTP endpoints for OpenRewrite transformations
type Handler struct {
	executor openrewrite.Executor
	config   *openrewrite.Config
}

// NewHandler creates a new OpenRewrite HTTP handler
func NewHandler(executor openrewrite.Executor) *Handler {
	return &Handler{
		executor: executor,
		config:   openrewrite.DefaultConfig(),
	}
}

// NewHandlerWithConfig creates a new handler with custom configuration
func NewHandlerWithConfig(executor openrewrite.Executor, config *openrewrite.Config) *Handler {
	return &Handler{
		executor: executor,
		config:   config,
	}
}

// RegisterRoutes registers all OpenRewrite routes with the Fiber app
func (h *Handler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/v1/openrewrite")
	
	// Health check endpoint
	api.Get("/health", h.handleHealth)
	
	// Transformation endpoint
	api.Post("/transform", h.handleTransform)
}

// handleHealth handles the health check endpoint
func (h *Handler) handleHealth(c *fiber.Ctx) error {
	health := HealthResponse{
		Status:    "healthy",
		Version:   "1.0.0",
		Timestamp: time.Now(),
	}
	
	// Check for Java version
	if out, err := exec.Command("java", "-version").CombinedOutput(); err == nil {
		// Java version is printed to stderr, parse it
		lines := strings.Split(string(out), "\n")
		if len(lines) > 0 {
			health.JavaVersion = strings.TrimSpace(lines[0])
		}
	}
	
	// Check for Maven version
	if out, err := exec.Command("mvn", "-version").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 0 {
			health.MavenVersion = strings.TrimSpace(lines[0])
		}
	}
	
	// Check for Gradle version
	if out, err := exec.Command("gradle", "--version").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Gradle") {
				health.GradleVersion = strings.TrimSpace(line)
				break
			}
		}
	}
	
	// Check for Git version
	if out, err := exec.Command("git", "--version").Output(); err == nil {
		health.GitVersion = strings.TrimSpace(string(out))
	}
	
	return c.JSON(health)
}

// handleTransform handles the transformation endpoint
func (h *Handler) handleTransform(c *fiber.Ctx) error {
	startTime := time.Now()
	
	// Parse request
	var req TransformRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "Invalid request body",
			Code:  "INVALID_REQUEST",
			Details: map[string]interface{}{
				"parse_error": err.Error(),
			},
		})
	}
	
	// Validate request
	if err := h.validateRequest(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   err.Error(),
			Code:    "VALIDATION_ERROR",
		})
	}
	
	// Decode tar archive from base64
	tarData, err := base64.StdEncoding.DecodeString(req.TarArchive)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "Invalid base64 encoded tar archive",
			Code:  "INVALID_TAR_ARCHIVE",
			Details: map[string]interface{}{
				"decode_error": err.Error(),
			},
		})
	}
	
	// Parse timeout if provided
	timeout := 5 * time.Minute // Default timeout
	if req.Timeout != "" {
		parsedTimeout, err := time.ParseDuration(req.Timeout)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: "Invalid timeout format",
				Code:  "INVALID_TIMEOUT",
				Details: map[string]interface{}{
					"timeout":      req.Timeout,
					"parse_error": err.Error(),
				},
			})
		}
		timeout = parsedTimeout
	}
	
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	// Convert recipe config to internal format
	internalRecipe := openrewrite.RecipeConfig{
		Recipe:    req.RecipeConfig.Recipe,
		Artifacts: req.RecipeConfig.Artifacts,
		Options:   req.RecipeConfig.Options,
	}
	
	// Execute transformation
	result, err := h.executor.Execute(ctx, req.JobID, tarData, internalRecipe)
	
	// Prepare response
	response := TransformResponse{
		JobID:    req.JobID,
		Duration: time.Since(startTime).Seconds(),
	}
	
	if result != nil {
		response.Success = result.Success
		response.BuildSystem = result.BuildSystem
		response.JavaVersion = result.JavaVersion
		
		if result.Success {
			// Encode diff to base64
			response.Diff = base64.StdEncoding.EncodeToString(result.Diff)
			
			// Parse diff statistics
			stats := h.parseDiffStats(result.Diff)
			stats.TarSize = len(tarData)
			stats.DiffSize = len(result.Diff)
			response.Stats = stats
		} else {
			response.Error = result.Error
		}
	} else if err != nil {
		response.Success = false
		response.Error = fmt.Sprintf("Transformation failed: %v", err)
	}
	
	return c.JSON(response)
}

// validateRequest validates the transformation request
func (h *Handler) validateRequest(req TransformRequest) error {
	// Validate JobID
	if req.JobID == "" {
		return fmt.Errorf("job_id is required")
	}
	if len(req.JobID) > 100 {
		return fmt.Errorf("job_id must not exceed 100 characters")
	}
	
	// Validate TarArchive
	if req.TarArchive == "" {
		return fmt.Errorf("tar_archive is required")
	}
	
	// Validate RecipeConfig
	if req.RecipeConfig.Recipe == "" {
		return fmt.Errorf("recipe is required")
	}
	
	// Validate timeout format if provided
	if req.Timeout != "" {
		if _, err := time.ParseDuration(req.Timeout); err != nil {
			return fmt.Errorf("invalid timeout format: %v", err)
		}
	}
	
	return nil
}

// parseDiffStats parses statistics from a unified diff
func (h *Handler) parseDiffStats(diff []byte) *TransformStats {
	stats := &TransformStats{
		FilesChanged: 0,
		LinesAdded:   0,
		LinesRemoved: 0,
	}
	
	if len(diff) == 0 {
		return stats
	}
	
	lines := strings.Split(string(diff), "\n")
	currentFile := ""
	
	for _, line := range lines {
		// Count files changed
		if strings.HasPrefix(line, "--- a/") {
			currentFile = strings.TrimPrefix(line, "--- a/")
		} else if strings.HasPrefix(line, "+++ b/") && currentFile != "" {
			stats.FilesChanged++
			currentFile = ""
		}
		
		// Count lines added/removed (excluding diff headers)
		if len(line) > 0 {
			switch line[0] {
			case '+':
				if !strings.HasPrefix(line, "+++") {
					stats.LinesAdded++
				}
			case '-':
				if !strings.HasPrefix(line, "---") {
					stats.LinesRemoved++
				}
			}
		}
	}
	
	return stats
}