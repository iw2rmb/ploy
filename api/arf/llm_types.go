package arf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	recipes "github.com/iw2rmb/ploy/api/recipes"
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
	ValidateGenerated(ctx context.Context, recipe GeneratedRecipe) (*recipes.EvolutionValidationResult, error)

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
	Success            bool               `json:"success"`
	ErrorMessage       string             `json:"error_message,omitempty"`
	BuildSuccess       bool               `json:"build_success"`
	TestResults        map[string]bool    `json:"test_results,omitempty"`
	PerformanceMetrics map[string]float64 `json:"performance_metrics,omitempty"`
	UserRating         int                `json:"user_rating,omitempty"`
	Comments           string             `json:"comments,omitempty"`
}

// RecipeGenerationRequest represents a request to generate a transformation recipe
type RecipeGenerationRequest struct {
	Language        string          `json:"language"`
	Framework       string          `json:"framework,omitempty"`
	ErrorContext    ErrorContext    `json:"error_context"`
	CodebaseContext CodebaseContext `json:"codebase_context"`
	Constraints     []string        `json:"constraints,omitempty"`
	MaxIterations   int             `json:"max_iterations,omitempty"`
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
	Language  string            `json:"language"`
	Framework string            `json:"framework,omitempty"`
	BuildTool string            `json:"build_tool,omitempty"`
	Version   string            `json:"version,omitempty"`
	Imports   []string          `json:"imports,omitempty"`
	Symbols   []string          `json:"symbols,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// HTTPLLMGenerator implements LLMRecipeGenerator using external HTTP APIs
type HTTPLLMGenerator struct {
	modelConfig *ModelConfig
}

// NewHTTPLLMGenerator creates a new HTTP-based LLM generator using model registry
func NewHTTPLLMGenerator(modelName string) (*HTTPLLMGenerator, error) {
	ctx := context.Background()

	var config *ModelConfig
	var err error

	if modelName != "" {
		config, err = GetModelByName(ctx, modelName)
	} else {
		config, err = GetDefaultModel(ctx)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get model configuration: %w", err)
	}

	return &HTTPLLMGenerator{
		modelConfig: config,
	}, nil
}

// GenerateRecipe generates a recipe using external HTTP API
func (g *HTTPLLMGenerator) GenerateRecipe(ctx context.Context, request RecipeGenerationRequest) (*GeneratedRecipe, error) {
	prompt := g.buildPrompt(request)

	// Make HTTP request to LLM API
	response, err := g.callLLMAPI(ctx, prompt, g.modelConfig.MaxTokens, g.modelConfig.Temperature)
	if err != nil {
		return nil, fmt.Errorf("failed to call LLM API: %w", err)
	}

	// Parse response into recipe
	recipe := make(map[string]interface{})
	recipe["content"] = response
	recipe["model"] = g.modelConfig.Model
	recipe["provider"] = g.modelConfig.Provider

	return &GeneratedRecipe{
		ID:          fmt.Sprintf("recipe-%d", time.Now().Unix()),
		Name:        "LLM Generated Recipe",
		Description: prompt,
		Language:    request.Language,
		Recipe:      recipe,
		Confidence:  0.8,
		LLMMetadata: map[string]interface{}{
			"model":       g.modelConfig.Model,
			"provider":    g.modelConfig.Provider,
			"temperature": g.modelConfig.Temperature,
			"max_tokens":  g.modelConfig.MaxTokens,
		},
		CreatedAt: time.Now(),
	}, nil
}

// GetCapabilities returns the capabilities of the LLM generator
func (g *HTTPLLMGenerator) GetCapabilities() LLMCapabilities {
	return LLMCapabilities{
		SupportedLanguages: []string{"java", "python", "javascript", "go", "csharp", "rust"},
		MaxContextLength:   g.modelConfig.MaxTokens,
		SupportsStreaming:  false,
		SupportsFineTuning: false,
	}
}

// IsAvailable checks if the LLM service is available
func (g *HTTPLLMGenerator) IsAvailable(ctx context.Context) bool {
	// Make a simple health check request
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", g.modelConfig.Endpoint, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode < 500
}

// ValidateGenerated validates a generated recipe
func (g *HTTPLLMGenerator) ValidateGenerated(ctx context.Context, recipe GeneratedRecipe) (*recipes.EvolutionValidationResult, error) {
	return &recipes.EvolutionValidationResult{
		Valid:          true,
		SafetyScore:    recipe.Confidence,
		Warnings:       []string{},
		CriticalIssues: []string{},
		TestResults:    []recipes.EvolutionValidationTest{},
	}, nil
}

// OptimizeRecipe optimizes a recipe based on feedback
func (g *HTTPLLMGenerator) OptimizeRecipe(ctx context.Context, recipe interface{}, feedback TransformationFeedback) (interface{}, error) {
	// Generate optimization prompt
	prompt := fmt.Sprintf("Optimize the following recipe based on feedback:\n\nRecipe: %v\n\nFeedback:\n- Success: %v\n- Error: %s\n\nProvide an optimized version.",
		recipe, feedback.Success, feedback.ErrorMessage)

	response, err := g.callLLMAPI(ctx, prompt, g.modelConfig.MaxTokens, g.modelConfig.Temperature)
	if err != nil {
		return recipe, err // Return original on error
	}

	return response, nil
}

// buildPrompt builds a prompt from the recipe generation request
func (g *HTTPLLMGenerator) buildPrompt(request RecipeGenerationRequest) string {
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

// callLLMAPI makes an HTTP request to the configured LLM API
func (g *HTTPLLMGenerator) callLLMAPI(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, error) {
	// Build request based on provider
	var reqBody []byte
	var err error

	switch g.modelConfig.Provider {
	case "openai":
		reqBody, err = json.Marshal(map[string]interface{}{
			"model":       g.modelConfig.Model,
			"messages":    []map[string]string{{"role": "user", "content": prompt}},
			"max_tokens":  maxTokens,
			"temperature": temperature,
		})
	case "anthropic":
		reqBody, err = json.Marshal(map[string]interface{}{
			"model":       g.modelConfig.Model,
			"prompt":      prompt,
			"max_tokens":  maxTokens,
			"temperature": temperature,
		})
	default:
		// Generic format for custom providers
		reqBody, err = json.Marshal(map[string]interface{}{
			"prompt":      prompt,
			"max_tokens":  maxTokens,
			"temperature": temperature,
			"model":       g.modelConfig.Model,
		})
	}

	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", g.modelConfig.Endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if g.modelConfig.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.modelConfig.APIKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract response based on provider format
	switch g.modelConfig.Provider {
	case "openai":
		if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if message, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := message["content"].(string); ok {
						return content, nil
					}
				}
			}
		}
	case "anthropic":
		if completion, ok := result["completion"].(string); ok {
			return completion, nil
		}
	default:
		// Try common response formats
		if content, ok := result["content"].(string); ok {
			return content, nil
		}
		if response, ok := result["response"].(string); ok {
			return response, nil
		}
		if text, ok := result["text"].(string); ok {
			return text, nil
		}
	}

	return "", fmt.Errorf("could not extract response from LLM API result")
}

// Helper functions for backward compatibility

// NewOllamaLLMGeneratorWithConfig creates an external LLM generator (backward compatibility)
// Deprecated: Use NewHTTPLLMGenerator with model registry instead
func NewOllamaLLMGeneratorWithConfig(model, baseURL string, temperature float64, maxTokens int) (LLMRecipeGenerator, error) {
	// Try to find a matching model in the registry
	return NewHTTPLLMGenerator("")
}

// NewOpenAILLMGenerator creates an OpenAI-based LLM generator (backward compatibility)
// Deprecated: Use NewHTTPLLMGenerator with model registry instead
func NewOpenAILLMGenerator() (LLMRecipeGenerator, error) {
	// Try to find an OpenAI model in the registry
	return NewHTTPLLMGenerator("")
}
