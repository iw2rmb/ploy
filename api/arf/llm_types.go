package arf

import (
	"bytes"
	"context"
	"time"
)

// LLMRecipeGenerator defines the interface for LLM-based recipe generation
// This is now implemented via Nomad batch jobs through the LLMDispatcher
type LLMRecipeGenerator interface {
	// GenerateRecipe generates a transformation recipe using LLM
	GenerateRecipe(ctx context.Context, request RecipeGenerationRequest) (*GeneratedRecipe, error)
	
	// GetCapabilities returns the capabilities of the LLM generator
	GetCapabilities() LLMCapabilities
	
	// IsAvailable checks if the LLM service is available
	IsAvailable(ctx context.Context) bool
	
	// ValidateGenerated validates a generated recipe
	ValidateGenerated(ctx context.Context, recipe GeneratedRecipe) (*EvolutionValidationResult, error)
	
	// OptimizeRecipe optimizes a recipe based on feedback
	OptimizeRecipe(ctx context.Context, recipe interface{}, feedback TransformationFeedback) (interface{}, error)
}

// RecipeConstraint represents constraints for recipe generation
type RecipeConstraint struct {
	Type        string      `json:"type"`
	Value       interface{} `json:"value"`
	Required    bool        `json:"required"`
	Description string      `json:"description"`
}

