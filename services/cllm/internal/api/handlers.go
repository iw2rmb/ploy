package api

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/google/uuid"
	
	"github.com/iw2rmb/ploy/services/cllm/internal/arf"
	"github.com/iw2rmb/ploy/services/cllm/internal/diff"
	"github.com/iw2rmb/ploy/services/cllm/internal/providers"
	"github.com/iw2rmb/ploy/services/cllm/internal/sandbox"
)

// Handlers contains all HTTP request handlers
type Handlers struct {
	providerManager *providers.ProviderManager
	sandboxManager  *sandbox.Manager
	diffGenerator   *diff.Generator
	diffParser      *diff.Parser
	diffApplier     *diff.Applier
	diffFormatter   *diff.Formatter
	arfHandler      *arf.Handler
	logger          *slog.Logger
}

// NewHandlers creates a new handlers instance
func NewHandlers(providerManager *providers.ProviderManager, sandboxManager *sandbox.Manager, logger *slog.Logger) *Handlers {
	// Get the default LLM provider for ARF handler
	defaultProvider, err := providerManager.GetDefaultProvider()
	if err != nil {
		logger.Warn("Failed to get default LLM provider for ARF handler", "error", err)
		defaultProvider = nil
	}

	return &Handlers{
		providerManager: providerManager,
		sandboxManager:  sandboxManager,
		diffGenerator:   diff.NewGenerator(),
		diffParser:      diff.NewParser(),
		diffApplier:     diff.NewApplier(),
		diffFormatter:   diff.NewFormatter(),
		arfHandler:      arf.NewHandler(defaultProvider, sandboxManager, logger),
		logger:          logger,
	}
}

// Health returns the health status of the service
func (h *Handlers) Health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"service":   "cllm",
	})
}

// Ready returns the readiness status of the service
func (h *Handlers) Ready(c *fiber.Ctx) error {
	// For now, we're ready if we're running
	// Later, we'll check dependencies like LLM providers
	return c.JSON(fiber.Map{
		"status":    "ready",
		"timestamp": time.Now().UTC(),
		"service":   "cllm",
	})
}

// Version returns the version information of the service
func (h *Handlers) Version(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"service": "cllm",
		"version": "0.1.0-dev",
		"build":   "dev",
		"commit":  "dev",
	})
}

// AnalyzeRequest represents a code analysis request
type AnalyzeRequest struct {
	Code     string            `json:"code"`
	Language string            `json:"language"`
	Context  map[string]string `json:"context,omitempty"`
}

// AnalyzeResponse represents a code analysis response
type AnalyzeResponse struct {
	Status      string                 `json:"status"`
	Analysis    string                 `json:"analysis"`
	Suggestions []string               `json:"suggestions,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Analyze handles code analysis requests
func (h *Handlers) Analyze(c *fiber.Ctx) error {
	// Parse request body
	var req AnalyzeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("invalid request body: %v", err),
		})
	}
	
	// Validate request
	if req.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "code field is required",
		})
	}
	
	// Get available provider
	ctx := c.Context()
	provider, err := h.providerManager.GetAvailableProvider(ctx)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "no available LLM providers",
		})
	}
	
	// Prepare analysis prompt
	systemPrompt := "You are a code analysis expert. Analyze the provided code and identify potential issues, improvements, and best practices."
	userPrompt := fmt.Sprintf("Analyze this %s code:\n\n```%s\n%s\n```", req.Language, req.Language, req.Code)
	
	// Create completion request
	completionReq := providers.CompletionRequest{
		SystemPrompt: systemPrompt,
		Messages: []providers.Message{
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
		MaxTokens:   1000,
		Timeout:     30 * time.Second,
	}
	
	// Get completion from provider
	response, err := provider.Complete(ctx, completionReq)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("analysis failed: %v", err),
		})
	}
	
	return c.JSON(AnalyzeResponse{
		Status:   "analyzed",
		Analysis: response.Content,
		Metadata: map[string]interface{}{
			"provider": provider.Name(),
			"model":    response.Model,
			"tokens":   response.Usage.TotalTokens,
		},
	})
}

// TransformRequest represents a code transformation request
type TransformRequest struct {
	Code               string            `json:"code"`
	Language           string            `json:"language"`
	TransformationType string            `json:"transformation_type"`
	ErrorContext       string            `json:"error_context,omitempty"`
	Instructions       string            `json:"instructions,omitempty"`
	Context            map[string]string `json:"context,omitempty"`
}

// TransformResponse represents a code transformation response
type TransformResponse struct {
	Status          string                 `json:"status"`
	TransformedCode string                 `json:"transformed_code"`
	Diff            string                 `json:"diff,omitempty"`
	Explanation     string                 `json:"explanation,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// Transform handles code transformation requests
func (h *Handlers) Transform(c *fiber.Ctx) error {
	// Parse request body
	var req TransformRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("invalid request body: %v", err),
		})
	}
	
	// Validate request
	if req.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "code field is required",
		})
	}
	if req.TransformationType == "" && req.Instructions == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "either transformation_type or instructions must be provided",
		})
	}
	
	// Get available provider
	ctx := c.Context()
	provider, err := h.providerManager.GetAvailableProvider(ctx)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "no available LLM providers",
		})
	}
	
	// Prepare transformation prompt
	systemPrompt := h.buildTransformationSystemPrompt(req)
	userPrompt := h.buildTransformationUserPrompt(req)
	
	// Create completion request
	completionReq := providers.CompletionRequest{
		SystemPrompt: systemPrompt,
		Messages: []providers.Message{
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.2,
		MaxTokens:   2000,
		Timeout:     60 * time.Second,
	}
	
	// Get completion from provider
	response, err := provider.Complete(ctx, completionReq)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("transformation failed: %v", err),
		})
	}
	
	// Extract transformed code and explanation from response
	transformedCode, explanation := h.parseTransformationResponse(response.Content)
	
	return c.JSON(TransformResponse{
		Status:          "transformed",
		TransformedCode: transformedCode,
		Explanation:     explanation,
		Metadata: map[string]interface{}{
			"provider": provider.Name(),
			"model":    response.Model,
			"tokens":   response.Usage.TotalTokens,
		},
	})
}

