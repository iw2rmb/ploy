package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	fiberrecover "github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/iw2rmb/ploy/api/acme"
	"github.com/iw2rmb/ploy/api/analysis"
	"github.com/iw2rmb/ploy/api/arf"
	"github.com/iw2rmb/ploy/api/certificates"
	"github.com/iw2rmb/ploy/api/coordination"
	"github.com/iw2rmb/ploy/api/dns"
	"github.com/iw2rmb/ploy/api/health"
	"github.com/iw2rmb/ploy/api/llms"
	"github.com/iw2rmb/ploy/api/metrics"
	modsapi "github.com/iw2rmb/ploy/api/mods"
	recipes "github.com/iw2rmb/ploy/api/recipes"
	"github.com/iw2rmb/ploy/api/routing"
	"github.com/iw2rmb/ploy/api/sbom"
	"github.com/iw2rmb/ploy/api/selfupdate"
	envstore "github.com/iw2rmb/ploy/internal/envstore"

	tarfrecipes "github.com/iw2rmb/ploy/internal/arf/recipes"
	"github.com/iw2rmb/ploy/internal/bluegreen"
	"github.com/iw2rmb/ploy/internal/cleanup"
	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	apperr "github.com/iw2rmb/ploy/internal/errors"
	policy "github.com/iw2rmb/ploy/internal/policy"
	"github.com/iw2rmb/ploy/internal/preview"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
)

// ServiceDependencies holds all external service dependencies
type ServiceDependencies struct {
	EnvStore                envstore.EnvStoreInterface
	TraefikRouter           *routing.TraefikRouter
	HealthChecker           *health.HealthChecker
	CleanupHandler          *cleanup.CleanupHandler
	TTLCleanupService       *cleanup.TTLCleanupService
	SelfUpdateHandler       *selfupdate.Handler
	DNSHandler              *dns.Handler
	ACMEHandler             *acme.Handler
	CertificateManager      *certificates.CertificateManager
	PlatformWildcardManager *certificates.PlatformWildcardCertificateManager
	RemediationHandler      *arf.Handler
	RecipesHandler          *recipes.HTTPHandler
	ModsHandler             *modsapi.Handler
	AnalysisHandler         *analysis.Handler
	LLMHandler              *llms.Handler
	SBOMHandler             *sbom.Handler
	CoordinationManager     *coordination.CoordinationManager
	BlueGreenManager        *bluegreen.Manager
	Metrics                 *metrics.Metrics
	StorageConfigPath       string
	// StorageFactory deprecated: use config service
	RecipeCatalog tarfrecipes.Registry
}

// Server represents the stateless controller server
type Server struct {
	app                *fiber.App
	config             *ControllerConfig
	dependencies       *ServiceDependencies
	shutdownChan       chan os.Signal
	coordinationCtx    context.Context
	coordinationCancel context.CancelFunc
	configService      *cfgsvc.Service
	indexerStorage     internalStorage.Storage
}

func (s *Server) runRecipeIndexerIfConfigured() {
	if s.config == nil {
		return
	}
	packsSpec := strings.TrimSpace(s.config.RemediationDefaultPacks)
	if packsSpec == "" {
		return
	}

	if s.indexerStorage == nil {
		st, err := s.resolveUnifiedStorage()
		if err != nil {
			log.Printf("Warning: unable to resolve storage for recipe indexer: %v", err)
			return
		}
		s.indexerStorage = st
	}

	var fetcher tarfrecipes.Fetcher
	switch {
	case s.config.RemediationFetcher != nil:
		fetcher = s.config.RemediationFetcher
	case strings.TrimSpace(s.config.RemediationMavenGroup) != "":
		base := strings.TrimSpace(s.config.RemediationRegistryURL)
		if base == "" {
			base = "https://repo1.maven.org/maven2"
		}
		fetcher = tarfrecipes.MavenFetcher{BaseURL: base, GroupID: s.config.RemediationMavenGroup}
	case strings.TrimSpace(s.config.RemediationRegistryURL) != "":
		fetcher = tarfrecipes.HTTPFetcher{BaseURL: strings.TrimRight(s.config.RemediationRegistryURL, "/")}
	default:
		log.Printf("Skipping recipe catalog indexing: no remediation fetcher or registry configured")
		return
	}

	var packs []tarfrecipes.PackSpec
	for _, part := range strings.Split(packsSpec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segments := strings.SplitN(part, ":", 2)
		if len(segments) != 2 {
			log.Printf("Skipping invalid remediation pack spec: %s", part)
			continue
		}
		pack := strings.TrimSpace(segments[0])
		ver := strings.TrimSpace(segments[1])
		if pack == "" || ver == "" {
			log.Printf("Skipping remediation pack spec with empty fields: %s", part)
			continue
		}
		packs = append(packs, tarfrecipes.PackSpec{Name: pack, Version: ver})
	}
	if len(packs) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	indexer := tarfrecipes.NewIndexer(fetcher, s.indexerStorage)
	if _, err := indexer.Refresh(ctx, packs); err != nil {
		log.Printf("Warning: failed to index remediation packs: %v", err)
		return
	}
	log.Printf("Indexed remediation packs: %s", packsSpec)
}