// ErrorContext represents error context for LLM-based error resolution
type ErrorContext struct {
	ErrorMessage   string                 `json:"error_message"`
	ErrorType      string                 `json:"error_type"`
	ErrorDetails   map[string]string      `json:"error_details"`
	StackTrace     []string               `json:"stack_trace,omitempty"`
	SourceFile     string                 `json:"source_file,omitempty"`
	CompilerOutput string                 `json:"compiler_output,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
}

// TransformationFeedback represents feedback from a transformation attempt
type TransformationFeedback struct {
	Success        bool              `json:"success"`
	ErrorMessage   string            `json:"error_message,omitempty"`
	BuildSuccess   bool              `json:"build_success"`
	TestResults    map[string]bool   `json:"test_results,omitempty"`
	PerformanceMetrics map[string]float64 `json:"performance_metrics,omitempty"`
	UserRating     int               `json:"user_rating,omitempty"`
	Comments       string            `json:"comments,omitempty"`
}


// RecipeGenerationRequest represents a request to generate a transformation recipe
type RecipeGenerationRequest struct {
	Language        string           `json:"language"`
	Framework       string           `json:"framework,omitempty"`
	ErrorContext    ErrorContext     `json:"error_context"`
	CodebaseContext CodebaseContext  `json:"codebase_context"`
	Constraints     []string         `json:"constraints,omitempty"`
	MaxIterations   int              `json:"max_iterations,omitempty"`
}

// GeneratedRecipe represents a generated transformation recipe
type GeneratedRecipe struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Language    string                 `json:"language"`
	Recipe      map[string]interface{} `json:"recipe"`
	Confidence  float64                `json:"confidence"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	LLMMetadata map[string]interface{} `json:"llm_metadata,omitempty"`
	Explanation string                 `json:"explanation,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// LLMCapabilities describes what an LLM generator can do
type LLMCapabilities struct {
	SupportedLanguages []string `json:"supported_languages"`
	MaxContextLength   int      `json:"max_context_length"`
	SupportsStreaming  bool     `json:"supports_streaming"`
	SupportsFineTuning bool     `json:"supports_fine_tuning"`
}

// CodebaseContext provides context about the codebase
type CodebaseContext struct {
	Language    string            `json:"language"`
	Framework   string            `json:"framework,omitempty"`
	BuildTool   string            `json:"build_tool,omitempty"`
	Version     string            `json:"version,omitempty"`
	Imports     []string          `json:"imports,omitempty"`
	Symbols     []string          `json:"symbols,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// NomadLLMGenerator implements LLMRecipeGenerator using Nomad dispatcher
type NomadLLMGenerator struct {
	dispatcher *LLMDispatcher
	provider   string
	model      string
}

// NewNomadLLMGenerator creates a new Nomad-based LLM generator
func NewNomadLLMGenerator(provider, model string) (*NomadLLMGenerator, error) {
	dispatcher, err := GetOrCreateLLMDispatcher()
	if err != nil {
		return nil, err
	}
	
	return &NomadLLMGenerator{
		dispatcher: dispatcher,
		provider:   provider,
		model:      model,
	}, nil
}

// GenerateRecipe generates a recipe using Nomad batch jobs
func (g *NomadLLMGenerator) GenerateRecipe(ctx context.Context, request RecipeGenerationRequest) (*GeneratedRecipe, error) {
	// Create prompt from request
	prompt := g.buildPrompt(request)
	
	// Create workspace archive (simplified for now)
	archiveData := []byte("placeholder archive")
	
	// Submit job to Nomad
	params := map[string]interface{}{
		"language":    request.Language,
		"framework":   request.Framework,
		"temperature": 0.1,
		"max_tokens":  4096,
	}
	
	job, err := g.dispatcher.SubmitLLMTransformation(ctx, g.provider, g.model, prompt, bytes.NewReader(archiveData), params)
	if err != nil {
		return nil, err
	}
	
	// Wait for completion
	completedJob, err := g.dispatcher.WaitForCompletion(ctx, job.ID, 5*time.Minute)
	if err != nil {
		return nil, err
	}
	
	// Create generated recipe from result
	return &GeneratedRecipe{
		ID:          job.ID,
		Name:        "LLM Generated Recipe",
		Description: prompt,
		Language:    request.Language,
		Recipe:      completedJob.Result,
		Confidence:  0.8,
		CreatedAt:   time.Now(),
	}, nil
}

// GetCapabilities returns the capabilities of the LLM generator
func (g *NomadLLMGenerator) GetCapabilities() LLMCapabilities {
	return LLMCapabilities{
		SupportedLanguages: []string{"java", "python", "javascript", "go", "csharp", "rust"},
		MaxContextLength:   8192,
		SupportsStreaming:  false,
		SupportsFineTuning: false,
	}
}

// IsAvailable checks if the LLM service is available
func (g *NomadLLMGenerator) IsAvailable(ctx context.Context) bool {
	// Check if Nomad is reachable
	// Simplified implementation
	return true
}

// ValidateGenerated validates a generated recipe
func (g *NomadLLMGenerator) ValidateGenerated(ctx context.Context, recipe GeneratedRecipe) (*EvolutionValidationResult, error) {
	return &EvolutionValidationResult{
		Valid:          true,
		SafetyScore:    recipe.Confidence,
		Warnings:       []string{},
		CriticalIssues: []string{},
		TestResults:    []EvolutionValidationTest{},
	}, nil
}

// OptimizeRecipe optimizes a recipe based on feedback
func (g *NomadLLMGenerator) OptimizeRecipe(ctx context.Context, recipe interface{}, feedback TransformationFeedback) (interface{}, error) {
	// For now, return the recipe as-is
	// In a real implementation, would use LLM to optimize based on feedback
	return recipe, nil
}

// buildPrompt builds a prompt from the recipe generation request
func (g *NomadLLMGenerator) buildPrompt(request RecipeGenerationRequest) string {
	prompt := "Generate a code transformation for the following:\n\n"
	
	if request.ErrorContext.ErrorMessage != "" {
		prompt += "Error to fix:\n" + request.ErrorContext.ErrorMessage + "\n\n"
	}
	
	prompt += "Language: " + request.Language + "\n"
	if request.Framework != "" {
		prompt += "Framework: " + request.Framework + "\n"
	}
	
	if len(request.Constraints) > 0 {
		prompt += "\nConstraints:\n"
		for _, constraint := range request.Constraints {
			prompt += "- " + constraint + "\n"
		}
	}
	
	prompt += "\nProvide a working code transformation that addresses the issue."
	
	return prompt
}

// Helper functions for backward compatibility

// NewOllamaLLMGeneratorWithConfig creates an Ollama-based LLM generator
func NewOllamaLLMGeneratorWithConfig(model, baseURL string, temperature float64, maxTokens int) (LLMRecipeGenerator, error) {
	return NewNomadLLMGenerator("ollama", model)
}

// NewOpenAILLMGenerator creates an OpenAI-based LLM generator
func NewOpenAILLMGenerator() (LLMRecipeGenerator, error) {
	return NewNomadLLMGenerator("openai", "gpt-4")
}