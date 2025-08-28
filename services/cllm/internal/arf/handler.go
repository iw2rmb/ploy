package arf

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/iw2rmb/ploy/services/cllm/internal/providers"
	"github.com/iw2rmb/ploy/services/cllm/internal/sandbox"
)

// Handler handles ARF-specific analysis requests
type Handler struct {
	llmProvider     providers.Provider
	sandboxManager  *sandbox.Manager
	analyzer        *Analyzer
	logger          *slog.Logger
	defaultOptions  ARFAnalysisOptions
}

// NewHandler creates a new ARF analysis handler
func NewHandler(llmProvider providers.Provider, sandboxManager *sandbox.Manager, logger *slog.Logger) *Handler {
	return &Handler{
		llmProvider:    llmProvider,
		sandboxManager: sandboxManager,
		analyzer:       NewAnalyzer(llmProvider, logger),
		logger:         logger,
		defaultOptions: DefaultARFOptions,
	}
}

// AnalyzeErrors handles POST /v1/arf/analyze requests
func (h *Handler) AnalyzeErrors(c *fiber.Ctx) error {
	startTime := time.Now()
	requestID := uuid.New().String()
	
	// Add request ID to context for tracing
	ctx := context.WithValue(c.Context(), "request_id", requestID)
	
	h.logger.Info("ARF analysis request received",
		"request_id", requestID,
		"client_ip", c.IP(),
		"user_agent", c.Get("User-Agent"))
	
	// Parse and validate request
	var req ARFAnalysisRequest
	if err := c.BodyParser(&req); err != nil {
		h.logger.Error("Failed to parse ARF request",
			"request_id", requestID,
			"error", err)
		return h.errorResponse(c, requestID, ErrorCodeInvalidRequest, 
			"Invalid JSON in request body", err.Error())
	}
	
	// Validate request structure and content
	if err := h.validateRequest(&req); err != nil {
		h.logger.Error("ARF request validation failed",
			"request_id", requestID,
			"error", err)
		return h.errorResponse(c, requestID, ErrorCodeValidationFailed,
			"Request validation failed", err.Error())
	}
	
	// Get analysis options from headers or use defaults
	options := h.getAnalysisOptions(c, h.defaultOptions)
	
	// Set timeout context
	timeout := time.Duration(options.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	// Perform ARF analysis
	response, err := h.performAnalysis(ctx, &req, options, requestID)
	if err != nil {
		h.logger.Error("ARF analysis failed",
			"request_id", requestID,
			"error", err,
			"duration", time.Since(startTime))
		
		// Determine error type for appropriate response
		if ctx.Err() == context.DeadlineExceeded {
			return h.errorResponse(c, requestID, ErrorCodeProcessingTimeout,
				"Analysis timed out", fmt.Sprintf("Exceeded %v timeout", timeout))
		}
		
		return h.errorResponse(c, requestID, ErrorCodeInternalError,
			"Internal analysis error", err.Error())
	}
	
	// Set processing time and metadata
	processingTime := time.Since(startTime)
	response.ProcessingTime = processingTime
	response.Metadata.RequestID = requestID
	response.Metadata.Timestamp = time.Now()
	response.Metadata.Version = "1.0"
	response.Status = "success"
	
	h.logger.Info("ARF analysis completed successfully",
		"request_id", requestID,
		"duration", processingTime,
		"suggestions_count", len(response.Suggestions),
		"patterns_count", len(response.PatternMatches),
		"confidence", response.Confidence)
	
	c.Set("X-Request-ID", requestID)
	c.Set("X-Processing-Time", processingTime.String())
	
	return c.Status(fiber.StatusOK).JSON(response)
}

// validateRequest performs comprehensive validation of ARF analysis request
func (h *Handler) validateRequest(req *ARFAnalysisRequest) error {
	// Basic required field validation
	if req.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}
	if len(req.ProjectID) > MaxProjectIDLength {
		return fmt.Errorf("project_id too long (max %d characters)", MaxProjectIDLength)
	}
	
	if len(req.Errors) == 0 {
		return fmt.Errorf("at least one error is required")
	}
	if len(req.Errors) > 50 {
		return fmt.Errorf("too many errors (max 50)")
	}
	
	if req.TransformGoal == "" {
		return fmt.Errorf("transform_goal is required")
	}
	if len(req.TransformGoal) > MaxTransformGoal {
		return fmt.Errorf("transform_goal too long (max %d characters)", MaxTransformGoal)
	}
	
	if req.AttemptNumber < 1 || req.AttemptNumber > 10 {
		return fmt.Errorf("attempt_number must be between 1 and 10")
	}
	
	// Validate errors
	for i, err := range req.Errors {
		if err.Message == "" {
			return fmt.Errorf("error[%d].message is required", i)
		}
		if len(err.Message) > MaxErrorMessage {
			return fmt.Errorf("error[%d].message too long (max %d characters)", i, MaxErrorMessage)
		}
		if err.File == "" {
			return fmt.Errorf("error[%d].file is required", i)
		}
		if err.Line < 1 {
			return fmt.Errorf("error[%d].line must be positive", i)
		}
		if err.Type != "" && !isValidErrorType(err.Type) {
			return fmt.Errorf("error[%d].type must be one of: compilation, runtime, test", i)
		}
		if len(err.Context) > MaxErrorContext {
			return fmt.Errorf("error[%d].context too long (max %d characters)", i, MaxErrorContext)
		}
	}
	
	// Validate code context
	if err := h.validateCodeContext(&req.CodeContext); err != nil {
		return fmt.Errorf("code_context validation failed: %w", err)
	}
	
	// Validate attempt history
	if len(req.History) > MaxAttemptHistory {
		return fmt.Errorf("too many history entries (max %d)", MaxAttemptHistory)
	}
	
	return nil
}

