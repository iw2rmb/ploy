package server

import (
	"fmt"
	"log"
	"time"

	"github.com/iw2rmb/ploy/api/certificates"
	"github.com/iw2rmb/ploy/api/health"
	"github.com/iw2rmb/ploy/api/metrics"

	cfgsvc "github.com/iw2rmb/ploy/internal/config"

	arfcore "github.com/iw2rmb/ploy/internal/arf/core"
	tarfrecipes "github.com/iw2rmb/ploy/internal/arf/recipes"
)

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

	// Initialize LLM Handler
	llmHandler, err := initializeLLMHandler(cfgService)
	if err != nil {
		log.Printf("Warning: Failed to initialize LLM handler: %v", err)
	}

    // Initialize Mods Handler
	transflowHandler, err := initializeTransflowHandler(cfg, cfgService)
	if err != nil {
		log.Printf("Warning: Failed to initialize Transflow handler: %v", err)
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
		TransflowHandler:        transflowHandler,
		AnalysisHandler:         analysisHandler,
		LLMHandler:              llmHandler,
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
