package arf

import (
	"context"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
)

// Phase3Config contains configuration for ARF Phase 3 components
type Phase3Config struct {
	// LLM Configuration
	LLMProvider    string  `yaml:"llm_provider"` // openai, anthropic, azure, ollama, cohere
	LLMAPIKey      string  `yaml:"llm_api_key"`
	LLMModel       string  `yaml:"llm_model"`
	LLMTemperature float64 `yaml:"llm_temperature"`
	LLMBaseURL     string  `yaml:"llm_base_url"` // For Ollama and custom endpoints

	PatternMinSamples int           `yaml:"pattern_min_samples"`
	PatternTimeWindow time.Duration `yaml:"pattern_time_window"`

	// Hybrid Pipeline Configuration
	ConfidenceThresholds ConfidenceThresholds `yaml:"confidence_thresholds"`
	EnhancementMode      EnhancementMode      `yaml:"enhancement_mode"`

	// Multi-Language Configuration
	TreeSitterPath string   `yaml:"tree_sitter_path"`
	SupportedLangs []string `yaml:"supported_languages"`
}

// DefaultPhase3Config returns default configuration for Phase 3
// Note: External model configuration is required - no defaults provided
func DefaultPhase3Config() *Phase3Config {
	return &Phase3Config{
		// LLM configuration must be provided via environment or API
		LLMProvider:    "", // Must be set explicitly
		LLMModel:       "", // Must be set explicitly
		LLMTemperature: 0.1,

		PatternMinSamples: 10,
		PatternTimeWindow: 30 * 24 * time.Hour, // 30 days

		ConfidenceThresholds: ConfidenceThresholds{
			MinOpenRewrite: 0.6,
			MinLLM:         0.7,
			MinHybrid:      0.8,
			RequiredBuild:  0.9,
		},

		EnhancementMode: PostProcessing,

		TreeSitterPath: "/usr/local/bin/tree-sitter",
		SupportedLangs: []string{"java", "javascript", "python", "go", "rust"},
	}
}

// LoadPhase3ConfigFromEnv loads configuration from environment variables
func LoadPhase3ConfigFromEnv() *Phase3Config {
	config := DefaultPhase3Config()

	// Load from environment
	if provider := os.Getenv("ARF_LLM_PROVIDER"); provider != "" {
		config.LLMProvider = provider
	}
	if apiKey := os.Getenv("ARF_LLM_API_KEY"); apiKey != "" {
		config.LLMAPIKey = apiKey
	}
	if model := os.Getenv("ARF_LLM_MODEL"); model != "" {
		config.LLMModel = model
	}
	if treeSitter := os.Getenv("ARF_TREE_SITTER_PATH"); treeSitter != "" {
		config.TreeSitterPath = treeSitter
	}

	// OpenRewrite always uses batch job dispatcher - no configuration needed

	return config
}

// InitializePhase3Components initializes all ARF Phase 3 components
func InitializePhase3Components(config *Phase3Config) (*Phase3Components, error) {
	var components Phase3Components
	var err error

	// Initialize LLM Generator - external model required
	if config.LLMAPIKey == "" || config.LLMProvider == "" {
		return nil, fmt.Errorf("external LLM configuration required: provider and API key must be set")
	}

	switch config.LLMProvider {
	case "openai":
		llmGen, err := NewOpenAILLMGenerator()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize OpenAI LLM: %w", err)
		}
		components.LLMGenerator = llmGen
	case "anthropic", "azure", "cohere":
		// Use HTTP LLM generator for other external providers
		llmGen, err := NewHTTPLLMGenerator("")
		if err != nil {
			return nil, fmt.Errorf("failed to initialize %s LLM: %w", config.LLMProvider, err)
		}
		components.LLMGenerator = llmGen
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s (only external models supported)", config.LLMProvider)
	}

	// Initialize Multi-Language Engine
	multiLang, err := NewTreeSitterMultiLanguageEngine()
	if err != nil {
		fmt.Printf("Warning: Failed to initialize multi-language engine: %v\n", err)
		// Create a basic multi-language engine
		multiLang = &TreeSitterMultiLanguageEngine{}
	}
	components.MultiLangEngine = multiLang

	// Initialize Hybrid Pipeline
	// Note: We need a proper RecipeExecutor, using nil for now
	components.HybridPipeline = NewDefaultHybridPipeline(
		nil, // RecipeExecutor - needs to be initialized separately
		components.LLMGenerator,
		components.MultiLangEngine,
	)

	// Initialize Strategy Selector
	components.StrategySelector = NewDefaultStrategySelector()

	return &components, nil
}

