package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/iw2rmb/ploy/chttp/internal/config"
	"github.com/iw2rmb/ploy/chttp/internal/executor"
	"github.com/iw2rmb/ploy/chttp/internal/handler"
	"github.com/iw2rmb/ploy/chttp/internal/health"
	"github.com/iw2rmb/ploy/chttp/internal/logging"
)

// Server represents the simplified CHTTP server
type Server struct {
	app    *fiber.App
	config *config.Config
	logger *logging.Logger
	handler *handler.Handler
}

// NewServer creates a new CHTTP server with the given configuration
func NewServer(configPath string) (*Server, error) {
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

	// Create Fiber app
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}

			logger.LogError(c.Context(), "HTTP error", err, map[string]interface{}{
				"status_code": code,
				"path":        c.Path(),
				"method":      c.Method(),
				"client_ip":   c.IP(),
			})

			return c.Status(code).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	// Add middleware
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, X-API-Key, Authorization",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))

	// Add authentication middleware
	app.Use(httpHandler.AuthMiddleware)

	// Routes
	setupRoutes(app, cfg, httpHandler)

	server := &Server{
		app:     app,
		config:  cfg,
		logger:  logger,
		handler: httpHandler,
	}

	return server, nil
}

// setupRoutes configures the HTTP routes
func setupRoutes(app *fiber.App, cfg *config.Config, h *handler.Handler) {
	// API routes
	api := app.Group("/api/v1")
	
	// Execute CLI command
	api.Post("/execute", h.ExecuteCommand)

	// Health check endpoint
	if cfg.Health.Enabled {
		app.Get(cfg.Health.Endpoint, h.HealthCheck)
	}

	// Root endpoint
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service": "CHTTP",
			"version": "1.0.0",
			"description": "Simple CLI-to-HTTP bridge",
		})
	})
}

// Start starts the HTTP server
func (s *Server) Start() error {
	// Setup graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
		s.logger.Info("Starting CHTTP server",
			"address", addr,
			"allowed_commands", s.config.Commands.Allowed,
			"log_level", s.config.Logging.Level,
		)

		if err := s.app.Listen(addr); err != nil {
			s.logger.LogError(context.Background(), "Server failed to start", err, map[string]interface{}{
				"address": addr,
			})
		}
	}()

	// Wait for shutdown signal
	<-c
	s.logger.Info("Shutting down CHTTP server")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return s.app.ShutdownWithContext(shutdownCtx)
}