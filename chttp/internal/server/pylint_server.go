package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/iw2rmb/ploy/chttp/internal/analyzer"
	"github.com/iw2rmb/ploy/chttp/internal/config"
	"github.com/iw2rmb/ploy/chttp/internal/executor"
	"github.com/iw2rmb/ploy/chttp/internal/handler"
	"github.com/iw2rmb/ploy/chttp/internal/health"
	"github.com/iw2rmb/ploy/chttp/internal/logging"
)

// PylintServer extends the basic CHTTP server with Pylint analysis capabilities
type PylintServer struct {
	app      *fiber.App
	config   *config.Config
	logger   *logging.Logger
	handler  *handler.Handler
	analyzer *analyzer.PylintAnalyzer
}

// NewPylintServer creates a new Pylint CHTTP server with analysis capabilities
func NewPylintServer(configPath string) (*PylintServer, error) {
	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize logger
	logger := logging.NewLogger(cfg)

	// Initialize CLI executor
	exec := executor.NewCLIExecutor(cfg)

	// Initialize health checker
	healthChecker := health.NewHealthChecker(cfg)

	// Initialize HTTP handler
	httpHandler := handler.NewHandler(cfg, logger, exec, healthChecker)

	// Initialize Pylint analyzer
	workDir := "/tmp/pylint-analysis"
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	pylintConfig := analyzer.PylintConfig{
		Executable:  "pylint",
		Timeout:     5 * time.Minute,
		MaxMemory:   "512MB",
		SandboxUser: "pylint",
	}

	pylintAnalyzer := analyzer.NewPylintAnalyzer(workDir, pylintConfig)

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "Pylint CHTTP Service",
		ReadTimeout:  10 * time.Minute, // Long timeout for analysis
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  15 * time.Minute,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			logger.Error("Request failed", "error", err, "path", c.Path())
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Internal server error",
			})
		},
	})

	// Add middleware
	app.Use(recover.New())
	app.Use(cors.New())

	// Create server instance
	server := &PylintServer{
		app:      app,
		config:   cfg,
		logger:   logger,
		handler:  httpHandler,
		analyzer: pylintAnalyzer,
	}

	// Setup routes
	server.setupRoutes()

	return server, nil
}

// setupRoutes configures HTTP routes for the Pylint CHTTP server
func (s *PylintServer) setupRoutes() {
	// Health check endpoint
	s.app.Get("/health", s.healthHandler)

	// Basic CLI execution endpoint (from base CHTTP server)
	s.app.Post("/api/v1/execute", s.authMiddleware, s.executeHandler)

	// Specialized Pylint analysis endpoint
	s.app.Post("/analyze", s.authMiddleware, s.analyzeHandler)

	// Service information endpoint
	s.app.Get("/info", s.infoHandler)
}

// authMiddleware provides API key authentication
func (s *PylintServer) authMiddleware(c *fiber.Ctx) error {
	apiKey := c.Get("X-API-Key")
	if apiKey == "" {
		s.logger.Warn("Missing API key", "remote_addr", c.IP())
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "API key required",
		})
	}

	if apiKey != s.config.Security.APIKey {
		s.logger.Warn("Invalid API key", "remote_addr", c.IP(), "provided_key", apiKey[:8]+"...")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid API key",
		})
	}

	return c.Next()
}

// healthHandler provides health check information
func (s *PylintServer) healthHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "healthy",
		"service":   "pylint-chttp",
		"version":   "1.0.0",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"uptime":    time.Since(time.Now()).String(),
		"analyzer": fiber.Map{
			"name":    "pylint",
			"version": "3.0.0",
			"ready":   true,
		},
	})
}

// executeHandler provides basic CLI execution (delegates to base handler)
func (s *PylintServer) executeHandler(c *fiber.Ctx) error {
	return s.handler.Execute(c)
}

// analyzeHandler provides specialized Python code analysis
func (s *PylintServer) analyzeHandler(c *fiber.Ctx) error {
	startTime := time.Now()
	
	s.logger.Info("Analysis request received",
		"remote_addr", c.IP(),
		"content_type", c.Get("Content-Type"),
		"content_length", len(c.Body()),
	)

	// Validate content type
	contentType := c.Get("Content-Type")
	if contentType != "application/gzip" && contentType != "application/x-gzip" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Content-Type must be application/gzip",
		})
	}

	// Get request body (gzipped tar archive)
	body := c.Body()
	if len(body) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Request body cannot be empty",
		})
	}

	// Check archive size limit (100MB)
	maxSize := 100 * 1024 * 1024 // 100MB
	if len(body) > maxSize {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{
			"error": "Archive too large (max 100MB)",
		})
	}

	// Perform analysis
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Minute)
	defer cancel()

	result, err := s.analyzer.AnalyzeArchive(ctx, body)
	if err != nil {
		s.logger.Error("Analysis failed",
			"error", err,
			"remote_addr", c.IP(),
			"duration", time.Since(startTime),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Analysis failed",
			"details": err.Error(),
		})
	}

	s.logger.Info("Analysis completed",
		"remote_addr", c.IP(),
		"analysis_id", result.ID,
		"status", result.Status,
		"issues_found", len(result.Result.Issues),
		"duration", time.Since(startTime),
	)

	return c.JSON(result)
}

// infoHandler provides service information
func (s *PylintServer) infoHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"service": fiber.Map{
			"name":        "pylint-chttp",
			"version":     "1.0.0",
			"description": "Python static analysis via Pylint over HTTP",
		},
		"analyzer": fiber.Map{
			"name":         "pylint",
			"version":      "3.0.0",
			"language":     "python",
			"capabilities": []string{
				"syntax-analysis",
				"style-checking",
				"error-detection",
				"complexity-analysis",
				"security-basic",
			},
		},
		"api": fiber.Map{
			"endpoints": []fiber.Map{
				{
					"method":      "POST",
					"path":        "/analyze",
					"description": "Analyze Python code archive",
					"content_type": "application/gzip",
				},
				{
					"method":      "POST",
					"path":        "/api/v1/execute",
					"description": "Execute CLI commands",
					"content_type": "application/json",
				},
				{
					"method":      "GET",
					"path":        "/health",
					"description": "Health check",
				},
				{
					"method":      "GET",
					"path":        "/info",
					"description": "Service information",
				},
			},
		},
		"integration": fiber.Map{
			"arf_compatible": true,
			"ploy_managed":   true,
			"lane_support":   []string{"C", "D", "E", "F"},
		},
	})
}

// Start starts the Pylint CHTTP server
func (s *PylintServer) Start() error {
	// Setup graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		s.logger.Info("Graceful shutdown initiated")
		
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		if err := s.app.ShutdownWithContext(ctx); err != nil {
			s.logger.Error("Shutdown failed", "error", err)
		}
	}()

	// Start server
	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	s.logger.Info("Starting Pylint CHTTP service", "addr", addr)
	
	if err := s.app.Listen(addr); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}

// Stop stops the Pylint CHTTP server
func (s *PylintServer) Stop() error {
	return s.app.Shutdown()
}