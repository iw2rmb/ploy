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
	modsapi "github.com/iw2rmb/ploy/api/mods"
	nvdapi "github.com/iw2rmb/ploy/api/nvd"
	recipes "github.com/iw2rmb/ploy/api/recipes"
	"github.com/iw2rmb/ploy/api/sbom"
	"github.com/iw2rmb/ploy/api/templates"

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

	// Initialize recipe executor (OpenRewrite dispatcher removed; Mods handles ORW)
	engine := recipes.NewRecipeExecutor(recipeStorage, sandboxMgr)

	// Create ARF handler - RecipeRegistry only (no fallback)

	// Always create handler with RecipeRegistry when storage provider is available
	// Strictly SeaweedFS-backed RecipeRegistry (no in-memory fallback)
	handler := arf.NewHandlerWithStorage(
		engine,
		recipeStorage,
		recipeIndex,
		recipeValidator,
		sandboxMgr,
		nil, // No SeaweedFS provider required; registry optional
	)
	log.Printf("ARF handler initialized (no RecipeRegistry storage provider)")

	// Wire NVD CVE database into ARF security engine (configurable via ARF config)
	{
		nvdCfg := arfConfig.NVD
		if nvdCfg.Enabled {
			nvd := nvdapi.NewNVDDatabase()
			if nvdCfg.APIKey != "" {
				nvd.SetAPIKey(nvdCfg.APIKey)
			}
			if nvdCfg.BaseURL != "" {
				nvd.SetBaseURL(nvdCfg.BaseURL)
			}
			if nvdCfg.Timeout > 0 {
				nvd.SetHTTPTimeout(nvdCfg.Timeout)
			}
			handler.SetCVEDatabase(nvd)
			log.Printf("ARF security engine configured with NVD CVE database (enabled)")
		} else {
			log.Printf("ARF NVD CVE database disabled by configuration")
		}
	}

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

func initializeModsHandler(cfg *ControllerConfig, cfgService *cfgsvc.Service) (*modsapi.Handler, error) {
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

	// Create Mods handler
	handler := modsapi.NewHandler(gitProvider, storage, statusStore)
	log.Printf("Mods handler initialized successfully")
	return handler, nil
}

func initializeSBOMHandler(cfgService *cfgsvc.Service) (*sbom.Handler, error) {
	log.Printf("Initializing SBOM handler")
	var st internalStorage.Storage
	var err error
	if cfgService != nil {
		st, err = resolveStorageFromConfigService(cfgService)
		if err != nil {
			log.Printf("Warning: SBOM storage not available: %v", err)
		}
	}
	h := sbom.NewHandler(st)
	return h, nil
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
