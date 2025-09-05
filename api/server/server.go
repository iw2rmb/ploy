package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	fiberrecover "github.com/gofiber/fiber/v2/middleware/recover"
	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/sirupsen/logrus"

	"github.com/iw2rmb/ploy/api/acme"
	"github.com/iw2rmb/ploy/api/analysis"
	javaanalyzer "github.com/iw2rmb/ploy/api/analysis/analyzers/java"
	pythonanalyzer "github.com/iw2rmb/ploy/api/analysis/analyzers/python"
	"github.com/iw2rmb/ploy/api/arf"
	"github.com/iw2rmb/ploy/api/certificates"
	"github.com/iw2rmb/ploy/api/consul_envstore"
	"github.com/iw2rmb/ploy/api/coordination"
	"github.com/iw2rmb/ploy/api/dns"
	"github.com/iw2rmb/ploy/api/domains"
	"github.com/iw2rmb/ploy/api/health"
	"github.com/iw2rmb/ploy/api/metrics"
	"github.com/iw2rmb/ploy/api/routing"
	"github.com/iw2rmb/ploy/api/selfupdate"
	"github.com/iw2rmb/ploy/api/templates"
	envstore "github.com/iw2rmb/ploy/internal/envstore"

	// "github.com/iw2rmb/ploy/api/openrewrite"
	"github.com/iw2rmb/ploy/api/version"
	"github.com/iw2rmb/ploy/internal/bluegreen"
	"github.com/iw2rmb/ploy/internal/build"
	"github.com/iw2rmb/ploy/internal/cleanup"
	"github.com/iw2rmb/ploy/internal/debug"
	"github.com/iw2rmb/ploy/internal/domain"

	// internal_openrewrite "github.com/iw2rmb/ploy/internal/openrewrite"
	arfcore "github.com/iw2rmb/ploy/internal/arf/core"
	tarfrecipes "github.com/iw2rmb/ploy/internal/arf/recipes"
	trecipes "github.com/iw2rmb/ploy/internal/arf/recipes"
	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	apperr "github.com/iw2rmb/ploy/internal/errors"
	policy "github.com/iw2rmb/ploy/internal/policy"
	"github.com/iw2rmb/ploy/internal/preview"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/utils"
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
	ARFHandler              *arf.Handler
	AnalysisHandler         *analysis.Handler
	CoordinationManager     *coordination.CoordinationManager
	BlueGreenManager        *bluegreen.Manager
	Metrics                 *metrics.Metrics
	StorageConfigPath       string
	// StorageFactory deprecated: use config service
	ARFEngine  arfcore.Engine
	ARFRecipes tarfrecipes.Registry
}

// ControllerConfig holds configuration for controller initialization
type ControllerConfig struct {
	Port              string
	ConsulAddr        string
	NomadAddr         string
	StorageConfigPath string
	CleanupConfigPath string
	UseConsulEnv      bool
	EnvStorePath      string
	CleanupAutoStart  bool
	ShutdownTimeout   time.Duration
	EnableCaching     bool
	// ARF default packs (e.g., "rewrite-java:8.1.0,rewrite-spring:5.0.0"). If set, indexer runs at startup.
	ArfDefaultPacks string
	// Optional Fetcher for ARF indexer (used in tests). When nil, indexing is skipped.
	ArfFetcher trecipes.Fetcher
	// Optional ARF registry URL for HTTPFetcher. Used only if ArfFetcher is nil.
	ArfRegistryURL string
	// Optional Maven group for MavenFetcher. If set, MavenFetcher is used.
	ArfMavenGroup string
}

// parseIntEnv parses integer from environment variable with fallback
func parseIntEnv(envVar string, defaultVal int) int {
	if val := os.Getenv(envVar); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return defaultVal
}

// LoadConfigFromEnv loads controller configuration from environment variables
func LoadConfigFromEnv() *ControllerConfig {
	// Prioritize NOMAD_PORT_http for dynamic port allocation, fall back to PORT, then default
	port := os.Getenv("NOMAD_PORT_http")
	if port == "" {
		port = utils.Getenv("PORT", "8081")
	}

	return &ControllerConfig{
		Port:              port,
		ConsulAddr:        utils.Getenv("CONSUL_HTTP_ADDR", "127.0.0.1:8500"),
		NomadAddr:         utils.Getenv("NOMAD_ADDR", "http://127.0.0.1:4646"),
		StorageConfigPath: getStorageConfigPath(),
		CleanupConfigPath: utils.Getenv("PLOY_CLEANUP_CONFIG", ""),
		UseConsulEnv:      utils.Getenv("PLOY_USE_CONSUL_ENV", "true") == "true",
		EnvStorePath:      utils.Getenv("PLOY_ENV_STORE_PATH", "/tmp/ploy-env-store"),
		CleanupAutoStart:  utils.Getenv("PLOY_CLEANUP_AUTO_START", "true") == "true",
		ShutdownTimeout:   30 * time.Second, // Graceful shutdown timeout
		EnableCaching:     utils.Getenv("PLOY_ENABLE_CACHING", "true") == "true",
		ArfDefaultPacks:   utils.Getenv("PLOY_ARF_DEFAULT_PACKS", ""),
		ArfRegistryURL:    utils.Getenv("PLOY_ARF_REGISTRY", "https://registry.dev.ployman.app"),
		ArfMavenGroup:     utils.Getenv("PLOY_ARF_MAVEN_GROUP", ""),
	}
}