// resolveUnifiedStorage prefers the config service if available, otherwise
// falls back to the existing factory helper with a file path.
func (s *Server) resolveUnifiedStorage() (internalStorage.Storage, error) {
	return resolveStorageFromConfigService(s.configService)
}

// NewServer creates a new controller server with dependency injection
func NewServer(config *ControllerConfig) (*Server, error) {
	log.Printf("Initializing Ploy Controller with configuration-driven setup")

	// Initialize centralized configuration service (prefer file + env) first
	var cfgService *cfgsvc.Service
	if config != nil && config.StorageConfigPath != "" {
		// Build options dynamically and enable Consul source if env flags present
		opts := []cfgsvc.Option{
			cfgsvc.WithFile(config.StorageConfigPath),
			cfgsvc.WithEnvironment("PLOY_"),
			cfgsvc.WithValidation(cfgsvc.NewStructValidator()),
			cfgsvc.WithCacheTTL(5 * time.Minute),
			cfgsvc.WithHotReload(500 * time.Millisecond),
		}
		if addr := os.Getenv("PLOY_CONFIG_CONSUL_ADDR"); addr != "" {
			if key := os.Getenv("PLOY_CONFIG_CONSUL_KEY"); key != "" {
				// Optional or required Consul source depending on toggle
				required := strings.EqualFold(os.Getenv("PLOY_CONFIG_CONSUL_REQUIRED"), "true")
				opts = append(opts, cfgsvc.WithConsul(addr, key, required))
			}
		}

		svc, err := cfgsvc.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize config service: %w", err)
		}
		cfgService = svc
		// Apply centralized policy configuration if present
		policy.ApplyFromConfig(cfgService)
	}

	// Initialize dependencies (prefer config service where applicable)
	deps, err := initializeDependenciesWithService(config, cfgService)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize dependencies: %w", err)
	}
	if deps.HealthChecker != nil && cfgService != nil {
		deps.HealthChecker.SetConfigService(cfgService)
	}

	// Create Fiber app with middleware
	app := fiber.New(fiber.Config{
		DisableStartupMessage: false,
		ReadTimeout:           10 * time.Minute, // 10-minute request timeout
		WriteTimeout:          10 * time.Minute, // 10-minute response timeout
		IdleTimeout:           60 * time.Second, // Connection idle timeout
		// Allow large tar uploads for build endpoints (default is 4MB)
		BodyLimit: 200 * 1024 * 1024, // 200MB
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			// Map to typed error
			te := apperr.From(err)
			// Log server-side
			log.Printf("Request error: code=%s status=%d msg=%s cause=%v", te.Code, te.HTTPStatus, te.Message, te.Unwrap())
			return c.Status(te.HTTPStatus).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    te.Code,
					"message": te.Message,
					"details": te.Details,
				},
			})
		},
	})

	// Add middleware with detailed panic recovery
	app.Use(fiberrecover.New(fiberrecover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, err interface{}) {
			log.Printf("PANIC RECOVERED: %v", err)
			log.Printf("Request: %s %s", c.Method(), c.OriginalURL())
			log.Printf("Headers: %v", c.GetReqHeaders())
			log.Printf("Body: %s", string(c.Body()))
		},
	}))
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} - ${latency}\n",
	}))

	// Add metrics middleware if metrics are available
	if deps.Metrics != nil {
		app.Use(deps.Metrics.MetricsMiddleware())
		deps.Metrics.StartUptimeUpdater()
	}

	app.Use(preview.Router)

	// Create coordination context
	coordinationCtx, coordinationCancel := context.WithCancel(context.Background())

	server := &Server{
		app:                app,
		config:             config,
		dependencies:       deps,
		shutdownChan:       make(chan os.Signal, 1),
		coordinationCtx:    coordinationCtx,
		coordinationCancel: coordinationCancel,
		configService:      cfgService,
	}

	// Setup routes
	server.setupRoutes()

	if config != nil {
		if server.indexerStorage == nil {
			if st, err := server.resolveUnifiedStorage(); err == nil {
				server.indexerStorage = st
			} else if err != nil {
				log.Printf("Warning: unable to prepare storage for recipe catalog indexer: %v", err)
			}
		}
		server.runRecipeIndexerIfConfigured()
	}

	return server, nil
}

