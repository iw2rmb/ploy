package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/iw2rmb/ploy/controller/openrewrite"
	internal_openrewrite "github.com/iw2rmb/ploy/internal/openrewrite"
)

// ServerConfig holds configuration for the OpenRewrite server
type ServerConfig struct {
	Port            string
	WorkspaceDir    string
	ShutdownTimeout time.Duration
}

// LoadConfigFromEnv loads server configuration from environment variables
func LoadConfigFromEnv() *ServerConfig {
	config := &ServerConfig{
		Port:            getEnv("OPENREWRITE_PORT", "8090"),
		WorkspaceDir:    getEnv("OPENREWRITE_WORKSPACE", "/app/workspace"),
		ShutdownTimeout: 30 * time.Second,
	}

	// Parse shutdown timeout from environment
	if timeoutStr := os.Getenv("OPENREWRITE_SHUTDOWN_TIMEOUT"); timeoutStr != "" {
		if timeout, err := time.ParseDuration(timeoutStr); err == nil {
			config.ShutdownTimeout = timeout
		}
	}

	return config
}

// getEnv gets environment variable with fallback
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	log.Printf("Starting OpenRewrite Service v1.0.0")

	// Load configuration
	config := LoadConfigFromEnv()
	
	log.Printf("Configuration loaded:")
	log.Printf("- Port: %s", config.Port)
	log.Printf("- Workspace: %s", config.WorkspaceDir)
	log.Printf("- Shutdown timeout: %s", config.ShutdownTimeout)

	// Ensure workspace directory exists
	if err := os.MkdirAll(config.WorkspaceDir, 0755); err != nil {
		log.Fatalf("Failed to create workspace directory: %v", err)
	}

	// Create OpenRewrite executor with custom configuration
	executorConfig := &internal_openrewrite.Config{
		WorkDir:          config.WorkspaceDir,
		MavenPath:        getEnv("OPENREWRITE_MAVEN_PATH", "mvn"),
		GradlePath:       getEnv("OPENREWRITE_GRADLE_PATH", "gradle"),
		JavaHome:         getEnv("JAVA_HOME", "/opt/java/openjdk"),
		GitPath:          getEnv("OPENREWRITE_GIT_PATH", "git"),
		MaxTransformTime: 5 * time.Minute,
		PreCachedArtifacts: []string{
			"org.openrewrite.recipe:rewrite-migrate-java:2.18.1",
			"org.openrewrite.recipe:rewrite-spring:5.21.0",
		},
	}

	executor := internal_openrewrite.NewExecutor(executorConfig)

	// Create HTTP handler
	handler := openrewrite.NewHandler(executor)

	// Setup Fiber app
	app := fiber.New(fiber.Config{
		ServerHeader: "OpenRewrite Service",
		AppName:      "OpenRewrite Service v1.0.0",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			
			log.Printf("Request error: %v", err)
			
			return c.Status(code).JSON(fiber.Map{
				"error": err.Error(),
				"code":  "SERVER_ERROR",
			})
		},
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} - ${latency}\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowMethods:     "GET, POST, HEAD, PUT, DELETE, PATCH, OPTIONS",
		AllowCredentials: false,
	}))

	// Register OpenRewrite routes
	handler.RegisterRoutes(app)

	// Add root health check for convenience
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "healthy",
			"service": "openrewrite",
			"version": "1.0.0",
		})
	})

	// Add root endpoint for service identification
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service":     "OpenRewrite Service",
			"version":     "1.0.0",
			"description": "Sandboxed OpenRewrite transformation service",
			"endpoints": map[string]string{
				"health":    "/health or /v1/openrewrite/health",
				"transform": "/v1/openrewrite/transform",
			},
		})
	})

	// Setup graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Start server in background
	go func() {
		addr := fmt.Sprintf(":%s", config.Port)
		log.Printf("OpenRewrite Service listening on %s", addr)
		log.Printf("Health check available at: http://localhost:%s/health", config.Port)
		log.Printf("Transform endpoint at: http://localhost:%s/v1/openrewrite/transform", config.Port)
		
		if err := app.Listen(addr); err != nil {
			log.Printf("Server startup failed: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	<-c
	log.Printf("Shutting down OpenRewrite Service...")

	// Create context for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()

	// Shutdown the server
	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	} else {
		log.Printf("OpenRewrite Service shutdown completed")
	}
}