// getStorageConfigPath resolves the storage configuration path without depending on api/config.
// Order: env override -> common external paths -> embedded default.
func getStorageConfigPath() string {
	if path := os.Getenv("PLOY_STORAGE_CONFIG"); path != "" {
		return path
	}
	externalPaths := []string{
		"/etc/ploy/storage/config.yaml",
		"/etc/ploy/config.yaml",
	}
	for _, p := range externalPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "configs/storage-config.yaml"
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

	// Optional: run ARF recipes indexer at startup if default packs configured
	if config != nil && strings.TrimSpace(config.ArfDefaultPacks) != "" {
		// Resolve unified storage via config service if available
		var store internalStorage.Storage
		if cfgService != nil {
			if st, err := resolveStorageFromConfigService(cfgService); err == nil {
				store = st
			}
		}
		// Determine fetcher
		fetcher := config.ArfFetcher
		// Track chosen registry and group for logging
		registryBase := ""
		mavenGroup := ""
		// Prefer MavenFetcher if group configured
		if fetcher == nil {
			base := strings.TrimSpace(os.Getenv("PLOY_ARF_REGISTRY"))
			if base == "" {
				base = strings.TrimSpace(config.ArfRegistryURL)
			}
			if strings.TrimSpace(config.ArfMavenGroup) != "" && base != "" {
				fetcher = trecipes.MavenFetcher{BaseURL: base, GroupID: config.ArfMavenGroup}
				registryBase = base
				mavenGroup = config.ArfMavenGroup
			}
		}
		// Fallback to HTTPFetcher if no Maven group provided
		if fetcher == nil {
			base := strings.TrimSpace(os.Getenv("PLOY_ARF_REGISTRY"))
			if base == "" {
				base = strings.TrimSpace(config.ArfRegistryURL)
			}
			if base != "" {
				fetcher = trecipes.HTTPFetcher{BaseURL: base}
				registryBase = base
			}
		}
		if store != nil && fetcher != nil {
			idx := trecipes.NewIndexer(fetcher, store)
			// Parse packs spec: name:version[,name:version]
			specs := []trecipes.PackSpec{}
			for _, part := range strings.Split(config.ArfDefaultPacks, ",") {
				if part = strings.TrimSpace(part); part != "" {
					nv := strings.SplitN(part, ":", 2)
					if len(nv) == 2 {
						specs = append(specs, trecipes.PackSpec{Name: nv[0], Version: nv[1]})
					}
				}
			}
			if len(specs) > 0 {
				// Log which fetcher and packs are configured
				fetcherType := "custom"
				switch fetcher.(type) {
				case trecipes.MavenFetcher:
					fetcherType = "maven"
				case trecipes.HTTPFetcher:
					fetcherType = "http"
				}
				log.Printf("ARF indexer configured: fetcher=%s base=%s group=%s packs=%s", fetcherType, registryBase, mavenGroup, config.ArfDefaultPacks)
				server.indexerStorage = store
				go idx.Refresh(context.Background(), specs)
			}
		} else {
			log.Printf("ARF indexer skipped: storage or fetcher unavailable (base=%s group=%s packs=%s)", registryBase, mavenGroup, config.ArfDefaultPacks)
		}
	} else {
		log.Printf("ARF indexer disabled: no default packs configured")
	}

	return server, nil
}

// initializeDependencies initializes all external service dependencies
func initializeDependencies(cfg *ControllerConfig) (*ServiceDependencies, error) {
	return initializeDependenciesWithService(cfg, nil)
}