// buildTransformationSystemPrompt builds the system prompt for transformations
func (h *Handlers) buildTransformationSystemPrompt(req TransformRequest) string {
	base := "You are an expert code transformation assistant. Transform code according to the specified requirements while maintaining correctness and following best practices."
	
	if req.TransformationType != "" {
		switch req.TransformationType {
		case "java11to17":
			return base + " Migrate Java code from Java 11 to Java 17, using new language features where appropriate."
		case "fix_compilation":
			return base + " Fix compilation errors in the code based on the error context provided."
		case "modernize":
			return base + " Modernize the code using current best practices and idioms."
		default:
			return base
		}
	}
	
	return base
}

// buildTransformationUserPrompt builds the user prompt for transformations
func (h *Handlers) buildTransformationUserPrompt(req TransformRequest) string {
	var prompt string
	
	if req.Instructions != "" {
		prompt = fmt.Sprintf("Instructions: %s\n\n", req.Instructions)
	}
	
	if req.ErrorContext != "" {
		prompt += fmt.Sprintf("Error context:\n%s\n\n", req.ErrorContext)
	}
	
	prompt += fmt.Sprintf("Code to transform (%s):\n\n```%s\n%s\n```\n\n", req.Language, req.Language, req.Code)
	prompt += "Provide the transformed code and a brief explanation of the changes made."
	
	return prompt
}

// parseTransformationResponse parses the LLM response to extract code and explanation
func (h *Handlers) parseTransformationResponse(response string) (code string, explanation string) {
	// Simple parsing - look for code blocks and extract them
	// In a real implementation, this would be more sophisticated
	
	// For now, return the entire response as both code and explanation
	// This will be improved in later iterations
	return response, "Transformation applied successfully"
}

// DiffRequest represents a request to generate a diff
type DiffHandlerRequest struct {
	Original     string                `json:"original"`
	Modified     string                `json:"modified"`
	OriginalPath string                `json:"original_path,omitempty"`
	ModifiedPath string                `json:"modified_path,omitempty"`
	Format       string                `json:"format,omitempty"`
	Options      diff.DiffOptions      `json:"options,omitempty"`
}

// DiffResponse represents a diff generation response
type DiffHandlerResponse struct {
	Status   string          `json:"status"`
	Diff     string          `json:"diff"`
	Format   string          `json:"format"`
	Stats    *diff.DiffStats `json:"stats,omitempty"`
	Metadata diff.DiffMetadata `json:"metadata"`
}

// Diff handles diff generation requests
func (h *Handlers) Diff(c *fiber.Ctx) error {
	// Parse request body
	var req DiffHandlerRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("invalid request body: %v", err),
		})
	}
	
	// Validate request
	if req.Original == "" && req.Modified == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "both original and modified content cannot be empty",
		})
	}
	
	// Set default format
	format := diff.DiffFormat(req.Format)
	if format == "" {
		format = diff.FormatUnified
	}
	
	// Validate format
	switch format {
	case diff.FormatUnified, diff.FormatJSON, diff.FormatSummary:
		// Valid formats
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("unsupported format: %s", req.Format),
		})
	}
	
	// Create diff request
	diffReq := diff.DiffRequest{
		Original:     req.Original,
		Modified:     req.Modified,
		OriginalPath: req.OriginalPath,
		ModifiedPath: req.ModifiedPath,
		Options:      req.Options,
	}
	
	// Ensure format is set in options
	diffReq.Options.Format = format
	diffReq.Options.IncludeStats = true // Always include stats for API responses
	
	// Generate the diff
	diffResp, err := h.diffGenerator.Generate(diffReq)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("diff generation failed: %v", err),
		})
	}
	
	// Format the diff if needed
	var formattedDiff string
	if diffResp.Content != "" {
		formattedDiff = diffResp.Content
	} else {
		formattedDiff, err = h.diffFormatter.Format(diffResp, format)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("diff formatting failed: %v", err),
			})
		}
	}
	
	return c.JSON(DiffHandlerResponse{
		Status:   "success",
		Diff:     formattedDiff,
		Format:   string(format),
		Stats:    diffResp.Stats,
		Metadata: diffResp.Metadata,
	})
}

