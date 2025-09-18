package server

import (
	"fmt"
	"log"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/iw2rmb/ploy/api/analysis"
	javaanalyzer "github.com/iw2rmb/ploy/api/analysis/analyzers/java"
	pythonanalyzer "github.com/iw2rmb/ploy/api/analysis/analyzers/python"
	"github.com/iw2rmb/ploy/api/llms"
	modsapi "github.com/iw2rmb/ploy/api/mods"
	nvdapi "github.com/iw2rmb/ploy/api/nvd"
	recipes "github.com/iw2rmb/ploy/api/recipes"
	"github.com/iw2rmb/ploy/api/sbom"
	"github.com/iw2rmb/ploy/api/security"
	"github.com/iw2rmb/ploy/api/templates"

	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/git/provider"
	"github.com/iw2rmb/ploy/internal/orchestration"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/utils"
)

func initializeSecurityHandlers(cfg *ControllerConfig, cfgService *cfgsvc.Service) (*security.Handler, *recipes.HTTPHandler, error) {
	log.Printf("Initializing security services")

	securityConfig := security.LoadConfigFromEnv()
	if err := securityConfig.Validate(); err != nil {
		return nil, nil, fmt.Errorf("security configuration validation failed: %w", err)
	}

	securityHandler := security.NewHandler()

	var (
		recipeRegistry *recipes.RecipeRegistry
		recipeStorage  recipes.RecipeStorage
		recipeIndex    recipes.RecipeIndexStore
		provider       internalStorage.StorageProvider
	)

	if cfgService != nil {
		storageClient, err := resolveStorageFromConfigService(cfgService)
		if err != nil {
			log.Printf("Warning: unable to resolve storage for recipe catalog: %v", err)
		} else {
			provider = internalStorage.NewProviderFromStorage(storageClient, "ploy-recipes")
			recipeRegistry = recipes.NewRecipeRegistry(provider)
			recipeStorage = recipes.NewRegistryStorageAdapter(recipeRegistry)
		}
	} else {
		log.Printf("Warning: config service unavailable; recipe catalog endpoints will be disabled")
	}

	recipesHandler := recipes.NewHTTPHandlerWithStorage(recipeStorage, recipeIndex, nil, provider, recipeRegistry)
	log.Printf("Recipe HTTP handler initialized (registry available: %t)", recipeRegistry != nil)

	nvdCfg := securityConfig.NVD
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
		securityHandler.SetCVEDatabase(nvd)
		log.Printf("Security engine configured with NVD CVE database (enabled)")
	} else {
		log.Printf("Security NVD CVE database disabled by configuration")
	}

	log.Printf("Security handler initialized successfully")
	return securityHandler, recipesHandler, nil
}

func initializeAnalysisHandler(cfg *ControllerConfig, cfgService *cfgsvc.Service) (*analysis.Handler, error) {
	log.Printf("Initializing Static Analysis handler")

	// Create a logger for the analysis engine
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// Check analysis mode from environment (nomad, legacy, or disabled)
	analysisModeRaw := utils.Getenv("PLOY_ANALYSIS_MODE", "nomad")
	analysisMode := strings.ToLower(strings.TrimSpace(analysisModeRaw))
	if analysisMode != "nomad" && analysisMode != "legacy" && analysisMode != "disabled" {
		log.Printf("Invalid analysis mode %q, defaulting to legacy", analysisModeRaw)
		analysisMode = "legacy"
	}
	log.Printf("Static analysis mode: %s", analysisMode)

	var engine *analysis.Engine

	switch analysisMode {
	case "nomad":
		if cfgService == nil {
			log.Printf("Analysis nomad mode requested but config service unavailable; falling back to legacy mode")
			analysisMode = "legacy"
		} else {
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
	case "disabled":
		engine = analysis.NewEngine(logger)
		log.Printf("Analysis engine disabled - no analyzers registered")
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

	}

	// TODO: Register additional language analyzers as they are implemented
	// Go, JavaScript, C#, Rust, etc.

	// Create the handler
	handler := analysis.NewHandler(engine, logger)

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