// initializeDependenciesWithService initializes dependencies, preferring a centralized
// configuration service when provided for validation and health wiring.
func initializeDependenciesWithService(cfg *ControllerConfig, cfgService *cfgsvc.Service) (*ServiceDependencies, error) {
	startTime := time.Now()
	log.Printf("Initializing service dependencies (caching: %v)", cfg.EnableCaching)
	log.Printf("Configuration: Port=%s, ConsulAddr=%s, NomadAddr=%s", cfg.Port, cfg.ConsulAddr, cfg.NomadAddr)

	// Validate storage configuration at startup
	if cfgService != nil {
		// Prefer centralized service for validation
		if err := cfgService.Reload(); err != nil {
			log.Printf("Warning: Storage configuration validation via service failed: %v", err)
		} else {
			log.Printf("✓ Storage configuration validated via service")
		}
	} else {
		log.Printf("Validating storage configuration from: %s", cfg.StorageConfigPath)
		if cfg.StorageConfigPath != "" {
			if _, err := cfgsvc.New(
				cfgsvc.WithFile(cfg.StorageConfigPath),
				cfgsvc.WithValidation(cfgsvc.NewStructValidator()),
			); err != nil {
				log.Printf("Warning: Storage configuration validation failed: %v", err)
			} else {
				log.Printf("✓ Storage configuration validated successfully")
			}
		}
	}

	// Initialize environment store with fallback logic
	log.Printf("Initializing environment store...")
	envStore, err := initializeEnvStore(cfg)
	if err != nil {
		log.Printf("❌ Failed to initialize environment store: %v", err)
		return nil, fmt.Errorf("failed to initialize environment store: %w", err)
	}
	log.Printf("✓ Environment store initialized successfully")

	// Initialize Traefik router
	log.Printf("Initializing Traefik router with Consul address: %s", cfg.ConsulAddr)
	traefikRouter, err := initializeTraefikRouter(cfg.ConsulAddr)
	if err != nil {
		log.Printf("⚠️  Failed to initialize Traefik router: %v", err)
	} else {
		log.Printf("✓ Traefik router initialized successfully")
	}

	// Storage factory removed; using centralized config service for storage resolution

	// Initialize health checker
	log.Printf("Initializing health checker...")
	healthChecker := health.NewHealthChecker(cfg.StorageConfigPath, cfg.ConsulAddr, cfg.NomadAddr)
	log.Printf("✓ Health checker initialized")

	// Initialize TTL cleanup service
	cleanupHandler, ttlService, err := initializeCleanupService(cfg)
	if err != nil {
		log.Printf("Warning: Failed to initialize cleanup service: %v", err)
	}

	// Initialize self-update handler
	selfUpdateHandler, err := initializeSelfUpdateHandler(cfg, cfgService)
	if err != nil {
		log.Printf("Warning: Failed to initialize self-update handler: %v", err)
	}

	// Initialize DNS handler
	dnsHandler, err := initializeDNSHandler(cfg.ConsulAddr)
	if err != nil {
		log.Printf("Warning: Failed to initialize DNS handler: %v", err)
	}

	// Initialize Certificate Manager
	certificateManager, err := initializeCertificateManager(cfg, cfgService)
	if err != nil {
		log.Printf("Warning: Failed to initialize certificate manager: %v", err)
	}

	// Initialize Platform Wildcard Certificate Manager
	var platformWildcardManager *certificates.PlatformWildcardCertificateManager
	if certificateManager != nil {
		platformWildcardManager, err = certificates.NewPlatformWildcardCertificateManager(certificateManager)
		if err != nil {
			log.Printf("Warning: Failed to initialize platform wildcard certificate manager: %v", err)
		} else if platformWildcardManager != nil {
			// Integrate platform wildcard manager with certificate manager
			certificateManager.SetPlatformWildcardManager(platformWildcardManager)
		}
	}

	// Initialize ARF Engine (consolidation Phase 4 - initial slice)
	arfEngine := arfcore.NewEngine(arfcore.EngineConfig{})
	// Prefer storage-backed recipes registry when storage is resolvable
	var arfRecipes tarfrecipes.Registry = tarfrecipes.NewInMemory()
	if cfgService != nil {
		if st, err := resolveStorageFromConfigService(cfgService); err == nil && st != nil {
			arfRecipes = tarfrecipes.NewStorageBacked(st)
		}
	}

	// Initialize ARF Handler
	arfHandler, err := initializeARFHandlerWithService(cfg, cfgService)
	if err != nil {
		log.Printf("Warning: Failed to initialize ARF handler: %v", err)
	}

	// Initialize Analysis Handler
	analysisHandler, err := initializeAnalysisHandler(cfg, arfHandler, cfgService)
	if err != nil {
		log.Printf("Warning: Failed to initialize analysis handler: %v", err)
	}

	// Initialize Metrics
	metricsInstance := metrics.NewMetrics()

	// Initialize Coordination Manager with metrics
	coordinationManager, err := initializeCoordinationManagerWithMetrics(cfg, metricsInstance)
	if err != nil {
		log.Printf("Warning: Failed to initialize coordination manager: %v", err)
	}

	// Initialize Blue-Green Manager
	blueGreenManager, err := initializeBlueGreenManager(cfg)
	if err != nil {
		log.Printf("Warning: Failed to initialize blue-green manager: %v", err)
	}

	deps := &ServiceDependencies{
		EnvStore:                envStore,
		TraefikRouter:           traefikRouter,
		HealthChecker:           healthChecker,
		CleanupHandler:          cleanupHandler,
		TTLCleanupService:       ttlService,
		SelfUpdateHandler:       selfUpdateHandler,
		DNSHandler:              dnsHandler,
		CertificateManager:      certificateManager,
		PlatformWildcardManager: platformWildcardManager,
		ARFHandler:              arfHandler,
		AnalysisHandler:         analysisHandler,
		CoordinationManager:     coordinationManager,
		BlueGreenManager:        blueGreenManager,
		Metrics:                 metricsInstance,
		StorageConfigPath:       cfg.StorageConfigPath,
		ARFEngine:               arfEngine,
		ARFRecipes:              arfRecipes,
	}

	// Record startup time
	startupDuration := time.Since(startTime)
	if metricsInstance != nil {
		metricsInstance.RecordStartupTime(startupDuration)
	}

	log.Printf("Service dependencies initialized successfully in %v (caching: %v)",
		startupDuration, cfg.EnableCaching)
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
func initializeSelfUpdateHandler(cfg *ControllerConfig, cfgService *cfgsvc.Service) (*selfupdate.Handler, error) {
	// Create storage client for self-update operations
	if cfgService == nil {
		return nil, fmt.Errorf("config service required for self-update handler")
	}
	unified, err := resolveStorageFromConfigService(cfgService)
	if err != nil {
		return nil, fmt.Errorf("resolve storage for self-update: %w", err)
	}
	provider := internalStorage.NewProviderFromStorage(unified, "artifacts")

	// Get current controller version
	currentVersion := selfupdate.GetCurrentVersion()

	// Create self-update handler
	handler, err := selfupdate.NewHandler(provider, cfg.ConsulAddr, currentVersion)
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

// initializeCertificateManager initializes the certificate manager
func initializeCertificateManager(cfg *ControllerConfig, cfgService *cfgsvc.Service) (*certificates.CertificateManager, error) {
	// Create Consul client
	consulConfig := consulapi.DefaultConfig()
	if cfg.ConsulAddr != "" {
		consulConfig.Address = cfg.ConsulAddr
	}
	consulClient, err := consulapi.NewClient(consulConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client: %w", err)
	}

	// Create storage client
	if cfgService == nil {
		return nil, fmt.Errorf("config service required for certificate manager")
	}
	storageClient, err := resolveStorageFromConfigService(cfgService)
	if err != nil {
		return nil, fmt.Errorf("resolve storage for certificates: %w", err)
	}

	// Create DNS provider (for ACME DNS-01 challenges)
	// Note: DNS provider can be nil for now, certificate manager should handle this gracefully
	dnsProvider, err := initializeDNSProvider()
	if err != nil {
		log.Printf("Warning: DNS provider initialization failed, certificates may not work: %v", err)
		dnsProvider = nil
	}

	// Create certificate manager (it should handle nil DNS provider gracefully)
	certificateManager, err := certificates.NewCertificateManager(consulClient, storageClient, dnsProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate manager: %w", err)
	}

	log.Printf("Certificate manager initialized successfully (DNS provider: %v)", dnsProvider != nil)
	return certificateManager, nil
}

// initializeDNSProvider creates a DNS provider for ACME challenges
func initializeDNSProvider() (dns.Provider, error) {
	// Get DNS provider type from environment
	providerType := os.Getenv("PLOY_APPS_DOMAIN_PROVIDER")
	if providerType == "" {
		log.Printf("PLOY_APPS_DOMAIN_PROVIDER not set, DNS provider disabled")
		return nil, nil
	}

	log.Printf("Initializing DNS provider: %s", providerType)

	switch strings.ToLower(providerType) {
	case "namecheap":
		return initializeNamecheapProvider()
	default:
		return nil, fmt.Errorf("unsupported DNS provider: %s", providerType)
	}
}

// initializeNamecheapProvider creates a Namecheap DNS provider
func initializeNamecheapProvider() (dns.Provider, error) {
	// Get API key from either production or sandbox environment
	apiKey := os.Getenv("NAMECHEAP_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("NAMECHEAP_SANDBOX_API_KEY")
	}

	config := dns.NamecheapConfig{
		APIUser:  os.Getenv("NAMECHEAP_API_USER"),
		APIKey:   apiKey,
		Username: os.Getenv("NAMECHEAP_USERNAME"),
		ClientIP: os.Getenv("NAMECHEAP_CLIENT_IP"),
		Sandbox:  os.Getenv("NAMECHEAP_SANDBOX") == "true",
	}

	// Validate required configuration
	if config.APIUser == "" || config.APIKey == "" || config.Username == "" || config.ClientIP == "" {
		return nil, fmt.Errorf("Namecheap DNS provider requires NAMECHEAP_API_USER, NAMECHEAP_API_KEY (or NAMECHEAP_SANDBOX_API_KEY), NAMECHEAP_USERNAME, and NAMECHEAP_CLIENT_IP environment variables")
	}

	log.Printf("Creating Namecheap DNS provider (sandbox: %v, user: %s, client_ip: %s)", config.Sandbox, config.APIUser, config.ClientIP)

	provider, err := dns.NewNamecheapProvider(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Namecheap provider: %w", err)
	}

	// Note: In production, validate configuration by making API call
	// For demonstration, we skip validation if using placeholder credentials
	if !strings.Contains(config.APIKey, "placeholder") {
		if err := provider.ValidateConfiguration(); err != nil {
			return nil, fmt.Errorf("Namecheap provider configuration validation failed: %w", err)
		}
		log.Printf("Namecheap DNS provider validated successfully")
	} else {
		log.Printf("Namecheap DNS provider created with placeholder credentials (validation skipped)")
	}

	log.Printf("Namecheap DNS provider initialized successfully")
	return provider, nil
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

// setupRoutes configures all API routes with dependency injection
func (s *Server) setupRoutes() {
	// Health and readiness endpoints (before API group)
	s.app.Get("/health", s.dependencies.HealthChecker.HealthHandler)
	s.app.Get("/ready", s.dependencies.HealthChecker.ReadinessHandler)
	s.app.Get("/live", s.dependencies.HealthChecker.LivenessHandler)
	s.app.Get("/health/metrics", s.dependencies.HealthChecker.MetricsHandler)
	s.app.Get("/health/deployment", s.dependencies.HealthChecker.DeploymentStatusHandler)
	s.app.Get("/health/update", s.dependencies.HealthChecker.UpdateStatusHandler)
	s.app.Get("/health/platform-certificates", s.handlePlatformCertificateHealth)
	s.app.Get("/health/coordination", s.handleCoordinationHealth)

	// Prometheus metrics endpoint
	if s.dependencies.Metrics != nil {
		s.app.Get("/metrics", s.dependencies.Metrics.Handler())
		log.Printf("Prometheus metrics endpoint configured at /metrics")
	}

	api := s.app.Group("/v1")

	// Application build endpoints with request-scoped storage
	api.Post("/apps/:app/builds", s.handleTriggerAppBuild) // apps namespace
	api.Get("/apps", build.ListApps)
	api.Get("/apps/:app/status", build.Status)
	api.Get("/apps/:app/logs", build.GetLogs)

	// Platform service endpoints with platform namespace
	api.Post("/platform/:service/builds", s.handleTriggerPlatformBuild)

	// Legacy build endpoint (backward compatibility - defaults to apps namespace)
	api.Post("/builds/:app", s.handleTriggerBuild)

	// Domain management with dependency injection
	s.setupDomainRoutes(api)

	// Certificate management (Heroku-style)
	s.setupCertificateRoutes(api)

	// Environment variables management with injected env store
	api.Post("/apps/:app/env", s.handleSetEnvVars)
	api.Get("/apps/:app/env", s.handleGetEnvVars)
	api.Put("/apps/:app/env/:key", s.handleSetEnvVar)
	api.Delete("/apps/:app/env/:key", s.handleDeleteEnvVar)

	// Debug, rollback, and destroy with dependency injection
	api.Post("/apps/:app/debug", s.handleDebugApp)
	api.Post("/apps/:app/rollback", debug.RollbackApp)
	api.Delete("/apps/:app", s.handleDestroyApp)

	// Blue-Green deployment endpoints
	s.setupBlueGreenRoutes(api)

	// Platform service routes (separate from regular apps to avoid conflicts)
	s.setupPlatformRoutes(api)

	// Storage endpoints with request-scoped clients
	api.Get("/storage/health", s.handleStorageHealth)
	api.Get("/storage/metrics", s.handleStorageMetrics)

	// Configuration management endpoints
	api.Get("/storage/config", s.handleGetStorageConfig)
	api.Post("/storage/config/reload", s.handleReloadStorageConfig)
	api.Post("/storage/config/validate", s.handleValidateStorageConfig)

	// ARF recipes minimal facade endpoint (Phase 4 initial slice)
	api.Get("/arf/recipes/ping", s.handleARFRecipesPing)
	api.Get("/arf/recipes", s.handleARFRecipesList)
	api.Get("/arf/recipes/search", s.handleARFRecipesSearch)
	api.Get("/arf/recipes/:id", s.handleARFRecipesGet)

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

	// ARF (Automated Remediation Framework) endpoints
	if s.dependencies.ARFHandler != nil {
		s.dependencies.ARFHandler.RegisterRoutes(s.app)
		log.Printf("ARF routes registered successfully")
	}

	// Internal ARF recipes handlers are now the default; legacy overlay removed

	// Static Analysis endpoints
	if s.dependencies.AnalysisHandler != nil {
		s.dependencies.AnalysisHandler.RegisterRoutes(s.app)
		log.Printf("Static Analysis routes registered successfully")
	}

	// Template management endpoints
	templateHandler, err := initializeTemplateHandler()
	if err != nil {
		log.Printf("Warning: Failed to initialize template handler: %v", err)
	} else {
		templates.SetupRoutes(s.app, templateHandler)
		log.Printf("Template management routes registered successfully")
	}

	// Version endpoints
	version.RegisterRoutes(s.app)

	// Health endpoints in API group for versioned access
	api.Get("/health", s.dependencies.HealthChecker.HealthHandler)
	api.Get("/ready", s.dependencies.HealthChecker.ReadinessHandler)
	api.Get("/live", s.dependencies.HealthChecker.LivenessHandler)
	api.Get("/health/metrics", s.dependencies.HealthChecker.MetricsHandler)
	api.Get("/health/deployment", s.dependencies.HealthChecker.DeploymentStatusHandler)

	log.Printf("API routes configured with dependency injection")
}

// setupRecipesCatalogRoutes wires the lightweight recipes catalog endpoints.
// It overlays:
//  - GET /v1/arf/recipes
//  - GET /v1/arf/recipes/:id
//  - POST /v1/arf/recipes/refresh
// onto the main router using the dedicated RecipesHandler.
// Legacy recipes catalog overlay removed in Phase 4; internal handlers are now default.

// setupDomainRoutes configures domain management routes
func (s *Server) setupDomainRoutes(api fiber.Router) {
	if s.dependencies.TraefikRouter != nil {
		// Use new Traefik-based domain management
		domainHandler := domains.NewDomainHandler(s.dependencies.TraefikRouter, s.dependencies.CertificateManager)
		domains.SetupDomainRoutes(s.app, domainHandler)
	} else {
		// Fallback to existing domain management
		api.Post("/apps/:app/domains", domain.AddDomain)
		api.Get("/apps/:app/domains", domain.ListDomains)
		api.Delete("/apps/:app/domains/:domain", domain.RemoveDomain)
	}
}

// setupCertificateRoutes configures Heroku-style certificate management routes
func (s *Server) setupCertificateRoutes(api fiber.Router) {
	if s.dependencies.CertificateManager != nil {
		// Heroku-style certificate management routes
		api.Get("/apps/:app/certificates", s.handleListAppCertificates)
		api.Get("/apps/:app/certificates/:domain", s.handleGetDomainCertificate)
		api.Post("/apps/:app/certificates/:domain/provision", s.handleProvisionCertificate)
		api.Post("/apps/:app/certificates/:domain/upload", s.handleUploadCertificate)
		api.Delete("/apps/:app/certificates/:domain", s.handleRemoveCertificate)

		log.Printf("Certificate management routes configured")
	} else {
		log.Printf("Certificate management routes skipped - certificate manager not available")
	}
}

// setupBlueGreenRoutes configures blue-green deployment routes
func (s *Server) setupBlueGreenRoutes(api fiber.Router) {
	if s.dependencies.BlueGreenManager != nil {
		// Blue-Green deployment management routes
		api.Post("/apps/:app/deploy/blue-green", s.handleStartBlueGreenDeployment)
		api.Get("/apps/:app/blue-green/status", s.handleGetBlueGreenStatus)
		api.Post("/apps/:app/blue-green/shift", s.handleShiftTraffic)
		api.Post("/apps/:app/blue-green/auto-shift", s.handleAutoShiftTraffic)
		api.Post("/apps/:app/blue-green/complete", s.handleCompleteBlueGreenDeployment)
		api.Post("/apps/:app/blue-green/rollback", s.handleRollbackBlueGreenDeployment)

		log.Printf("Blue-Green deployment routes configured")
	} else {
		log.Printf("Blue-Green deployment routes skipped - blue-green manager not available")
	}
}

// setupPlatformRoutes configures platform service routes
func (s *Server) setupPlatformRoutes(api fiber.Router) {
	// Platform services use separate routes to avoid conflicts with regular apps
	platformAPI := api.Group("/platform")

	// Platform deployment endpoints
	platformAPI.Post("/:service/deploy", s.handlePlatformDeploy)
	platformAPI.Get("/:service/status", s.handlePlatformStatus)
	platformAPI.Post("/:service/rollback", s.handlePlatformRollback)
	platformAPI.Delete("/:service", s.handlePlatformRemove)
	platformAPI.Get("/:service/logs", s.handlePlatformLogs)

	log.Printf("Platform service routes configured at /v1/platform/*")
}

// handleListAppCertificates lists certificates for an app
func (s *Server) handleListAppCertificates(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
	}

	if s.dependencies.CertificateManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Certificate management not available"})
	}

	certificates, err := s.dependencies.CertificateManager.ListAppCertificates(appName)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to list certificates: %v", err)})
	}

	return c.JSON(fiber.Map{
		"status":       "success",
		"app":          appName,
		"certificates": certificates,
		"count":        len(certificates),
	})
}

