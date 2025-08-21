package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/ploy/ploy/controller/config"
	"github.com/ploy/ploy/controller/consul_envstore"
	"github.com/ploy/ploy/controller/dns"
	"github.com/ploy/ploy/controller/domains"
	"github.com/ploy/ploy/controller/envstore"
	"github.com/ploy/ploy/controller/health"
	"github.com/ploy/ploy/controller/routing"
	"github.com/ploy/ploy/controller/selfupdate"
	"github.com/ploy/ploy/internal/build"
	"github.com/ploy/ploy/internal/cert"
	"github.com/ploy/ploy/internal/cleanup"
	"github.com/ploy/ploy/internal/debug"
	"github.com/ploy/ploy/internal/domain"
	"github.com/ploy/ploy/internal/preview"
	"github.com/ploy/ploy/internal/storage"
	"github.com/ploy/ploy/internal/utils"
)

// ServiceDependencies holds all external service dependencies
type ServiceDependencies struct {
	EnvStore          envstore.EnvStoreInterface
	TraefikRouter     *routing.TraefikRouter
	HealthChecker     *health.HealthChecker
	CleanupHandler    *cleanup.CleanupHandler
	TTLCleanupService *cleanup.TTLCleanupService
	SelfUpdateHandler *selfupdate.Handler
	DNSHandler        *dns.Handler
	StorageConfigPath string
}

// ControllerConfig holds configuration for controller initialization
type ControllerConfig struct {
	Port                string
	ConsulAddr          string
	NomadAddr           string
	StorageConfigPath   string
	CleanupConfigPath   string
	UseConsulEnv        bool
	EnvStorePath        string
	CleanupAutoStart    bool
	ShutdownTimeout     time.Duration
}

// LoadConfigFromEnv loads controller configuration from environment variables
func LoadConfigFromEnv() *ControllerConfig {
	return &ControllerConfig{
		Port:              utils.Getenv("PORT", "8081"),
		ConsulAddr:        utils.Getenv("CONSUL_HTTP_ADDR", "127.0.0.1:8500"),
		NomadAddr:         utils.Getenv("NOMAD_ADDR", "http://127.0.0.1:4646"),
		StorageConfigPath: config.GetStorageConfigPath(),
		CleanupConfigPath: utils.Getenv("PLOY_CLEANUP_CONFIG", ""),
		UseConsulEnv:      utils.Getenv("PLOY_USE_CONSUL_ENV", "true") == "true",
		EnvStorePath:      utils.Getenv("PLOY_ENV_STORE_PATH", "/tmp/ploy-env-store"),
		CleanupAutoStart:  utils.Getenv("PLOY_CLEANUP_AUTO_START", "true") == "true",
		ShutdownTimeout:   30 * time.Second, // Graceful shutdown timeout
	}
}

// Server represents the stateless controller server
type Server struct {
	app          *fiber.App
	config       *ControllerConfig
	dependencies *ServiceDependencies
	shutdownChan chan os.Signal
}

