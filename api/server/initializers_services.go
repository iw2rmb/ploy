package server

import (
	"fmt"
	"log"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/iw2rmb/ploy/api/analysis"
	javaanalyzer "github.com/iw2rmb/ploy/api/analysis/analyzers/java"
	pythonanalyzer "github.com/iw2rmb/ploy/api/analysis/analyzers/python"
	"github.com/iw2rmb/ploy/api/arf"
	"github.com/iw2rmb/ploy/api/llms"
	"github.com/iw2rmb/ploy/api/templates"
	"github.com/iw2rmb/ploy/api/transflow"

	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/git/provider"
	"github.com/iw2rmb/ploy/internal/orchestration"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/utils"
)

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
		return nil, fmt.Errorf("failed to initialize ARF storage backend: %w", err)
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

	// SeaweedFS Master and Filer have different ports
	seaweedfsMaster := utils.Getenv("SEAWEEDFS_MASTER", "http://seaweedfs-filer.service.consul:9333")
	seaweedfsFiler := utils.Getenv("SEAWEEDFS_FILER", seaweedfsURL)

	seaweedConfig := internalStorage.SeaweedFSConfig{
		Master:      seaweedfsMaster,
		Filer:       seaweedfsFiler,
		Collection:  "ploy-recipes", // Use recipes collection for RecipeRegistry
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
		log.Printf("Creating OpenRewrite dispatcher with: nomad=%s, registry=%s, seaweedfs_master=%s, seaweedfs_filer=%s, api=%s",
			nomadAddr, registryURL, seaweedfsMaster, seaweedfsFiler, apiURL)

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
		log.Printf("Check SeaweedFS connectivity at: master=%s, filer=%s", seaweedfsMaster, seaweedfsFiler)
	}

	// Initialize recipe executor with optional dispatcher
	engine := arf.NewRecipeExecutor(recipeStorage, sandboxMgr, openRewriteDispatcher)
	if openRewriteDispatcher != nil {
		log.Printf("SUCCESS: Recipe executor initialized WITH OpenRewrite dispatcher for fallback execution")
	} else {
		log.Printf("WARNING: Recipe executor initialized WITHOUT OpenRewrite dispatcher")
		log.Printf("OpenRewrite recipes that are not in storage will fail")
	}

	// Create ARF handler - RecipeRegistry only (no fallback)
	var handler *arf.Handler

	// Require SeaweedFS storage for RecipeRegistry
	if storageProvider == nil {
		return nil, fmt.Errorf("SeaweedFS storage is required for RecipeRegistry - check SeaweedFS connectivity")
	}

	// Always create handler with RecipeRegistry when storage provider is available
	// Strictly SeaweedFS-backed RecipeRegistry (no in-memory fallback)
	handler = arf.NewHandlerWithStorage(
		engine,
		recipeStorage,
		recipeIndex,
		recipeValidator,
		sandboxMgr,
		storageProvider, // Pass the working SeaweedFS storage provider for RecipeRegistry
	)
	log.Printf("ARF handler initialized with RecipeRegistry backend (storageProvider: %T)", storageProvider)

	// ARF async transforms and healing were removed; no Consul store is configured here.

	log.Printf("ARF handler initialized successfully")
	return handler, nil
}

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

func initializeLLMHandler(cfgService *cfgsvc.Service) (*llms.Handler, error) {
	log.Printf("Initializing LLM model registry handler")

	// Resolve unified storage from config service
	if cfgService == nil {
		return nil, fmt.Errorf("config service is required for LLM handler")
	}

	storage, err := resolveStorageFromConfigService(cfgService)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve storage for LLM handler: %w", err)
	}

	// Create LLM handler
	handler := llms.NewHandler(storage)
	log.Printf("LLM model registry handler initialized successfully")
	return handler, nil
}

func initializeTransflowHandler(cfg *ControllerConfig, cfgService *cfgsvc.Service) (*transflow.Handler, error) {
    log.Printf("Initializing Mods handler")

	// Create GitLab provider
	gitProvider := provider.NewGitLabProvider()

	// Resolve unified storage from config service
	var storage internalStorage.Storage
	if cfgService != nil {
		var err error
		storage, err = resolveStorageFromConfigService(cfgService)
		if err != nil {
                log.Printf("Warning: Failed to resolve storage for Mods handler: %v", err)
		}
	}

	// Create status store (Consul KV)
	var statusStore orchestration.KV
	if cfg.ConsulAddr != "" {
		statusStore = orchestration.NewKV()
	}

	// Create transflow handler
	handler := transflow.NewHandler(gitProvider, storage, statusStore)
    log.Printf("Mods handler initialized successfully")
	return handler, nil
}

func initializeTemplateHandler() (*templates.Handler, error) {
	log.Printf("Initializing template management handler")
	handler, err := templates.NewHandler()
	if err != nil {
		return nil, fmt.Errorf("failed to create template handler: %w", err)
	}
	log.Printf("Template management handler initialized successfully")
	return handler, nil
}