// handleGetDomainCertificate gets certificate info for a domain
func (s *Server) handleGetDomainCertificate(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")

	if appName == "" || domain == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name and domain are required"})
	}

	if s.dependencies.CertificateManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Certificate management not available"})
	}

	certificate, err := s.dependencies.CertificateManager.GetDomainCertificate(appName, domain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": fmt.Sprintf("Certificate not found: %v", err)})
	}

	return c.JSON(certificate)
}

// handleProvisionCertificate manually provisions a certificate for a domain
func (s *Server) handleProvisionCertificate(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")

	if appName == "" || domain == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name and domain are required"})
	}

	if s.dependencies.CertificateManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Certificate management not available"})
	}

	ctx := context.Background()
	certificate, err := s.dependencies.CertificateManager.ProvisionCertificate(ctx, appName, domain)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to provision certificate: %v", err)})
	}

	return c.JSON(fiber.Map{
		"status":      "provisioned",
		"app":         appName,
		"domain":      domain,
		"certificate": certificate,
	})
}

// handleRemoveCertificate removes a certificate for a domain
func (s *Server) handleRemoveCertificate(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")

	if appName == "" || domain == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name and domain are required"})
	}

	if s.dependencies.CertificateManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Certificate management not available"})
	}

	err := s.dependencies.CertificateManager.RemoveDomainCertificate(appName, domain)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to remove certificate: %v", err)})
	}

	return c.JSON(fiber.Map{
		"status": "removed",
		"app":    appName,
		"domain": domain,
	})
}

