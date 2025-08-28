package handler

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/iw2rmb/ploy/chttp/internal/config"
	"github.com/iw2rmb/ploy/chttp/internal/executor"
	"github.com/iw2rmb/ploy/chttp/internal/health"
	"github.com/iw2rmb/ploy/chttp/internal/logging"
)

// Handler contains HTTP request handlers
type Handler struct {
	config   *config.Config
	logger   *logging.Logger
	executor *executor.CLIExecutor
	health   *health.HealthChecker
}

// NewHandler creates a new HTTP handler
func NewHandler(cfg *config.Config, logger *logging.Logger, exec *executor.CLIExecutor, healthChecker *health.HealthChecker) *Handler {
	return &Handler{
		config:   cfg,
		logger:   logger,
		executor: exec,
		health:   healthChecker,
	}
}

// ExecuteCommand handles CLI command execution requests
func (h *Handler) ExecuteCommand(c *fiber.Ctx) error {
	start := time.Now()
	
	// Parse request
	var req executor.ExecuteRequest
	if err := c.BodyParser(&req); err != nil {
		h.logger.LogError(c.Context(), "Failed to parse request body", err, map[string]interface{}{
			"client_ip": c.IP(),
			"path":      c.Path(),
		})
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate request
	if err := h.executor.ValidateRequest(req); err != nil {
		h.logger.LogError(c.Context(), "Request validation failed", err, map[string]interface{}{
			"client_ip": c.IP(),
			"command":   req.Command,
		})
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Execute command
	response, err := h.executor.Execute(c.Context(), req)
	if err != nil {
		h.logger.LogError(c.Context(), "Command execution failed", err, map[string]interface{}{
			"client_ip": c.IP(),
			"command":   req.Command,
			"args":      req.Args,
		})
	}

	// Log execution
	outputLength := len(response.Stdout) + len(response.Stderr)
	if duration, parseErr := time.ParseDuration(response.Duration); parseErr == nil {
		h.logger.LogCLIExecution(c.Context(), req.Command, req.Args, duration, response.Success, response.ExitCode, outputLength)
	}

	// Log HTTP request
	duration := time.Since(start)
	statusCode := 200
	if !response.Success {
		statusCode = 500
	}
	h.logger.LogHTTPRequest(c.Context(), c.Method(), c.Path(), statusCode, duration, c.IP())

	// Return response
	if response.Success {
		return c.JSON(response)
	} else {
		return c.Status(500).JSON(response)
	}
}

// HealthCheck handles health check requests
func (h *Handler) HealthCheck(c *fiber.Ctx) error {
	start := time.Now()
	
	status := h.health.CheckHealth(c.Context())
	
	// Log HTTP request
	duration := time.Since(start)
	h.logger.LogHTTPRequest(c.Context(), c.Method(), c.Path(), 200, duration, c.IP())
	
	return c.JSON(status)
}

// AuthMiddleware provides basic API key authentication
func (h *Handler) AuthMiddleware(c *fiber.Ctx) error {
	// Skip auth for health endpoint
	if h.config.Health.Enabled && c.Path() == h.config.Health.Endpoint {
		return c.Next()
	}

	// Get API key from header
	apiKey := c.Get("X-API-Key")
	if apiKey == "" {
		// Try Authorization header with "Bearer" prefix
		auth := c.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			apiKey = strings.TrimPrefix(auth, "Bearer ")
		}
	}

	// Validate API key
	if apiKey != h.config.Security.APIKey {
		h.logger.LogAuthentication(c.Context(), false, c.IP(), "invalid or missing API key")
		return c.Status(401).JSON(fiber.Map{
			"error": "Invalid or missing API key",
		})
	}

	h.logger.LogAuthentication(c.Context(), true, c.IP(), "valid API key")
	return c.Next()
}

// LoggingMiddleware logs all requests
func (h *Handler) LoggingMiddleware(c *fiber.Ctx) error {
	start := time.Now()
	
	// Process request
	err := c.Next()
	
	// Log request after processing
	duration := time.Since(start)
	h.logger.LogHTTPRequest(c.Context(), c.Method(), c.Path(), c.Response().StatusCode(), duration, c.IP())
	
	return err
}