// Phase3Components contains all initialized Phase 3 components
type Phase3Components struct {
	LLMGenerator     LLMRecipeGenerator
	HybridPipeline   HybridPipeline
	MultiLangEngine  MultiLanguageEngine
	StrategySelector StrategySelector
}

// Note: Mock LLM generator removed - only external models are supported

// mockLLMGenerator is a mock implementation for testing
type mockLLMGenerator struct {
	generateRecipeFn  func(context.Context, RecipeGenerationRequest) (*GeneratedRecipe, error)
	getCapabilitiesFn func() LLMCapabilities
	isAvailableFn     func(context.Context) bool
	validateFn        func(context.Context, GeneratedRecipe) (*EvolutionValidationResult, error)
	optimizeFn        func(context.Context, interface{}, TransformationFeedback) (interface{}, error)
}

func (m *mockLLMGenerator) GenerateRecipe(ctx context.Context, request RecipeGenerationRequest) (*GeneratedRecipe, error) {
	return &GeneratedRecipe{
		ID:          fmt.Sprintf("mock-%d", time.Now().Unix()),
		Name:        "Mock Generated Recipe",
		Description: "This is a mock generated recipe",
		Language:    request.Language,
		Recipe:      map[string]interface{}{"mock": true},
		Confidence:  0.5,
		CreatedAt:   time.Now(),
	}, nil
}

func (m *mockLLMGenerator) GetCapabilities() LLMCapabilities {
	return LLMCapabilities{
		SupportedLanguages: []string{"java", "python", "javascript", "go"},
		MaxContextLength:   8192,
		SupportsStreaming:  false,
		SupportsFineTuning: false,
	}
}

func (m *mockLLMGenerator) IsAvailable(ctx context.Context) bool {
	return true
}

func (m *mockLLMGenerator) ValidateGenerated(ctx context.Context, recipe GeneratedRecipe) (*EvolutionValidationResult, error) {
	return &EvolutionValidationResult{
		Valid:          true,
		SafetyScore:    1.0,
		Warnings:       []string{},
		CriticalIssues: []string{},
		TestResults:    []EvolutionValidationTest{},
	}, nil
}

func (m *mockLLMGenerator) OptimizeRecipe(ctx context.Context, recipe interface{}, feedback TransformationFeedback) (interface{}, error) {
	// For mock, just return the recipe as-is
	return recipe, nil
}

// CreateOpenRewriteEngine creates OpenRewrite engine using batch job dispatcher
func CreateOpenRewriteEngine(config *Phase3Config) interface{} {
	// Always use embedded mode with batch job dispatcher
	// Service mode has been deprecated in favor of batch jobs
	return NewOpenRewriteEngine()
}

// CreateHandlerWithPhase3 creates a handler with all Phase 3 components initialized
func CreateHandlerWithPhase3(executor *RecipeExecutor, sandboxMgr SandboxManager) (*Handler, error) {
	// Load configuration
	config := LoadPhase3ConfigFromEnv()

	// Initialize Phase 3 components
	components, err := InitializePhase3Components(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Phase 3 components: %w", err)
	}

	// Create handler with Phase 3 components
	return NewHandlerWithPhase3(
		executor,
		sandboxMgr,
		components.LLMGenerator,
		components.HybridPipeline,
		components.MultiLangEngine,
		components.StrategySelector,
	), nil
}