// handleUploadCertificate handles uploading custom certificate bundles
func (s *Server) handleUploadCertificate(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")

	if appName == "" || domain == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name and domain are required"})
	}

	if s.dependencies.CertificateManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Certificate management not available"})
	}

	// Parse multipart form
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Failed to parse multipart form: %v", err)})
	}

	// Get certificate data
	certFiles := form.Value["certificate"]
	if len(certFiles) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Certificate is required"})
	}
	certificate := []byte(certFiles[0])

	// Get private key data
	keyFiles := form.Value["private_key"]
	if len(keyFiles) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Private key is required"})
	}
	privateKey := []byte(keyFiles[0])

	// Get CA certificate data (optional)
	var caCert []byte
	caFiles := form.Value["ca_certificate"]
	if len(caFiles) > 0 {
		caCert = []byte(caFiles[0])
	}

	// Create certificate record
	ctx := context.Background()
	domainCert, err := s.dependencies.CertificateManager.UploadCustomCertificate(ctx, appName, domain, certificate, privateKey, caCert)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to upload certificate: %v", err)})
	}

	return c.JSON(fiber.Map{
		"status":      "uploaded",
		"app":         appName,
		"domain":      domain,
		"certificate": domainCert,
		"message":     "Custom certificate uploaded successfully",
	})
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
	log.Printf("Received shutdown signal, initiating graceful shutdown...")

	return s.Shutdown()
}

