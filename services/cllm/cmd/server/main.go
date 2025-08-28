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
	"github.com/iw2rmb/ploy/services/cllm/internal/api"
	"github.com/iw2rmb/ploy/services/cllm/internal/config"
	"github.com/iw2rmb/ploy/services/cllm/internal/providers"
	"github.com/iw2rmb/ploy/services/cllm/internal/sandbox"
)

// Server represents the CLLM HTTP server
type Server struct {
	app             *fiber.App
	config          *config.Config
	providerManager *providers.ProviderManager
	sandboxManager  *sandbox.Manager
}

// NewServer creates a new server instance
func NewServer() (*Server, error) {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	
	// Create Fiber app
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{
				"error":     err.Error(),
				"timestamp": time.Now().UTC(),
			})
		},
	})
	
	// Initialize sandbox manager
	sandboxConfig := sandbox.ManagerConfig{
		WorkDir:        cfg.Sandbox.WorkDir,
		MaxMemory:      cfg.Sandbox.MaxMemory,
		MaxCPUTime:     cfg.Sandbox.MaxCPUTime,
		MaxProcesses:   cfg.Sandbox.MaxProcesses,
		CleanupTimeout: cfg.Sandbox.CleanupTimeout,
	}
	sandboxManager, err := sandbox.NewManager(sandboxConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox manager: %w", err)
	}
	
	// Initialize provider manager
	providerConfigs := []providers.ProviderConfig{}
	
	// Add Ollama provider if configured
	if cfg.Providers.Ollama.Enabled {
		providerConfigs = append(providerConfigs, providers.ProviderConfig{
			Type:    "ollama",
			BaseURL: cfg.Providers.Ollama.BaseURL,
			Model:   cfg.Providers.Ollama.Model,
			Timeout: cfg.Providers.Ollama.Timeout,
		})
	}
	
	// Add OpenAI provider if configured
	if cfg.Providers.OpenAI.Enabled {
		providerConfigs = append(providerConfigs, providers.ProviderConfig{
			Type:    "openai",
			BaseURL: cfg.Providers.OpenAI.BaseURL,
			APIKey:  cfg.Providers.OpenAI.APIKey,
			Model:   cfg.Providers.OpenAI.Model,
			Timeout: cfg.Providers.OpenAI.Timeout,
		})
	}
	
	// Add mock provider for development/testing
	if cfg.Environment == "development" {
		providerConfigs = append(providerConfigs, providers.ProviderConfig{
			Type: "mock",
		})
	}
	
	// Ensure at least one provider is configured
	if len(providerConfigs) == 0 {
		// Default to mock provider if none configured
		providerConfigs = append(providerConfigs, providers.ProviderConfig{
			Type: "mock",
		})
	}
	
	providerManager, err := providers.NewProviderManager(providerConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider manager: %w", err)
	}
	
	// Setup middleware
	api.SetupMiddleware(app)
	
	// Setup routes
	handlers := api.NewHandlers(providerManager, sandboxManager)
	api.SetupRoutes(app, handlers)
	
	return &Server{
		app:             app,
		config:          cfg,
		providerManager: providerManager,
		sandboxManager:  sandboxManager,
	}, nil
}

// Start starts the HTTP server
func (s *Server) Start(addr string) error {
	if addr == "" {
		addr = fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	}
	
	log.Printf("Starting CLLM server on %s", addr)
	return s.app.Listen(addr)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down CLLM server...")
	
	// Close provider manager
	if s.providerManager != nil {
		if err := s.providerManager.Close(); err != nil {
			log.Printf("Error closing provider manager: %v", err)
		}
	}
	
	// Shutdown sandbox manager
	if s.sandboxManager != nil {
		if err := s.sandboxManager.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down sandbox manager: %v", err)
		}
	}
	
	return s.app.ShutdownWithContext(ctx)
}

// GetHandler returns the Fiber app handler (for testing)
func (s *Server) GetHandler() *fiber.App {
	return s.app
}

func main() {
	// Create server
	server, err := NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	
	// Start server in a goroutine
	go func() {
		if err := server.Start(""); err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()
	
	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	
	log.Println("Received shutdown signal")
	
	// Create context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Attempt graceful shutdown
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	
	log.Println("Server exited")
}