// NewServer creates a new controller server with dependency injection
func NewServer(config *ControllerConfig) (*Server, error) {
	log.Printf("Initializing Ploy Controller with configuration-driven setup")
	
	// Initialize dependencies
	deps, err := initializeDependencies(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize dependencies: %w", err)
	}

	// Create Fiber app with middleware
	app := fiber.New(fiber.Config{
		DisableStartupMessage: false,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			log.Printf("Request error: %v", err)
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
	app.Use(preview.Router)

	server := &Server{
		app:          app,
		config:       config,
		dependencies: deps,
		shutdownChan: make(chan os.Signal, 1),
	}

	// Setup routes
	server.setupRoutes()

	return server, nil
}

// initializeDependencies initializes all external service dependencies
func initializeDependencies(cfg *ControllerConfig) (*ServiceDependencies, error) {
	log.Printf("Initializing service dependencies")

	// Validate storage configuration at startup
	if _, err := config.Load(cfg.StorageConfigPath); err != nil {
		log.Printf("Warning: Storage configuration validation failed: %v", err)
	} else {
		log.Printf("Storage configuration validated successfully from: %s", cfg.StorageConfigPath)
	}

	// Initialize environment store with fallback logic
	envStore, err := initializeEnvStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize environment store: %w", err)
	}

	// Initialize Traefik router
	traefikRouter, err := initializeTraefikRouter(cfg.ConsulAddr)
	if err != nil {
		log.Printf("Warning: Failed to initialize Traefik router: %v", err)
	}

	// Initialize health checker
	healthChecker := health.NewHealthChecker(cfg.StorageConfigPath, cfg.ConsulAddr, cfg.NomadAddr)
	log.Printf("Health checker initialized")

	// Initialize TTL cleanup service
	cleanupHandler, ttlService, err := initializeCleanupService(cfg)
	if err != nil {
		log.Printf("Warning: Failed to initialize cleanup service: %v", err)
	}

	// Initialize self-update handler
	selfUpdateHandler, err := initializeSelfUpdateHandler(cfg)
	if err != nil {
		log.Printf("Warning: Failed to initialize self-update handler: %v", err)
	}

	// Initialize DNS handler
	dnsHandler, err := initializeDNSHandler(cfg.ConsulAddr)
	if err != nil {
		log.Printf("Warning: Failed to initialize DNS handler: %v", err)
	}

	deps := &ServiceDependencies{
		EnvStore:          envStore,
		TraefikRouter:     traefikRouter,
		HealthChecker:     healthChecker,
		CleanupHandler:    cleanupHandler,
		TTLCleanupService: ttlService,
		SelfUpdateHandler: selfUpdateHandler,
		DNSHandler:        dnsHandler,
		StorageConfigPath: cfg.StorageConfigPath,
	}

	log.Printf("Service dependencies initialized successfully")
	return deps, nil
}

// initializeEnvStore initializes environment store with fallback logic
func initializeEnvStore(cfg *ControllerConfig) (envstore.EnvStoreInterface, error) {
	if cfg.UseConsulEnv {
		if consulEnvStore, err := consul_envstore.New(cfg.ConsulAddr, "ploy/apps"); err == nil {
			// Test Consul connectivity
			if err := consulEnvStore.HealthCheck(); err == nil {
				log.Printf("Using Consul KV store for environment variables at %s", cfg.ConsulAddr)
				return consulEnvStore, nil
			} else {
				log.Printf("Consul env store health check failed, falling back to file-based store: %v", err)
			}
		} else {
			log.Printf("Failed to initialize Consul env store, falling back to file-based store: %v", err)
		}
	}
	
	// Fallback to file-based store
	fileEnvStore := envstore.New(cfg.EnvStorePath)
	log.Printf("Using file-based environment store at %s", cfg.EnvStorePath)
	return fileEnvStore, nil
}

// initializeTraefikRouter initializes Traefik router if available
func initializeTraefikRouter(consulAddr string) (*routing.TraefikRouter, error) {
	traefikRouter, err := routing.NewTraefikRouter(consulAddr)
	if err != nil {
		return nil, err
	}
	log.Printf("Traefik router initialized with Consul address: %s", consulAddr)
	return traefikRouter, nil
}

// initializeCleanupService initializes TTL cleanup service
func initializeCleanupService(cfg *ControllerConfig) (*cleanup.CleanupHandler, *cleanup.TTLCleanupService, error) {
	configManager := cleanup.NewConfigManager(cfg.CleanupConfigPath)
	
	// Load or create cleanup configuration
	cleanupConfig, err := configManager.LoadConfig()
	if err != nil {
		log.Printf("Warning: Failed to load cleanup configuration, using defaults: %v", err)
		cleanupConfig = cleanup.DefaultTTLConfig()
	}
	
	// Override with environment variables if present
	envConfig := cleanup.LoadConfigFromEnv()
	if envConfig.PreviewTTL != cleanupConfig.PreviewTTL {
		cleanupConfig.PreviewTTL = envConfig.PreviewTTL
	}
	if envConfig.CleanupInterval != cleanupConfig.CleanupInterval {
		cleanupConfig.CleanupInterval = envConfig.CleanupInterval
	}
	if envConfig.MaxAge != cleanupConfig.MaxAge {
		cleanupConfig.MaxAge = envConfig.MaxAge
	}
	if envConfig.DryRun != cleanupConfig.DryRun {
		cleanupConfig.DryRun = envConfig.DryRun
	}
	if envConfig.NomadAddr != cleanupConfig.NomadAddr {
		cleanupConfig.NomadAddr = envConfig.NomadAddr
	}
	
	// Create TTL cleanup service
	ttlCleanupService := cleanup.NewTTLCleanupService(cleanupConfig)
	cleanupHandler := cleanup.NewCleanupHandler(ttlCleanupService, configManager)
	
	// Start TTL cleanup service if enabled
	if cfg.CleanupAutoStart {
		if err := ttlCleanupService.Start(); err != nil {
			log.Printf("Warning: Failed to start TTL cleanup service: %v", err)
		} else {
			log.Printf("TTL cleanup service started (interval: %v, preview TTL: %v)", 
				cleanupConfig.CleanupInterval, cleanupConfig.PreviewTTL)
		}
	}

	return cleanupHandler, ttlCleanupService, nil
}

// initializeSelfUpdateHandler initializes self-update handler
func initializeSelfUpdateHandler(cfg *ControllerConfig) (*selfupdate.Handler, error) {
	// Create storage client for self-update operations
	storageClient, err := config.CreateStorageClientFromConfig(cfg.StorageConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client for self-update: %w", err)
	}

	// Get current controller version
	currentVersion := selfupdate.GetCurrentVersion()

	// Create self-update handler
	handler, err := selfupdate.NewHandler(storageClient, cfg.ConsulAddr, currentVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to create self-update handler: %w", err)
	}

	log.Printf("Self-update handler initialized (current version: %s)", currentVersion)
	return handler, nil
}

// initializeDNSHandler initializes DNS management handler
func initializeDNSHandler(consulAddr string) (*dns.Handler, error) {
	dnsHandler, err := dns.NewHandler(consulAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create DNS handler: %w", err)
	}

	log.Printf("DNS handler initialized with Consul address: %s", consulAddr)
	return dnsHandler, nil
}

// getStorageClient creates a new storage client for each request (stateless)
func (s *Server) getStorageClient() (*storage.StorageClient, error) {
	return config.CreateStorageClientFromConfig(s.dependencies.StorageConfigPath)
}

// setupRoutes configures all API routes with dependency injection
func (s *Server) setupRoutes() {
	// Health and readiness endpoints (before API group)
	s.app.Get("/health", s.dependencies.HealthChecker.HealthHandler)
	s.app.Get("/ready", s.dependencies.HealthChecker.ReadinessHandler)
	s.app.Get("/live", s.dependencies.HealthChecker.LivenessHandler)
	s.app.Get("/health/metrics", s.dependencies.HealthChecker.MetricsHandler)
	s.app.Get("/health/deployment", s.dependencies.HealthChecker.DeploymentStatusHandler)

	api := s.app.Group("/v1")
	
	// Application build endpoints with request-scoped storage
	api.Post("/apps/:app/builds", s.handleTriggerBuild)
	api.Get("/apps", build.ListApps)
	api.Get("/status/:app", build.Status)
	
	// Domain management with dependency injection
	s.setupDomainRoutes(api)
	
	// Certificate management
	api.Post("/certs/issue", cert.IssueCertificate)
	api.Get("/certs", cert.ListCertificates)
	
	// Environment variables management with injected env store
	api.Post("/apps/:app/env", s.handleSetEnvVars)
	api.Get("/apps/:app/env", s.handleGetEnvVars)
	api.Put("/apps/:app/env/:key", s.handleSetEnvVar)
	api.Delete("/apps/:app/env/:key", s.handleDeleteEnvVar)
	
	// Debug, rollback, and destroy with dependency injection
	api.Post("/apps/:app/debug", s.handleDebugApp)
	api.Post("/apps/:app/rollback", debug.RollbackApp)
	api.Delete("/apps/:app", s.handleDestroyApp)
	
	// Storage endpoints with request-scoped clients
	api.Get("/storage/health", s.handleStorageHealth)
	api.Get("/storage/metrics", s.handleStorageMetrics)
	
	// Configuration management endpoints
	api.Get("/storage/config", s.handleGetStorageConfig)
	api.Post("/storage/config/reload", s.handleReloadStorageConfig)
	api.Post("/storage/config/validate", s.handleValidateStorageConfig)

	// TTL cleanup endpoints with dependency injection
	if s.dependencies.CleanupHandler != nil {
		cleanup.SetupRoutes(s.app, s.dependencies.CleanupHandler)
	}

	// Self-update endpoints with dependency injection
	if s.dependencies.SelfUpdateHandler != nil {
		selfupdate.SetupRoutes(s.app, s.dependencies.SelfUpdateHandler)
	}

	// DNS management endpoints with dependency injection
	if s.dependencies.DNSHandler != nil {
		dns.SetupDNSRoutes(s.app, s.dependencies.DNSHandler)
	}

	// Health endpoints in API group for versioned access
	api.Get("/health", s.dependencies.HealthChecker.HealthHandler)
	api.Get("/ready", s.dependencies.HealthChecker.ReadinessHandler)
	api.Get("/live", s.dependencies.HealthChecker.LivenessHandler)
	api.Get("/health/metrics", s.dependencies.HealthChecker.MetricsHandler)
	api.Get("/health/deployment", s.dependencies.HealthChecker.DeploymentStatusHandler)

	log.Printf("API routes configured with dependency injection")
}

// setupDomainRoutes configures domain management routes
func (s *Server) setupDomainRoutes(api fiber.Router) {
	if s.dependencies.TraefikRouter != nil {
		// Use new Traefik-based domain management
		domainHandler := domains.NewDomainHandler(s.dependencies.TraefikRouter)
		domains.SetupDomainRoutes(s.app, domainHandler)
	} else {
		// Fallback to existing domain management
		api.Post("/apps/:app/domains", domain.AddDomain)
		api.Get("/apps/:app/domains", domain.ListDomains)
		api.Delete("/apps/:app/domains/:domain", domain.RemoveDomain)
	}
}

// Start starts the server with graceful shutdown support
func (s *Server) Start() error {
	// Setup signal handling for graceful shutdown
	signal.Notify(s.shutdownChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		log.Printf("Ploy Controller listening on :%s", s.config.Port)
		if err := s.app.Listen(":" + s.config.Port); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-s.shutdownChan
	log.Printf("Received shutdown signal, initiating graceful shutdown...")

	return s.Shutdown()
}

// Shutdown performs graceful shutdown with connection draining
func (s *Server) Shutdown() error {
	log.Printf("Starting graceful shutdown procedure")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
	defer cancel()

	// Stop TTL cleanup service first
	if s.dependencies.TTLCleanupService != nil {
		log.Printf("Stopping TTL cleanup service")
		if err := s.dependencies.TTLCleanupService.Stop(); err != nil {
			log.Printf("Warning: Failed to stop TTL cleanup service: %v", err)
		} else {
			log.Printf("TTL cleanup service stopped successfully")
		}
	}

	// Shutdown HTTP server with connection draining
	log.Printf("Shutting down HTTP server (timeout: %v)", s.config.ShutdownTimeout)
	if err := s.app.ShutdownWithTimeout(s.config.ShutdownTimeout); err != nil {
		log.Printf("Error during server shutdown: %v", err)
		return err
	}

	// Close any remaining connections
	log.Printf("Draining remaining connections")

	// Wait for context timeout or completion
	select {
	case <-ctx.Done():
		log.Printf("Shutdown timeout reached")
		return ctx.Err()
	default:
		log.Printf("Graceful shutdown completed successfully")
		return nil
	}
}