// Shutdown performs graceful shutdown with connection draining
func (s *Server) Shutdown() error {
	log.Printf("Starting graceful shutdown procedure")

	// Cancel coordination context first to stop leader election
	if s.coordinationCancel != nil {
		log.Printf("Stopping coordination manager")
		s.coordinationCancel()

		// Give coordination manager time to clean up
		time.Sleep(2 * time.Second)
	}

	// Stop coordination manager
	if s.dependencies.CoordinationManager != nil {
		s.dependencies.CoordinationManager.Stop()
		log.Printf("Coordination manager stopped")
	}

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
	defer cancel()

	// Stop TTL cleanup service
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

// ensurePlatformWildcardCertificate ensures platform wildcard certificate is provisioned
func (s *Server) ensurePlatformWildcardCertificate() error {
	if s.dependencies.PlatformWildcardManager == nil {
		return nil // Platform wildcard management disabled
	}

	// Validate platform domain configuration
	if err := s.dependencies.PlatformWildcardManager.ValidatePlatformDomain(); err != nil {
		return fmt.Errorf("platform domain validation failed: %w", err)
	}

	// Create context with timeout for certificate provisioning
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Printf("Ensuring platform wildcard certificate for domain: %s",
		s.dependencies.PlatformWildcardManager.GetPlatformDomain())

	// Provision platform wildcard certificate if needed
	if err := s.dependencies.PlatformWildcardManager.EnsurePlatformWildcardCertificate(ctx); err != nil {
		return fmt.Errorf("failed to ensure platform wildcard certificate: %w", err)
	}

	log.Printf("Platform wildcard certificate provisioning completed successfully")
	return nil
}

// registerControllerWithTraefik registers the controller with Traefik for platform domain access
func (s *Server) registerControllerWithTraefik() error {
	if s.dependencies.TraefikRouter == nil {
		log.Printf("Traefik router not available, skipping controller registration")
		return nil
	}

	// Get Nomad allocation information from environment
	allocID := os.Getenv("NOMAD_ALLOC_ID")
	allocIP := os.Getenv("NOMAD_IP_http")

	if allocID == "" || allocIP == "" {
		log.Printf("Nomad allocation information not available (NOMAD_ALLOC_ID=%s, NOMAD_IP_http=%s), skipping Traefik registration", allocID, allocIP)
		return nil
	}

	// Parse port from server configuration
	port := 8081 // Default port
	if s.config.Port != "" {
		if parsedPort, err := strconv.Atoi(s.config.Port); err == nil {
			port = parsedPort
		}
	}

	// Register controller with Traefik
	if err := s.dependencies.TraefikRouter.RegisterController(allocID, allocIP, port); err != nil {
		return fmt.Errorf("failed to register controller with Traefik: %w", err)
	}

	controllerDomain := s.dependencies.TraefikRouter.GenerateControllerDomain()
	log.Printf("Controller registered with Traefik, accessible at: https://%s", controllerDomain)
	return nil
}

// handlePlatformCertificateHealth handles platform wildcard certificate health checks
func (s *Server) handlePlatformCertificateHealth(c *fiber.Ctx) error {
	if s.dependencies.PlatformWildcardManager == nil || !s.dependencies.PlatformWildcardManager.IsEnabled() {
		return c.JSON(fiber.Map{
			"status":  "disabled",
			"message": "Platform wildcard certificate management disabled (PLOY_APPS_DOMAIN not set)",
		})
	}

	ctx := context.Background()
	cert, err := s.dependencies.PlatformWildcardManager.GetPlatformWildcardCertificate(ctx)
	if err != nil {
		return c.Status(503).JSON(fiber.Map{
			"status": "error",
			"error":  err.Error(),
			"domain": s.dependencies.PlatformWildcardManager.GetWildcardDomain(),
		})
	}

	daysUntilExpiry := int(time.Until(cert.ExpiresAt).Hours() / 24)

	// Determine health status based on expiry
	status := "healthy"
	if daysUntilExpiry <= 7 {
		status = "expiring_soon"
	} else if daysUntilExpiry <= 1 {
		status = "critical"
	}

	return c.JSON(fiber.Map{
		"status":             status,
		"platform_domain":    s.dependencies.PlatformWildcardManager.GetPlatformDomain(),
		"wildcard_domain":    cert.Domain,
		"expires_at":         cert.ExpiresAt,
		"days_until_expiry":  daysUntilExpiry,
		"issued_at":          cert.IssuedAt,
		"auto_renew_enabled": true,
	})
}

// initializeARFHandler initializes the Automated Remediation Framework handler
func initializeARFHandler(cfg *ControllerConfig) (*arf.Handler, error) {
	return initializeARFHandlerWithService(cfg, nil)
}

// initializeARFHandlerWithService prefers unified storage resolved from the centralized
// config service when provided, with file-based factory as a fallback.
func initializeARFHandlerWithService(cfg *ControllerConfig, cfgService *cfgsvc.Service) (*arf.Handler, error) {
	log.Printf("Initializing ARF (Automated Remediation Framework)")

	// Load ARF configuration from environment
	arfConfig := arf.LoadConfigFromEnv()

	// Validate configuration
	if err := arfConfig.Validate(); err != nil {
		return nil, fmt.Errorf("ARF configuration validation failed: %w", err)
	}

	log.Printf("ARF configuration loaded: storage=%s, index=%s, validation=%v",
		arfConfig.Storage.Backend, arfConfig.Index.Backend, arfConfig.Validation.Enabled)

	// Initialize sandbox manager
	jailBaseDir := utils.Getenv("ARF_JAIL_BASE_DIR", "/jail/arf")
	jailTemplateDir := utils.Getenv("ARF_JAIL_TEMPLATE_DIR", "/jail/template")
	jailInterface := utils.Getenv("ARF_JAIL_INTERFACE", "lo0")
	maxSandboxes := 10
	defaultTTL := 30 * time.Minute

	sandboxMgr := arf.NewSandboxManagerForOS(jailBaseDir, jailTemplateDir, maxSandboxes, defaultTTL, jailInterface)

	// Initialize storage backend
	recipeStorage, err := arfConfig.InitializeStorage()
	if err != nil {
		log.Printf("Warning: Failed to initialize ARF storage backend, falling back to in-memory: %v", err)
		recipeStorage = arf.NewInMemoryRecipeStorage()
	}

	// Initialize index backend
	recipeIndex, err := arfConfig.InitializeIndex()
	if err != nil {
		log.Printf("Warning: Failed to initialize ARF index backend: %v", err)
		recipeIndex = nil
	}

	// Initialize validator
	recipeValidator := arfConfig.InitializeValidator()

	// Initialize OpenRewrite dispatcher for dynamic recipe downloading
	var openRewriteDispatcher *arf.OpenRewriteDispatcher
	nomadAddr := utils.Getenv("NOMAD_ADDR", "http://nomad.service.consul:4646")
	registryURL := utils.Getenv("PLOY_REGISTRY_URL", "registry.dev.ployman.app")
	seaweedfsURL := utils.Getenv("SEAWEEDFS_URL", "http://seaweedfs-filer.service.consul:8888")
	apiURL := utils.Getenv("PLOY_API_URL", "http://api.service.consul:8081")

	// Create storage provider for OpenRewrite dispatcher
	var storageProvider internalStorage.StorageProvider
	seaweedConfig := internalStorage.SeaweedFSConfig{
		Master:      seaweedfsURL,
		Filer:       seaweedfsURL,
		Collection:  "artifacts",
		Replication: "000",
		Timeout:     30,
	}
	seaweedClient, err := internalStorage.NewSeaweedFSClient(seaweedConfig)
	if err != nil {
		log.Printf("Warning: Failed to create SeaweedFS client for OpenRewrite dispatcher: %v", err)
		storageProvider = nil
	} else {
		// Use the raw client as provider
		storageProvider = seaweedClient
	}

	if storageProvider != nil {
		log.Printf("Creating OpenRewrite dispatcher with: nomad=%s, registry=%s, seaweedfs=%s, api=%s",
			nomadAddr, registryURL, seaweedfsURL, apiURL)

		// Create ARF service with unified storage interface
		// Storage already has "artifacts" bucket configured via collection in storage config
		// ARFService should not add any bucket prefix - storage handles it internally
		// Centralized config service is required for unified storage
		if cfgService == nil {
			return nil, fmt.Errorf("config service is required for ARF unified storage")
		}
		unifiedStorage, err := resolveStorageFromConfigService(cfgService)
		if err != nil {
			log.Printf("ERROR: Failed to create unified storage for ARF via config service: %v", err)
			return nil, fmt.Errorf("failed to create unified storage for ARF: %w", err)
		}

		arfStorageService, err := arf.NewARFService(unifiedStorage)
		if err != nil {
			log.Printf("ERROR: Failed to create ARF service: %v", err)
			return nil, fmt.Errorf("failed to create ARF service: %w", err)
		}
		log.Printf("ARF service created with unified storage (bucket handled by storage layer)")

		openRewriteDispatcher, err = arf.NewOpenRewriteDispatcher(
			nomadAddr,
			registryURL,
			seaweedfsURL,
			apiURL,
			arfStorageService,
		)
		if err != nil {
			log.Printf("ERROR: Failed to create OpenRewrite dispatcher: %v", err)
			log.Printf("This will prevent OpenRewrite transformations from working")
			openRewriteDispatcher = nil
		} else {
			log.Printf("SUCCESS: OpenRewrite dispatcher initialized for dynamic recipe downloading")
		}
	} else {
		log.Printf("WARNING: No storage provider available - OpenRewrite dispatcher will not be initialized")
		log.Printf("Check SeaweedFS connectivity at: %s", seaweedfsURL)
	}

	// Initialize recipe executor with optional dispatcher
	engine := arf.NewRecipeExecutor(recipeStorage, sandboxMgr, openRewriteDispatcher)
	if openRewriteDispatcher != nil {
		log.Printf("SUCCESS: Recipe executor initialized WITH OpenRewrite dispatcher for fallback execution")
	} else {
		log.Printf("WARNING: Recipe executor initialized WITHOUT OpenRewrite dispatcher")
		log.Printf("OpenRewrite recipes that are not in storage will fail")
	}

	// Create ARF handler based on available backends
	var handler *arf.Handler
	if recipeStorage != nil && (recipeIndex != nil || arfConfig.Storage.Backend == "memory") {
		// Use storage-aware handler
		handler = arf.NewHandlerWithStorage(
			engine,
			recipeStorage,
			recipeIndex,
			recipeValidator,
			sandboxMgr,
			storageProvider, // Pass the storage provider for recipe registry
		)
		log.Printf("ARF handler initialized with storage backend: %s", arfConfig.Storage.Backend)
	} else {
		// Fallback to catalog-based handler
		keyPrefix := utils.Getenv("ARF_CONSUL_PREFIX", "arf")
		catalog, err := arf.NewConsulRecipeCatalog(cfg.ConsulAddr, keyPrefix)
		if err != nil {
			return nil, fmt.Errorf("failed to create recipe catalog: %w", err)
		}

		handler = arf.NewHandler(engine, catalog, sandboxMgr)
		log.Printf("ARF handler initialized with catalog fallback")
	}

	// Initialize Consul store for async transformations (required)
	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = cfg.ConsulAddr
	consulClient, err := consulapi.NewClient(consulConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client for ARF async transformations: %w", err)
	}
	keyPrefix := utils.Getenv("ARF_CONSUL_PREFIX", "ploy/arf/transforms")
	consulStore := arf.NewConsulHealingStore(consulClient, keyPrefix)
	handler.SetConsulStore(consulStore)
	log.Printf("ARF async transformations enabled with Consul store")

	log.Printf("ARF handler initialized successfully")
	return handler, nil
}

// initializeCoordinationManager initializes the coordination manager for leader election
func initializeCoordinationManager(cfg *ControllerConfig) (*coordination.CoordinationManager, error) {
	return initializeCoordinationManagerWithMetrics(cfg, nil)
}

// initializeCoordinationManagerWithMetrics initializes the coordination manager with metrics
func initializeCoordinationManagerWithMetrics(cfg *ControllerConfig, metrics *metrics.Metrics) (*coordination.CoordinationManager, error) {
	log.Printf("Initializing coordination manager for leader election")

	coordinationMgr, err := coordination.NewCoordinationManagerWithMetrics(cfg.ConsulAddr, metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to create coordination manager: %w", err)
	}

	log.Printf("Coordination manager initialized successfully")
	return coordinationMgr, nil
}

// initializeBlueGreenManager initializes the blue-green deployment manager
func initializeBlueGreenManager(cfg *ControllerConfig) (*bluegreen.Manager, error) {
	log.Printf("Initializing blue-green deployment manager")

	// Initialize Consul client
	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = cfg.ConsulAddr
	consulClient, err := consulapi.NewClient(consulConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create consul client: %w", err)
	}

	// Initialize Nomad client
	nomadConfig := nomadapi.DefaultConfig()
	nomadConfig.Address = cfg.NomadAddr
	nomadClient, err := nomadapi.NewClient(nomadConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create nomad client: %w", err)
	}

	// Create blue-green manager
	blueGreenManager := bluegreen.NewManager(consulClient, nomadClient)

	log.Printf("Blue-green deployment manager initialized successfully")
	return blueGreenManager, nil
}

// handleCoordinationHealth handles coordination and leader election health checks
func (s *Server) handleCoordinationHealth(c *fiber.Ctx) error {
	if s.dependencies.CoordinationManager == nil {
		return c.JSON(fiber.Map{
			"status":  "disabled",
			"message": "Coordination manager not initialized",
		})
	}

	isLeader := s.dependencies.CoordinationManager.IsLeader()
	status := "follower"
	if isLeader {
		status = "leader"
	}

	response := fiber.Map{
		"status":    status,
		"is_leader": isLeader,
		"timestamp": time.Now(),
	}

	// Add TTL cleanup status if we're the leader
	if isLeader {
		// Note: TTL cleanup stats would be available through the coordination manager
		// This is a placeholder for future implementation
		response["coordination_tasks"] = fiber.Map{
			"ttl_cleanup": "active",
		}
	}

	return c.JSON(response)
}

// Blue-Green Deployment Handlers

// handleStartBlueGreenDeployment starts a new blue-green deployment
func (s *Server) handleStartBlueGreenDeployment(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
	}

	if s.dependencies.BlueGreenManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Blue-Green deployment not available"})
	}

	// Parse request body for version information
	var req struct {
		Version string `json:"version"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Version == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Version is required"})
	}

	// Start blue-green deployment
	ctx := c.Context()
	state, err := s.dependencies.BlueGreenManager.StartBlueGreenDeployment(ctx, appName, req.Version)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to start blue-green deployment: %v", err),
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"status":     "deployment_started",
		"message":    "Blue-green deployment initiated successfully",
		"deployment": state,
	})
}

// handleGetBlueGreenStatus gets the current blue-green deployment status
func (s *Server) handleGetBlueGreenStatus(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
	}

	if s.dependencies.BlueGreenManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Blue-Green deployment not available"})
	}

	// Get deployment state
	ctx := c.Context()
	state, err := s.dependencies.BlueGreenManager.GetDeploymentState(ctx, appName)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to get deployment state: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"status":     "success",
		"deployment": state,
	})
}

// handleShiftTraffic manually shifts traffic between blue and green deployments
func (s *Server) handleShiftTraffic(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
	}

	if s.dependencies.BlueGreenManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Blue-Green deployment not available"})
	}

	// Parse request body for target weight
	var req struct {
		TargetWeight int `json:"target_weight"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.TargetWeight < 0 || req.TargetWeight > 100 {
		return c.Status(400).JSON(fiber.Map{"error": "Target weight must be between 0 and 100"})
	}

	// Shift traffic
	ctx := c.Context()
	if err := s.dependencies.BlueGreenManager.ShiftTraffic(ctx, appName, req.TargetWeight); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to shift traffic: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"status":        "success",
		"message":       "Traffic shifted successfully",
		"target_weight": req.TargetWeight,
	})
}