// ParseRequest represents a request to parse a diff
type ParseHandlerRequest struct {
	Diff          string `json:"diff"`
	Format        string `json:"format,omitempty"`
	Validate      bool   `json:"validate,omitempty"`
	SecurityCheck bool   `json:"security_check,omitempty"`
}

// ParseResponse represents a diff parsing response
type ParseHandlerResponse struct {
	Status     string                  `json:"status"`
	Changes    []diff.FileChange       `json:"changes"`
	Stats      diff.DiffStats          `json:"stats"`
	Warnings   []string                `json:"warnings,omitempty"`
	Validation *diff.ValidationResult  `json:"validation,omitempty"`
}

// Parse handles diff parsing requests
func (h *Handlers) Parse(c *fiber.Ctx) error {
	// Parse request body
	var req ParseHandlerRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("invalid request body: %v", err),
		})
	}
	
	// Validate request
	if req.Diff == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "diff content is required",
		})
	}
	
	// Set default format
	format := diff.DiffFormat(req.Format)
	if format == "" {
		format = diff.FormatUnified
	}
	
	// Create parse request
	parseReq := diff.ParseRequest{
		Content:       req.Diff,
		Format:        format,
		Validate:      req.Validate,
		SecurityCheck: req.SecurityCheck,
	}
	
	// Parse the diff
	parseResp, err := h.diffParser.Parse(parseReq)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("diff parsing failed: %v", err),
		})
	}
	
	return c.JSON(ParseHandlerResponse{
		Status:     "success",
		Changes:    parseResp.Changes,
		Stats:      parseResp.Stats,
		Warnings:   parseResp.Warnings,
		Validation: parseResp.Validation,
	})
}

// ApplyRequest represents a request to apply a diff
type ApplyHandlerRequest struct {
	Diff    string             `json:"diff"`
	Target  string             `json:"target"`
	Options diff.ApplyOptions  `json:"options,omitempty"`
}

// ApplyResponse represents a diff application response
type ApplyHandlerResponse struct {
	Status       string           `json:"status"`
	Result       string           `json:"result"`
	Success      bool             `json:"success"`
	AppliedHunks int              `json:"applied_hunks"`
	FailedHunks  int              `json:"failed_hunks"`
	Conflicts    []diff.Conflict  `json:"conflicts,omitempty"`
	Report       diff.ApplyReport `json:"report"`
}

// Apply handles diff application requests
func (h *Handlers) Apply(c *fiber.Ctx) error {
	// Parse request body
	var req ApplyHandlerRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("invalid request body: %v", err),
		})
	}
	
	// Validate request
	if req.Diff == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "diff content is required",
		})
	}
	if req.Target == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "target content is required",
		})
	}
	
	// Create apply request
	applyReq := diff.ApplyRequest{
		Diff:    req.Diff,
		Target:  req.Target,
		Options: req.Options,
	}
	
	// Apply the diff
	applyResp, err := h.diffApplier.Apply(applyReq)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("diff application failed: %v", err),
		})
	}
	
	status := "success"
	if !applyResp.Success {
		status = "partial_success"
	}
	
	return c.JSON(ApplyHandlerResponse{
		Status:       status,
		Result:       applyResp.Result,
		Success:      applyResp.Success,
		AppliedHunks: applyResp.AppliedHunks,
		FailedHunks:  applyResp.FailedHunks,
		Conflicts:    applyResp.Conflicts,
		Report:       applyResp.Report,
	})
}

// SetupMiddleware configures all middleware for the Fiber app
func SetupMiddleware(app *fiber.App) {
	// Request ID middleware - adds unique ID to each request
	app.Use(requestid.New(requestid.Config{
		Header:     "X-Request-ID",
		Generator:  func() string { return uuid.New().String() },
		ContextKey: "requestid",
	}))
	
	// Logger middleware
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${latency} ${method} ${path} ${queryParams} ${body}\n",
		TimeFormat: time.RFC3339,
		TimeZone:   "UTC",
	}))
	
	// Recovery middleware - recover from panics
	app.Use(recover.New())
	
	// CORS middleware
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization,X-Requested-With,X-Request-ID",
	}))
}

// SetupRoutes configures all routes for the application
func SetupRoutes(app *fiber.App, handlers *Handlers) {
	// Health and readiness endpoints
	app.Get("/health", handlers.Health)
	app.Get("/ready", handlers.Ready)
	app.Get("/version", handlers.Version)
	
	// API v1 routes
	v1 := app.Group("/v1")
	v1.Post("/analyze", handlers.Analyze)
	v1.Post("/transform", handlers.Transform)
	
	// Diff API routes
	v1.Post("/diff", handlers.Diff)
	v1.Post("/diff/parse", handlers.Parse)
	v1.Post("/diff/apply", handlers.Apply)
	
	// ARF-specific routes
	arfGroup := v1.Group("/arf")
	arfGroup.Post("/analyze", handlers.arfHandler.AnalyzeErrors)
	arfGroup.Get("/health", handlers.arfHandler.Health)
}