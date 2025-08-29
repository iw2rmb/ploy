package arf

import (
	"context"
	"database/sql"
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

	// Learning System Configuration
	LearningDBURL     string        `yaml:"learning_db_url"`
	PatternMinSamples int           `yaml:"pattern_min_samples"`
	PatternTimeWindow time.Duration `yaml:"pattern_time_window"`

	// Hybrid Pipeline Configuration
	ConfidenceThresholds ConfidenceThresholds `yaml:"confidence_thresholds"`
	EnhancementMode      EnhancementMode      `yaml:"enhancement_mode"`

	// Multi-Language Configuration
	TreeSitterPath string   `yaml:"tree_sitter_path"`
	SupportedLangs []string `yaml:"supported_languages"`

	// A/B Testing Configuration
	ABTestMinSamples   int     `yaml:"ab_test_min_samples"`
	ABTestConfidence   float64 `yaml:"ab_test_confidence"`
	ABTestTrafficSplit float64 `yaml:"ab_test_traffic_split"`
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

		ABTestMinSamples:   100,
		ABTestConfidence:   0.95,
		ABTestTrafficSplit: 0.5,
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
	if dbURL := os.Getenv("ARF_LEARNING_DB_URL"); dbURL != "" {
		config.LearningDBURL = dbURL
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

	// Initialize Learning System
	if config.LearningDBURL != "" {
		components.LearningSystem, err = initializeLearningSystem(config.LearningDBURL)
		if err != nil {
			// Log error but continue with nil learning system
			fmt.Printf("Warning: Failed to initialize learning system: %v\n", err)
		}
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

	// Initialize A/B Test Framework
	if components.LearningSystem != nil {
		components.ABTestFramework = NewDefaultABTestFramework(getDBFromLearningSystem(components.LearningSystem))
	}

	// Initialize Strategy Selector
	components.StrategySelector = NewDefaultStrategySelector()

	return &components, nil
}

// Phase3Components contains all initialized Phase 3 components
type Phase3Components struct {
	LLMGenerator     LLMRecipeGenerator
	LearningSystem   LearningSystem
	HybridPipeline   HybridPipeline
	MultiLangEngine  MultiLanguageEngine
	ABTestFramework  ABTestFramework
	StrategySelector StrategySelector
}

// initializeLearningSystem creates and initializes the learning system with database
func initializeLearningSystem(dbURL string) (LearningSystem, error) {
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to learning database: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping learning database: %w", err)
	}

	// Create PostgreSQL learning system
	return NewPostgreSQLLearningSystem()
}

// getDBFromLearningSystem extracts the database connection from learning system
func getDBFromLearningSystem(ls LearningSystem) *sql.DB {
	// Type assertion to get DB from PostgreSQL learning system
	if pgls, ok := ls.(*PostgreSQLLearningSystem); ok {
		return pgls.db
	}
	return nil
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
func CreateHandlerWithPhase3(executor *RecipeExecutor, catalog RecipeCatalog, sandboxMgr SandboxManager) (*Handler, error) {
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
		catalog,
		sandboxMgr,
		components.LLMGenerator,
		components.LearningSystem,
		components.HybridPipeline,
		components.MultiLangEngine,
		components.ABTestFramework,
		components.StrategySelector,
	), nil
}