// handleAutoShiftTraffic automatically shifts traffic using the default strategy
func (s *Server) handleAutoShiftTraffic(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
	}

	if s.dependencies.BlueGreenManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Blue-Green deployment not available"})
	}

	// Start automatic traffic shifting in background
	ctx := c.Context()
	go func() {
		if err := s.dependencies.BlueGreenManager.AutoShiftTraffic(ctx, appName); err != nil {
			log.Printf("Auto traffic shift failed for app %s: %v", appName, err)
		}
	}()

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Automatic traffic shifting started",
	})
}

// handleCompleteBlueGreenDeployment completes the blue-green deployment
func (s *Server) handleCompleteBlueGreenDeployment(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
	}

	if s.dependencies.BlueGreenManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Blue-Green deployment not available"})
	}

	// Complete deployment
	ctx := c.Context()
	if err := s.dependencies.BlueGreenManager.CompleteDeployment(ctx, appName); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to complete deployment: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Blue-green deployment completed successfully",
	})
}

// handleRollbackBlueGreenDeployment rolls back the blue-green deployment
func (s *Server) handleRollbackBlueGreenDeployment(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
	}

	if s.dependencies.BlueGreenManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Blue-Green deployment not available"})
	}

	// Rollback deployment
	ctx := c.Context()
	if err := s.dependencies.BlueGreenManager.RollbackDeployment(ctx, appName); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to rollback deployment: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Blue-green deployment rolled back successfully",
	})
}

