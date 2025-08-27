package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"

	"github.com/iw2rmb/ploy/chttp/internal/analyzers"
	"github.com/iw2rmb/ploy/chttp/internal/auth"
	"github.com/iw2rmb/ploy/chttp/internal/config"
	"github.com/iw2rmb/ploy/chttp/internal/sandbox"
)

// Server represents the CHTTP server
type Server struct {
	app            *fiber.App
	config         *config.Config
	authManager    *auth.Manager
	sandboxManager *sandbox.Manager
	shutdownChan   chan os.Signal
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

	// Initialize authentication manager
	authManager, err := auth.NewManager(cfg.Security.PublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth manager: %w", err)
	}

	// Initialize sandbox manager
	sandboxManager := sandbox.NewManager(
		"/tmp",  // TODO: Make configurable
		cfg.Security.RunAsUser,
		cfg.Security.MaxMemory,
		cfg.Security.MaxCPU,
	)

	// Create Fiber app
	app := fiber.New(fiber.Config{
		DisableStartupMessage: false,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Internal server error",
			})
		},
	})

	// Add middleware
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} - ${latency}\n",
	}))

	// Initialize server
	server := &Server{
		app:            app,
		config:         cfg,
		authManager:    authManager,
		sandboxManager: sandboxManager,
		shutdownChan:   make(chan os.Signal, 1),
	}

	// Setup routes
	server.setupRoutes()

	return server, nil
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	// Health endpoint (no auth required)
	s.app.Get("/health", s.healthHandler)

	// Authentication middleware for all other routes
	s.app.Use(s.authManager.Middleware())

	// Analysis endpoint
	s.app.Post("/analyze", s.analyzeHandler)
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
	return c.JSON(fiber.Map{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service":   s.config.Service.Name,
	})
}

// analyzeHandler handles analysis requests
func (s *Server) analyzeHandler(c *fiber.Ctx) error {
	// Validate content type
	if c.Get("Content-Type") != "application/gzip" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Content-Type must be application/gzip",
		})
	}

	// Get request body (archive data)
	archiveData := c.Body()
	if len(archiveData) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Request body is required",
		})
	}

	// Validate archive
	maxSizeBytes := int64(100 * 1024 * 1024) // 100MB default
	if err := s.sandboxManager.ValidateArchive(archiveData, s.config.Input.AllowedExtensions, maxSizeBytes); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Invalid archive: %v", err),
		})
	}

	// Generate analysis ID
	analysisID := uuid.New().String()

	// Extract archive
	ctx, cancel := context.WithTimeout(c.Context(), s.config.GetTimeoutDuration())
	defer cancel()

	extractPath, cleanup, err := s.sandboxManager.ExtractArchive(ctx, archiveData)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to extract archive: %v", err),
		})
	}
	defer cleanup()

	// Execute analysis
	result, err := s.executeAnalysis(ctx, extractPath)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Analysis failed: %v", err),
		})
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
	var issues []Issue

	switch s.config.Output.Parser {
	case "pylint_json":
		return s.parsePylintJSON(stdout, stderr, exitCode)
	case "test", "test_parser":
		// Test parser for unit tests - creates mock issues
		if stdout != "" {
			issues = append(issues, Issue{
				File:     "test.py",
				Line:     1,
				Column:   1,
				Severity: "info",
				Rule:     "test-rule",
				Message:  "Test analysis output: " + stdout,
			})
		}
		return issues, nil
	default:
		// Generic parser - treat non-zero exit code as an issue
		if exitCode != 0 && stderr != "" {
			issues = append(issues, Issue{
				File:     "unknown",
				Line:     0,
				Severity: "error",
				Rule:     "execution-error",
				Message:  stderr,
			})
		}
	}

	return issues, nil
}

// parsePylintJSON parses Pylint JSON output using the dedicated Pylint parser
func (s *Server) parsePylintJSON(stdout, stderr string, exitCode int) ([]Issue, error) {
	parser := analyzers.NewPylintParser()
	issues, err := parser.ParseOutput(stdout, stderr, exitCode)
	if err != nil {
		return nil, err
	}
	
	// Convert analyzer issues to server issues
	var serverIssues []Issue
	for _, issue := range issues {
		serverIssues = append(serverIssues, Issue{
			File:     issue.File,
			Line:     issue.Line,
			Column:   issue.Column,
			Severity: issue.Severity,
			Rule:     issue.Rule,
			Message:  issue.Message,
		})
	}
	
	return serverIssues, nil
}

// Helper function for tests and main to create auth manager
func NewAuthManager(publicKeyPath string) (*auth.Manager, error) {
	return auth.NewManager(publicKeyPath)
}