// getStorageClient creates a new storage client for each request (stateless with caching)
// Now returns the new storage.Storage interface instead of *storage.StorageClient
func (s *Server) getStorageClient() (internalStorage.Storage, error) {
	// Centralized config service is required
	start := time.Now()
	st, err := resolveStorageFromConfigService(s.configService)
	if s.dependencies.Metrics != nil {
		s.dependencies.Metrics.RecordConfigLoadTime("storage", err == nil, time.Since(start))
	}
	return st, err
}

// Start starts the server with graceful shutdown support
func (s *Server) Start() error {
	// Setup signal handling for graceful shutdown
	signal.Notify(s.shutdownChan, os.Interrupt, syscall.SIGTERM)

	// Start coordination manager for leader election
	if s.dependencies.CoordinationManager != nil {
		go func() {
			log.Printf("🔄 Starting coordination manager for leader election")
			defer func() {
				if r := recover(); r != nil {
					log.Printf("❌ PANIC in coordination manager: %v", r)
				}
			}()
			if err := s.dependencies.CoordinationManager.Run(s.coordinationCtx); err != nil {
				log.Printf("❌ Coordination manager error: %v", err)
			} else {
				log.Printf("✓ Coordination manager completed successfully")
			}
		}()
	} else {
		log.Printf("⚠️  Coordination manager not available, running in single-instance mode")
	}

	// Ensure platform wildcard certificate is provisioned
	if err := s.ensurePlatformWildcardCertificate(); err != nil {
		log.Printf("Warning: Failed to ensure platform wildcard certificate: %v", err)
		// Don't fail server startup, certificate provisioning can be retried
	}

	// Skip self-registration when running in Nomad (let Nomad template handle Traefik registration)
	if os.Getenv("NOMAD_ALLOC_ID") != "" {
		log.Printf("Running in Nomad environment - skipping API self-registration (using Nomad service template)")
	} else {
		// Register controller with Traefik for platform domain access (only in non-Nomad environments)
		if err := s.registerControllerWithTraefik(); err != nil {
			log.Printf("Warning: Failed to register controller with Traefik: %v", err)
			// Don't fail server startup, Traefik registration can be retried
		}
	}

	// Start server in goroutine
	go func() {
		log.Printf("Ploy Controller listening on :%s", s.config.Port)
		if err := s.app.Listen(":" + s.config.Port); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-s.shutdownChan
	log.Printf("Graceful shutdown initiated")

	return s.Shutdown()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() error {
	log.Printf("Shutting down server...")

	// Cancel coordination context to stop leader election
	s.coordinationCancel()

	// Stop all TTL cleanup services if available
	if s.dependencies.TTLCleanupService != nil {
		if err := s.dependencies.TTLCleanupService.Stop(); err != nil {
			log.Printf("Warning: Failed to stop TTL cleanup service: %v", err)
		} else {
			log.Printf("TTL cleanup service stopped successfully")
		}
	}

	// Gracefully shutdown the Fiber app with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
	defer cancel()

	if err := s.app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Printf("Forced shutdown: %v", err)
		return err
	}

	log.Printf("Server shutdown completed successfully")
	return nil
}

// ensurePlatformWildcardCertificate ensures the platform wildcard certificate is provisioned
func (s *Server) ensurePlatformWildcardCertificate() error {
	if s.dependencies.PlatformWildcardManager == nil || !s.dependencies.PlatformWildcardManager.IsEnabled() {
		log.Printf("Platform wildcard certificate management disabled (PLOY_APPS_DOMAIN not set)")
		return nil
	}

	ctx := context.Background()
	_, err := s.dependencies.PlatformWildcardManager.GetPlatformWildcardCertificate(ctx)
	if err != nil {
		log.Printf("Failed to ensure platform wildcard certificate: %v", err)
		return err
	}

	log.Printf("✓ Platform wildcard certificate ensured successfully")
	return nil
}

// registerControllerWithTraefik registers the controller with Traefik for platform domain access
func (s *Server) registerControllerWithTraefik() error {
	if s.dependencies.TraefikRouter == nil {
		log.Printf("Traefik router not available - skipping controller registration")
		return nil
	}

	// Get platform domain from environment
	platformDomain := os.Getenv("PLOY_PLATFORM_DOMAIN")
	if platformDomain == "" {
		log.Printf("PLOY_PLATFORM_DOMAIN not set - skipping controller registration")
		return nil
	}

	// Note: Traefik registration would go here if RegisterService method existed
	log.Printf("✓ Controller registration with Traefik skipped - method not available")
	return nil
}

// Note: Build handlers are defined in handlers.go