// initializeTemplateHandler creates a new template handler
func initializeTemplateHandler() (*templates.Handler, error) {
	log.Printf("Initializing template management handler")
	handler, err := templates.NewHandler()
	if err != nil {
		return nil, fmt.Errorf("failed to create template handler: %w", err)
	}
	log.Printf("Template management handler initialized successfully")
	return handler, nil
}

// initializeOpenRewriteHandler initializes the OpenRewrite transformation handler
// func initializeOpenRewriteHandler(cfg *ControllerConfig) (*openrewrite.Handler, error) {
// 	log.Printf("Initializing OpenRewrite handler")
//
// 	// Create OpenRewrite configuration
// 	config := &internal_openrewrite.Config{
// 		WorkDir:          "/tmp/openrewrite",
// 		MavenPath:        "mvn",
// 		GradlePath:       "gradle",
// 		GitPath:          "git",
// 		MaxTransformTime: 5 * time.Minute,
// 	}
//
// 	// Set JAVA_HOME from environment if available
// 	if javaHome := os.Getenv("JAVA_HOME"); javaHome != "" {
// 		config.JavaHome = javaHome
// 	}
//
// 	// Create executor
// 	executor := internal_openrewrite.NewExecutor(config)
//
// 	// Create handler
// 	handler := openrewrite.NewHandlerWithConfig(executor, config)
//
// 	log.Printf("OpenRewrite handler initialized successfully")
// 	return handler, nil
// }

// loadCHTTPPrivateKey loads an RSA private key from a PEM file

// initializeAnalysisHandler initializes the static analysis handler
func initializeAnalysisHandler(cfg *ControllerConfig, arfHandler *arf.Handler, cfgService *cfgsvc.Service) (*analysis.Handler, error) {
	log.Printf("Initializing Static Analysis handler")

	// Create a logger for the analysis engine
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// Check analysis mode from environment (nomad, legacy, or disabled)
	analysisMode := utils.Getenv("PLOY_ANALYSIS_MODE", "nomad")
	log.Printf("Static analysis mode: %s", analysisMode)

	var engine *analysis.Engine

	if analysisMode == "nomad" {
		if cfgService == nil {
			log.Printf("Analysis nomad mode requested but config service unavailable; falling back to legacy mode")
			analysisMode = "legacy"
		} else {
			// Create Nomad‑based dispatcher using unified storage from config service
			st, err := cfgService.Get().CreateStorageClient()
			if err != nil {
				return nil, fmt.Errorf("failed to create storage for analysis: %w", err)
			}
			dispatcher, err := analysis.NewAnalysisDispatcherOrchestration(st)
			if err != nil {
				return nil, fmt.Errorf("failed to create analysis dispatcher: %w", err)
			}
			engine = analysis.NewEngineWithDispatcher(logger, dispatcher)
			log.Printf("Initialized Nomad-based analysis engine with unified storage")
		}
	}
	if analysisMode == "legacy" || engine == nil {
		// Create legacy engine with local analyzers
		engine = analysis.NewEngine(logger)

		// Register Java analyzer with Error Prone
		javaAnalyzer := javaanalyzer.NewErrorProneAnalyzer(logger)
		if err := engine.RegisterAnalyzer("java", javaAnalyzer); err != nil {
			return nil, fmt.Errorf("failed to register Java analyzer: %w", err)
		}

		// Register legacy Python analyzer
		pythonAnalyzer := pythonanalyzer.NewPylintAnalyzer(logger)
		if err := engine.RegisterAnalyzer("python", pythonAnalyzer); err != nil {
			return nil, fmt.Errorf("failed to register Python analyzer: %w", err)
		}
		log.Printf("Registered legacy local analyzers")

	} else if analysisMode == "disabled" {
		// Create minimal engine with no analyzers
		engine = analysis.NewEngine(logger)
		log.Printf("Analysis engine disabled - no analyzers registered")

	} else {
		return nil, fmt.Errorf("invalid analysis mode: %s (must be 'nomad', 'legacy', or 'disabled')", analysisMode)
	}

	// TODO: Register additional language analyzers as they are implemented
	// Go, JavaScript, C#, Rust, etc.

	// Create the handler
	handler := analysis.NewHandler(engine, arfHandler, logger)

	log.Printf("Static Analysis handler initialized with %d language analyzers (mode: %s)",
		len(engine.GetSupportedLanguages()), analysisMode)
	return handler, nil
}

// Test comment for self-update verification - Fri Aug 29 09:21:29 MSK 2025
