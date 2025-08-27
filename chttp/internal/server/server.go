package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/google/uuid"

	"github.com/iw2rmb/ploy/chttp/internal/auth"
	"github.com/iw2rmb/ploy/chttp/internal/config"
	"github.com/iw2rmb/ploy/chttp/internal/errors"
	"github.com/iw2rmb/ploy/chttp/internal/parsers"
	"github.com/iw2rmb/ploy/chttp/internal/sandbox"
	"github.com/iw2rmb/ploy/chttp/internal/security"
)

// Server represents the CHTTP server
type Server struct {
	app            *fiber.App
	config         *config.Config
	authManager    *auth.Manager
	sandboxManager *sandbox.Manager
	shutdownChan   chan os.Signal
	bufferPool     *sync.Pool
	streamSemaphore chan struct{} // For limiting concurrent streams
	rateLimiter    *security.RateLimiter
	pathSanitizer  *security.PathSanitizer
	resourceLimiter *security.ResourceLimiter
	errorMetrics   *errors.ErrorMetrics
	healthChecker  *errors.HealthChecker
}

// AnalysisRequest represents the structure expected in analysis requests
type AnalysisRequest struct {
	Archive []byte `json:"archive"`
}

// AnalysisResponse represents the response from analysis
type AnalysisResponse struct {
	ID        string                 `json:"id"`
	Status    string                 `json:"status"`
	Timestamp string                 `json:"timestamp"`
	Result    AnalysisResult         `json:"result,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// AnalysisResult contains the analysis findings
type AnalysisResult struct {
	Issues []Issue `json:"issues"`
}

// Issue represents a single analysis issue
type Issue struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column,omitempty"`
	Severity string `json:"severity"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
}

// NewServer creates a new CHTTP server with the given configuration
func NewServer(configPath string) (*Server, error) {
	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize authentication manager if auth is enabled
	var authManager *auth.Manager
	if cfg.Security.AuthMethod == "public_key" {
		authManager, err = auth.NewManager(cfg.Security.PublicKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize auth manager: %w", err)
		}
	}

	// Initialize sandbox manager
	sandboxManager := sandbox.NewManager(
		"/tmp",  // TODO: Make configurable
		cfg.Security.RunAsUser,
		cfg.Security.MaxMemory,
		cfg.Security.MaxCPU,
	)

	// Initialize error handling components
	errorMetrics := errors.NewErrorMetrics()
	healthChecker := errors.NewHealthChecker()
	
	// Create Fiber app with streaming support (no custom error handler - will use middleware)
	fiberConfig := fiber.Config{
		DisableStartupMessage: false,
	}
	
	// Enable streaming if configured
	if cfg.Input.StreamingEnabled {
		fiberConfig.StreamRequestBody = true
		fiberConfig.BodyLimit = -1 // No limit for streaming
	}
	
	app := fiber.New(fiberConfig)

	// Add comprehensive error handling middleware (replaces recover middleware)
	errorMiddleware := errors.NewErrorMiddleware(errors.ErrorMiddlewareConfig{
		EnableStackTrace: true,
		EnableLogging:    true,
		LogLevel:        "warning",
		IncludeDetails:   false, // Don't expose internal details in production
	})
	app.Use(errorMiddleware)
	
	// Add logger middleware
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} - ${latency}\n",
	}))

	// Initialize security components
	var rateLimiter *security.RateLimiter
	if cfg.Security.RateLimitPerSecond > 0 {
		rateLimiter = security.NewRateLimiter(security.RateLimitConfig{
			RequestsPerSecond: float64(cfg.Security.RateLimitPerSecond),
			BurstSize:         cfg.Security.RateLimitBurst,
			PerClient:         true,
		})
	}

	pathSanitizer := security.NewPathSanitizer(cfg.Security.TempDir)
	
	// Parse CPU limit
	maxCPU := 1.0
	if cfg.Security.MaxCPU != "" {
		if cpu, err := strconv.ParseFloat(cfg.Security.MaxCPU, 64); err == nil {
			maxCPU = cpu
		}
	}
	
	resourceLimiter := security.NewResourceLimiter(security.ResourceLimits{
		MaxCPU:      maxCPU,
		MaxMemory:   parseMemoryLimit(cfg.Security.MaxMemory),
		MaxFiles:    cfg.Security.MaxOpenFiles,
		MaxDuration: cfg.Executable.Timeout,
	})

	// Add health checks
	healthChecker.AddComponent("config", func() error {
		if cfg == nil {
			return errors.NewError(errors.ErrorTypeInternal, "configuration not loaded", nil)
		}
		return nil
	})
	
	if authManager != nil {
		healthChecker.AddComponent("auth", func() error {
			// Simple health check - could validate key exists
			return nil
		})
	}

	// Initialize server
	server := &Server{
		app:             app,
		config:          cfg,
		authManager:     authManager,
		sandboxManager:  sandboxManager,
		shutdownChan:    make(chan os.Signal, 1),
		rateLimiter:     rateLimiter,
		pathSanitizer:   pathSanitizer,
		resourceLimiter: resourceLimiter,
		errorMetrics:    errorMetrics,
		healthChecker:   healthChecker,
	}
	
	// Initialize streaming components if enabled
	if cfg.Input.StreamingEnabled {
		server.bufferPool = &sync.Pool{
			New: func() interface{} {
				return make([]byte, cfg.Input.BufferSize)
			},
		}
		server.streamSemaphore = make(chan struct{}, cfg.Input.MaxConcurrentStreams)
	}

	// Setup routes
	server.setupRoutes()

	return server, nil
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	// Health endpoint (no auth required)
	s.app.Get("/health", s.healthHandler)

	// Rate limiting middleware if enabled
	if s.rateLimiter != nil {
		s.app.Use(func(c *fiber.Ctx) error {
			// Use client IP as identifier
			clientID := c.IP()
			if !s.rateLimiter.Allow(clientID) {
				s.errorMetrics.RecordError(errors.ErrorTypeRateLimit, "warning")
				return errors.NewError(errors.ErrorTypeRateLimit, "Rate limit exceeded", nil).
					WithField("client_ip", clientID).
					WithRetryable(true)
			}
			return c.Next()
		})
	}

	// Authentication middleware for all other routes if enabled
	if s.authManager != nil {
		s.app.Use(s.authManager.Middleware())
	}

	// Analysis endpoint - use streaming handler if enabled
	if s.config.Input.StreamingEnabled {
		s.app.Post("/analyze", s.streamingAnalyzeHandler)
	} else {
		s.app.Post("/analyze", s.analyzeHandler)
	}
}

// Start starts the CHTTP server
func (s *Server) Start() error {
	// Setup signal handling
	signal.Notify(s.shutdownChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		addr := s.config.GetListenAddr()
		fmt.Printf("CHTTP Server %s starting on %s\n", s.config.Service.Name, addr)
		
		if err := s.app.Listen(addr); err != nil {
			fmt.Printf("Server error: %v\n", err)
		}
	}()

	// Wait for shutdown signal
	<-s.shutdownChan
	fmt.Println("Shutting down server...")

	return s.Shutdown()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() error {
	// Shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return s.app.ShutdownWithContext(ctx)
}

// healthHandler handles health check requests
func (s *Server) healthHandler(c *fiber.Ctx) error {
	health := s.healthChecker.CheckHealth()
	
	// Add service metadata
	response := fiber.Map{
		"service":   s.config.Service.Name,
		"version":   "1.0.0", // Could be from config or build info
		"status":    health.Status,
		"timestamp": health.Timestamp,
		"components": health.Components,
		"metrics": s.errorMetrics.GetStats(),
	}
	
	// Return appropriate status code
	statusCode := 200
	if health.Status == "degraded" {
		statusCode = 503
	}
	
	return c.Status(statusCode).JSON(response)
}

// analyzeHandler handles analysis requests
func (s *Server) analyzeHandler(c *fiber.Ctx) error {
	// Validate content type
	if c.Get("Content-Type") != "application/gzip" {
		s.errorMetrics.RecordError(errors.ErrorTypeValidation, "warning")
		return errors.NewError(errors.ErrorTypeValidation, "Invalid content type", nil).
			WithField("expected", "application/gzip").
			WithField("received", c.Get("Content-Type"))
	}

	// Get request body (archive data)
	archiveData := c.Body()
	if len(archiveData) == 0 {
		s.errorMetrics.RecordError(errors.ErrorTypeValidation, "warning")
		return errors.NewError(errors.ErrorTypeValidation, "Request body is required", nil).
			WithContext("operation", "read_body")
	}

	// Validate archive
	maxSizeBytes := int64(100 * 1024 * 1024) // 100MB default
	if err := s.sandboxManager.ValidateArchive(archiveData, s.config.Input.AllowedExtensions, maxSizeBytes); err != nil {
		s.errorMetrics.RecordError(errors.ErrorTypeValidation, "warning")
		return errors.NewError(errors.ErrorTypeValidation, "Archive validation failed", err).
			WithField("archive_size", len(archiveData)).
			WithField("max_size", maxSizeBytes).
			WithContext("operation", "validate_archive")
	}

	// Generate analysis ID
	analysisID := uuid.New().String()

	// Extract archive
	ctx, cancel := context.WithTimeout(c.Context(), s.config.GetTimeoutDuration())
	defer cancel()

	extractPath, cleanup, err := s.sandboxManager.ExtractArchive(ctx, archiveData)
	if err != nil {
		s.errorMetrics.RecordError(errors.ErrorTypeExecution, "error")
		return errors.NewError(errors.ErrorTypeExecution, "Failed to extract archive", err).
			WithField("analysis_id", analysisID).
			WithContext("operation", "extract_archive")
	}
	defer cleanup()

	// Execute analysis
	result, err := s.executeAnalysis(ctx, extractPath)
	if err != nil {
		s.errorMetrics.RecordError(errors.ErrorTypeExecution, "error")
		return errors.NewError(errors.ErrorTypeExecution, "Analysis execution failed", err).
			WithField("analysis_id", analysisID).
			WithField("extract_path", extractPath).
			WithContext("operation", "execute_analysis")
	}

	// Build response
	response := AnalysisResponse{
		ID:        analysisID,
		Status:    "success",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Result:    *result,
	}

	return c.JSON(response)
}

// streamingAnalyzeHandler handles analysis requests with streaming support
func (s *Server) streamingAnalyzeHandler(c *fiber.Ctx) error {
	// Validate content type
	if c.Get("Content-Type") != "application/gzip" {
		s.errorMetrics.RecordError(errors.ErrorTypeValidation, "warning")
		return errors.NewError(errors.ErrorTypeValidation, "Invalid content type for streaming", nil).
			WithField("expected", "application/gzip").
			WithField("received", c.Get("Content-Type")).
			WithContext("operation", "streaming_validate_content_type")
	}
	
	// Try to acquire semaphore for concurrent stream limiting
	select {
	case s.streamSemaphore <- struct{}{}:
		defer func() { <-s.streamSemaphore }()
	default:
		s.errorMetrics.RecordError(errors.ErrorTypeResource, "warning")
		return errors.NewError(errors.ErrorTypeResource, "Too many concurrent streaming requests", nil).
			WithField("max_concurrent", cap(s.streamSemaphore)).
			WithContext("operation", "streaming_throttle").
			WithRetryable(true)
	}
	
	// Generate analysis ID
	analysisID := uuid.New().String()
	
	// Create pipes for streaming
	pr, pw := io.Pipe()
	
	// Start streaming copy in background
	go func() {
		defer pw.Close()
		
		// Get buffer from pool
		buf := s.bufferPool.Get().([]byte)
		defer s.bufferPool.Put(buf)
		
		// Copy request body stream to pipe using pooled buffer
		_, err := io.CopyBuffer(pw, c.Context().RequestBodyStream(), buf)
		if err != nil {
			pw.CloseWithError(err)
		}
	}()
	
	// Extract archive using streaming
	ctx, cancel := context.WithTimeout(c.Context(), s.config.GetTimeoutDuration())
	defer cancel()
	
	extractPath, cleanup, err := s.sandboxManager.ExtractStreamingArchive(ctx, pr)
	if err != nil {
		s.errorMetrics.RecordError(errors.ErrorTypeExecution, "error")
		return errors.NewError(errors.ErrorTypeExecution, "Failed to extract streaming archive", err).
			WithField("analysis_id", analysisID).
			WithContext("operation", "extract_streaming_archive")
	}
	defer cleanup()
	
	// Execute analysis
	result, err := s.executeAnalysis(ctx, extractPath)
	if err != nil {
		s.errorMetrics.RecordError(errors.ErrorTypeExecution, "error")
		return errors.NewError(errors.ErrorTypeExecution, "Analysis execution failed", err).
			WithField("analysis_id", analysisID).
			WithField("extract_path", extractPath).
			WithContext("operation", "execute_analysis")
	}
	
	// Build response
	response := AnalysisResponse{
		ID:        analysisID,
		Status:    "success",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Result:    *result,
	}
	
	return c.JSON(response)
}

// executeAnalysis runs the configured analysis tool on the extracted files
func (s *Server) executeAnalysis(ctx context.Context, workingDir string) (*AnalysisResult, error) {
	// Execute the configured command
	execResult, err := s.sandboxManager.ExecuteCommand(
		ctx,
		s.config.Executable.Path,
		s.config.Executable.Args,
		workingDir,
	)
	if err != nil {
		return nil, fmt.Errorf("command execution failed: %w", err)
	}

	// Parse output based on configured parser
	issues, err := s.parseAnalysisOutput(execResult.Stdout, execResult.Stderr, execResult.ExitCode)
	if err != nil {
		return nil, fmt.Errorf("failed to parse analysis output: %w", err)
	}

	return &AnalysisResult{
		Issues: issues,
	}, nil
}

// parseAnalysisOutput parses the output from the analysis tool
func (s *Server) parseAnalysisOutput(stdout, stderr string, exitCode int) ([]Issue, error) {
	var parser parsers.Parser
	var err error
	
	// Get or create appropriate parser
	switch s.config.Output.Parser {
	case "auto":
		// Auto-detect parser based on output
		parser, err = parsers.AutoDetect(stdout, stderr, exitCode)
		if err != nil {
			// Fall back to regex parser if auto-detection fails
			parser = s.getCustomParser()
		}
	case "custom":
		// Use configured custom parser
		parser = s.getCustomParser()
	case "test", "test_parser":
		// Test parser for unit tests
		return []Issue{{
			File:     "test.py",
			Line:     1,
			Column:   1,
			Severity: "info",
			Rule:     "test-rule",
			Message:  "Test analysis output: " + stdout,
		}}, nil
	default:
		// Try to get named parser from registry
		parser, err = parsers.Get(s.config.Output.Parser)
		if err != nil {
			// Fall back to custom parser if named parser not found
			parser = s.getCustomParser()
		}
	}
	
	// Parse output
	parserIssues, err := parser.ParseOutput(stdout, stderr, exitCode)
	if err != nil {
		return nil, fmt.Errorf("parsing failed: %w", err)
	}
	
	// Convert parser issues to server issues
	var issues []Issue
	for _, pi := range parserIssues {
		issues = append(issues, Issue{
			File:     pi.File,
			Line:     pi.Line,
			Column:   pi.Column,
			Severity: pi.Severity,
			Rule:     pi.Rule,
			Message:  pi.Message,
		})
	}
	
	return issues, nil
}

// getCustomParser creates a custom parser based on configuration
func (s *Server) getCustomParser() parsers.Parser {
	if s.config.Output.CustomParser == nil {
		// Return a default regex parser if no custom config
		return parsers.CreateDefaultRegexParser()
	}
	
	switch s.config.Output.CustomParser.Type {
	case "regex":
		parser := parsers.NewRegexParser("custom-regex")
		// Add configured patterns
		for _, pattern := range s.config.Output.CustomParser.Patterns {
			parser.AddPattern(pattern.Name, pattern.Pattern, pattern.Severity, pattern.Groups)
		}
		return parser
	case "json":
		parser := parsers.NewGenericJSONParser("custom-json")
		if s.config.Output.ParserOptions != nil {
			parser.Configure(s.config.Output.ParserOptions)
		}
		return parser
	default:
		return parsers.CreateDefaultRegexParser()
	}
}


// Helper function for tests and main to create auth manager
func NewAuthManager(publicKeyPath string) (*auth.Manager, error) {
	return auth.NewManager(publicKeyPath)
}

// parseMemoryLimit converts memory strings like "512MB" to bytes
func parseMemoryLimit(limit string) int64 {
	if limit == "" {
		return 0
	}
	
	limit = strings.ToUpper(limit)
	var multiplier int64 = 1
	
	if strings.HasSuffix(limit, "KB") {
		multiplier = 1024
		limit = strings.TrimSuffix(limit, "KB")
	} else if strings.HasSuffix(limit, "MB") {
		multiplier = 1024 * 1024
		limit = strings.TrimSuffix(limit, "MB")
	} else if strings.HasSuffix(limit, "GB") {
		multiplier = 1024 * 1024 * 1024
		limit = strings.TrimSuffix(limit, "GB")
	}
	
	if val, err := strconv.ParseInt(limit, 10, 64); err == nil {
		return val * multiplier
	}
	
	return 0
}