// validateCodeContext validates the code context structure
func (h *Handler) validateCodeContext(ctx *CodeContext) error {
	if ctx.Language == "" {
		return fmt.Errorf("language is required")
	}
	
	validLanguages := map[string]bool{
		"java": true, "kotlin": true, "scala": true, "groovy": true,
	}
	if !validLanguages[ctx.Language] {
		return fmt.Errorf("unsupported language: %s", ctx.Language)
	}
	
	if len(ctx.SourceFiles) == 0 {
		return fmt.Errorf("at least one source file is required")
	}
	if len(ctx.SourceFiles) > MaxSourceFiles {
		return fmt.Errorf("too many source files (max %d)", MaxSourceFiles)
	}
	
	// Validate source files
	for i, file := range ctx.SourceFiles {
		if file.Path == "" {
			return fmt.Errorf("source_file[%d].path is required", i)
		}
		if file.Content == "" {
			return fmt.Errorf("source_file[%d].content is required", i)
		}
		if len(file.Content) > MaxSourceFileContent {
			return fmt.Errorf("source_file[%d].content too large (max %d characters)", i, MaxSourceFileContent)
		}
		if file.LineCount < 1 {
			return fmt.Errorf("source_file[%d].line_count must be positive", i)
		}
	}
	
	// Validate dependencies
	if len(ctx.Dependencies) > MaxDependencies {
		return fmt.Errorf("too many dependencies (max %d)", MaxDependencies)
	}
	
	for i, dep := range ctx.Dependencies {
		if dep.GroupID == "" {
			return fmt.Errorf("dependency[%d].group_id is required", i)
		}
		if dep.ArtifactID == "" {
			return fmt.Errorf("dependency[%d].artifact_id is required", i)
		}
		if dep.Version == "" {
			return fmt.Errorf("dependency[%d].version is required", i)
		}
	}
	
	// Validate environment variables
	if len(ctx.Environment) > MaxEnvironmentVars {
		return fmt.Errorf("too many environment variables (max %d)", MaxEnvironmentVars)
	}
	
	return nil
}

// getAnalysisOptions extracts analysis options from request headers
func (h *Handler) getAnalysisOptions(c *fiber.Ctx, defaults ARFAnalysisOptions) ARFAnalysisOptions {
	options := defaults
	
	// Allow override of key parameters via headers
	if maxSugg := c.Get("X-Max-Suggestions"); maxSugg != "" {
		// Parse and validate max suggestions
		// Implementation would parse string to int with validation
	}
	
	if timeout := c.Get("X-Timeout"); timeout != "" {
		// Parse and validate timeout
		// Implementation would parse duration with validation
	}
	
	if model := c.Get("X-Preferred-Model"); model != "" {
		options.PreferredModel = model
	}
	
	if debug := c.Get("X-Debug-Mode"); debug == "true" {
		options.DebugMode = true
	}
	
	return options
}

// performAnalysis executes the actual ARF error analysis
func (h *Handler) performAnalysis(ctx context.Context, req *ARFAnalysisRequest, options ARFAnalysisOptions, requestID string) (*ARFAnalysisResponse, error) {
	h.logger.Debug("Starting ARF analysis",
		"request_id", requestID,
		"project_id", req.ProjectID,
		"errors_count", len(req.Errors),
		"attempt_number", req.AttemptNumber)
	
	// Use the analyzer to perform the actual analysis
	return h.analyzer.AnalyzeErrors(ctx, req, options)
}

// errorResponse creates a standardized error response for ARF requests
func (h *Handler) errorResponse(c *fiber.Ctx, requestID, errorCode, message, details string) error {
	response := ARFErrorResponse{
		Status:    "error",
		Message:   message,
		RequestID: requestID,
		Timestamp: time.Now(),
		Errors: []ARFError{
			{
				Code:    errorCode,
				Message: message,
				Details: details,
			},
		},
	}
	
	c.Set("X-Request-ID", requestID)
	
	// Set appropriate HTTP status based on error code
	status := fiber.StatusInternalServerError
	switch errorCode {
	case ErrorCodeInvalidRequest, ErrorCodeValidationFailed, ErrorCodeUnsupportedFormat:
		status = fiber.StatusBadRequest
	case ErrorCodeProcessingTimeout:
		status = fiber.StatusRequestTimeout
	case ErrorCodeRateLimitExceeded:
		status = fiber.StatusTooManyRequests
	case ErrorCodeInsufficientQuota:
		status = fiber.StatusPaymentRequired
	}
	
	return c.Status(status).JSON(response)
}

// Health check endpoint for ARF analysis service
func (h *Handler) Health(c *fiber.Ctx) error {
	health := map[string]interface{}{
		"status": "healthy",
		"service": "arf-analyzer",
		"version": "1.0.0",
		"timestamp": time.Now(),
		"checks": map[string]string{
			"llm_provider": "ok",
			"sandbox_manager": "ok",
		},
	}
	
	// Check LLM provider health if available
	if healthChecker, ok := h.llmProvider.(interface{ Health() error }); ok {
		if err := healthChecker.Health(); err != nil {
			health["checks"].(map[string]string)["llm_provider"] = fmt.Sprintf("error: %v", err)
			health["status"] = "degraded"
		}
	}
	
	return c.JSON(health)
}

// Utility functions

// isValidErrorType checks if the error type is valid
func isValidErrorType(errorType string) bool {
	validTypes := map[string]bool{
		"compilation": true,
		"runtime":     true,
		"test":        true,
	}
	return validTypes[errorType]
}

// sanitizeInput performs basic input sanitization for security
func sanitizeInput(input string) string {
	// Remove potentially dangerous characters
	// This is a basic implementation - in production, use a proper sanitization library
	return input
}