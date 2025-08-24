package arf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// OllamaProvider implements LLM recipe generation using Ollama local models
type OllamaProvider struct {
	baseURL       string
	model         string
	temperature   float64
	httpClient    *http.Client
	streamingMode bool
	contextLength int
	timeout       time.Duration
}

// OllamaConfig contains configuration for Ollama provider
type OllamaConfig struct {
	BaseURL          string        `json:"base_url" yaml:"base_url"`
	Model            string        `json:"model" yaml:"model"`
	Temperature      float64       `json:"temperature" yaml:"temperature"`
	StreamingEnabled bool          `json:"streaming_enabled" yaml:"streaming_enabled"`
	ContextLength    int           `json:"context_length" yaml:"context_length"`
	Timeout          time.Duration `json:"timeout" yaml:"timeout"`
}

// NewOllamaProvider creates a new Ollama-based LLM provider
func NewOllamaProvider(config OllamaConfig) (*OllamaProvider, error) {
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434"
	}
	if config.Model == "" {
		return nil, fmt.Errorf("model is required for Ollama provider")
	}
	if config.Temperature == 0 {
		config.Temperature = 0.1
	}
	if config.ContextLength == 0 {
		config.ContextLength = 4096
	}
	if config.Timeout == 0 {
		config.Timeout = 120 * time.Second
	}

	return &OllamaProvider{
		baseURL:       config.BaseURL,
		model:         config.Model,
		temperature:   config.Temperature,
		httpClient:    &http.Client{Timeout: config.Timeout},
		streamingMode: config.StreamingEnabled,
		contextLength: config.ContextLength,
		timeout:       config.Timeout,
	}, nil
}

// GenerateRecipe generates a transformation recipe using Ollama
func (o *OllamaProvider) GenerateRecipe(ctx context.Context, request RecipeGenerationRequest) (*GeneratedRecipe, error) {
	startTime := time.Now()

	// Build the prompt for recipe generation
	prompt := o.buildRecipeGenerationPrompt(request)

	// Create Ollama request
	ollamaReq := OllamaGenerateRequest{
		Model:       o.model,
		Prompt:      prompt,
		Temperature: o.temperature,
		Stream:      false,
		Options: OllamaOptions{
			NumCtx:     o.contextLength,
			NumPredict: 2048,
			TopK:       40,
			TopP:       0.9,
		},
	}

	// Call Ollama API
	response, err := o.callOllama(ctx, "/api/generate", ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("Ollama API call failed: %w", err)
	}

	// Parse response
	generatedRecipe, err := o.parseOllamaResponse(response, startTime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	// Add request context to recipe
	generatedRecipe.Recipe.Language = request.Language
	generatedRecipe.Recipe.Category = "llm_generated"

	return generatedRecipe, nil
}

// ValidateGenerated validates a generated recipe
func (o *OllamaProvider) ValidateGenerated(ctx context.Context, recipe GeneratedRecipe) (*EvolutionValidationResult, error) {
	// Build validation prompt
	prompt := o.buildValidationPrompt(recipe)

	ollamaReq := OllamaGenerateRequest{
		Model:       o.model,
		Prompt:      prompt,
		Temperature: 0.0, // Use zero temperature for validation
		Stream:      false,
		Options: OllamaOptions{
			NumCtx:     o.contextLength,
			NumPredict: 500,
		},
	}

	response, err := o.callOllama(ctx, "/api/generate", ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("validation call failed: %w", err)
	}

	// Parse validation response
	var ollamaResp OllamaGenerateResponse
	if err := json.Unmarshal(response, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to parse validation response: %w", err)
	}

	// Extract validation result from response
	isValid := strings.Contains(strings.ToLower(ollamaResp.Response), "valid")
	safetyScore := 0.7 // Default safety score for Ollama

	return &EvolutionValidationResult{
		Valid:          isValid,
		SafetyScore:    safetyScore,
		Warnings:       o.extractValidationIssues(ollamaResp.Response),
		CriticalIssues: []string{},
		TestResults:    []EvolutionValidationTest{},
	}, nil
}

// OptimizeRecipe optimizes an existing recipe based on feedback
func (o *OllamaProvider) OptimizeRecipe(ctx context.Context, recipe Recipe, feedback TransformationFeedback) (*Recipe, error) {
	prompt := o.buildOptimizationPrompt(recipe, feedback)

	ollamaReq := OllamaGenerateRequest{
		Model:       o.model,
		Prompt:      prompt,
		Temperature: o.temperature,
		Stream:      false,
		Options: OllamaOptions{
			NumCtx:     o.contextLength,
			NumPredict: 2048,
		},
	}

	response, err := o.callOllama(ctx, "/api/generate", ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("optimization call failed: %w", err)
	}

	// Parse optimized recipe
	optimizedRecipe, err := o.parseOptimizedRecipe(response, recipe)
	if err != nil {
		return nil, fmt.Errorf("failed to parse optimized recipe: %w", err)
	}

	return optimizedRecipe, nil
}

// GetCapabilities returns the capabilities of the Ollama provider
func (o *OllamaProvider) GetCapabilities() ProviderCapabilities {
	return ProviderCapabilities{
		SupportsStreaming:  true,
		MaxContextLength:   o.contextLength,
		SupportedLanguages: []string{"java", "python", "go", "javascript", "typescript", "rust", "c++"},
		SupportsFineTuning: false,
		LocalExecution:     true,
		RequiresAPIKey:     false,
		SupportsBatching:   false,
		MaxTokensPerMinute: -1, // No rate limiting for local models
	}
}

// EstimateCost estimates the cost of using Ollama (always free for local models)
func (o *OllamaProvider) EstimateCost(tokens TokenUsage) CostEstimate {
	return CostEstimate{
		Provider:     "ollama",
		Model:        o.model,
		InputCost:    0,
		OutputCost:   0,
		TotalCost:    0,
		Currency:     "USD",
		CostPerToken: 0,
		FreeModel:    true,
	}
}

// StreamGenerate generates a recipe with streaming response
func (o *OllamaProvider) StreamGenerate(ctx context.Context, request RecipeGenerationRequest) (<-chan StreamResponse, error) {
	responseChan := make(chan StreamResponse, 100)

	go func() {
		defer close(responseChan)

		prompt := o.buildRecipeGenerationPrompt(request)
		
		ollamaReq := OllamaGenerateRequest{
			Model:       o.model,
			Prompt:      prompt,
			Temperature: o.temperature,
			Stream:      true,
			Options: OllamaOptions{
				NumCtx:     o.contextLength,
				NumPredict: 2048,
			},
		}

		// Stream response handling
		o.streamOllama(ctx, "/api/generate", ollamaReq, responseChan)
	}()

	return responseChan, nil
}

// Helper methods

func (o *OllamaProvider) callOllama(ctx context.Context, endpoint string, request interface{}) ([]byte, error) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (o *OllamaProvider) streamOllama(ctx context.Context, endpoint string, request interface{}, responseChan chan<- StreamResponse) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		responseChan <- StreamResponse{Error: fmt.Errorf("failed to marshal request: %w", err)}
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		responseChan <- StreamResponse{Error: fmt.Errorf("failed to create request: %w", err)}
		return
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		responseChan <- StreamResponse{Error: fmt.Errorf("HTTP request failed: %w", err)}
		return
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	for {
		var streamResp OllamaStreamResponse
		if err := decoder.Decode(&streamResp); err != nil {
			if err == io.EOF {
				break
			}
			responseChan <- StreamResponse{Error: err}
			return
		}

		responseChan <- StreamResponse{
			Content: streamResp.Response,
			Done:    streamResp.Done,
		}

		if streamResp.Done {
			break
		}
	}
}

func (o *OllamaProvider) buildRecipeGenerationPrompt(request RecipeGenerationRequest) string {
	var prompt strings.Builder

	prompt.WriteString("You are an expert code transformation specialist. ")
	prompt.WriteString("Generate a transformation recipe for the following issue:\n\n")
	
	prompt.WriteString(fmt.Sprintf("Language: %s\n", request.Language))
	prompt.WriteString(fmt.Sprintf("Framework: %s\n", request.TargetFramework))
	prompt.WriteString(fmt.Sprintf("Error: %s\n", request.ErrorContext.ErrorMessage))
	
	if len(request.ErrorContext.StackTrace) > 0 {
		prompt.WriteString("Stack trace:\n")
		for _, line := range request.ErrorContext.StackTrace[:min(10, len(request.ErrorContext.StackTrace))] {
			prompt.WriteString(fmt.Sprintf("  %s\n", line))
		}
	}

	prompt.WriteString("\nGenerate a JSON recipe with the following structure:\n")
	prompt.WriteString(`{
  "name": "recipe name",
  "description": "what this recipe does",
  "language": "target language",
  "pattern": "code pattern to match",
  "replacement": "replacement code",
  "preconditions": ["list of preconditions"],
  "postconditions": ["list of postconditions"],
  "confidence": 0.0 to 1.0
}`)

	return prompt.String()
}

func (o *OllamaProvider) buildValidationPrompt(recipe GeneratedRecipe) string {
	recipeJSON, _ := json.MarshalIndent(recipe.Recipe, "", "  ")
	
	return fmt.Sprintf(`Validate the following transformation recipe:

%s

Is this recipe syntactically and semantically valid for %s?
Will it correctly transform the code without introducing errors?
Respond with "VALID" or "INVALID" and explain any issues.`,
		string(recipeJSON), recipe.Recipe.Language)
}

func (o *OllamaProvider) buildOptimizationPrompt(recipe Recipe, feedback TransformationFeedback) string {
	recipeJSON, _ := json.MarshalIndent(recipe, "", "  ")
	
	return fmt.Sprintf(`Optimize the following transformation recipe based on feedback:

Recipe:
%s

Feedback:
Success: %v
Errors: %v
Compilation Status: %v
Test Results: %v

Generate an improved version of this recipe that addresses the feedback.
Return the optimized recipe in the same JSON format.`,
		string(recipeJSON), feedback.Success, feedback.ErrorMessages, 
		feedback.CompilationResults.Success, feedback.TestResults.PassedTests)
}

func (o *OllamaProvider) parseOllamaResponse(response []byte, startTime time.Time) (*GeneratedRecipe, error) {
	var ollamaResp OllamaGenerateResponse
	if err := json.Unmarshal(response, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Extract JSON from response
	jsonStr := o.extractJSONFromResponse(ollamaResp.Response)
	if jsonStr == "" {
		// Fallback: try to parse the entire response as JSON
		jsonStr = ollamaResp.Response
	}

	// Parse recipe from JSON
	var recipe Recipe
	if err := json.Unmarshal([]byte(jsonStr), &recipe); err != nil {
		// If JSON parsing fails, create a basic recipe from the response
		recipe = Recipe{
			ID:          fmt.Sprintf("ollama-%d", time.Now().Unix()),
			Name:        "Generated Recipe",
			Description: ollamaResp.Response,
			Confidence:  0.5,
		}
	}

	return &GeneratedRecipe{
		Recipe:     recipe,
		Confidence: recipe.Confidence,
		Explanation: "Generated by Ollama " + o.model,
		LLMMetadata: LLMGenerationData{
			Model:          o.model,
			PromptTokens:   ollamaResp.PromptEvalCount,
			ResponseTokens: ollamaResp.EvalCount,
			Temperature:    o.temperature,
			RequestTime:    startTime,
			ProcessingTime: time.Since(startTime),
		},
	}, nil
}

func (o *OllamaProvider) parseOptimizedRecipe(response []byte, originalRecipe Recipe) (*Recipe, error) {
	var ollamaResp OllamaGenerateResponse
	if err := json.Unmarshal(response, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Extract JSON from response
	jsonStr := o.extractJSONFromResponse(ollamaResp.Response)
	if jsonStr == "" {
		jsonStr = ollamaResp.Response
	}

	// Parse optimized recipe
	var optimized Recipe
	if err := json.Unmarshal([]byte(jsonStr), &optimized); err != nil {
		// Return original recipe if parsing fails
		return &originalRecipe, nil
	}

	// Preserve original ID if not set
	if optimized.ID == "" {
		optimized.ID = originalRecipe.ID
	}

	return &optimized, nil
}

func (o *OllamaProvider) extractJSONFromResponse(content string) string {
	// Look for JSON blocks in the response
	start := strings.Index(content, "{")
	if start == -1 {
		return ""
	}

	// Find the matching closing brace
	depth := 0
	inString := false
	escape := false
	
	for i := start; i < len(content); i++ {
		char := content[i]
		
		if escape {
			escape = false
			continue
		}
		
		if char == '\\' {
			escape = true
			continue
		}
		
		if char == '"' && !escape {
			inString = !inString
			continue
		}
		
		if !inString {
			if char == '{' {
				depth++
			} else if char == '}' {
				depth--
				if depth == 0 {
					return content[start : i+1]
				}
			}
		}
	}
	
	return ""
}

func (o *OllamaProvider) extractValidationIssues(response string) []string {
	var issues []string
	
	// Look for common issue patterns in the response
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "•") {
			issues = append(issues, strings.TrimSpace(line[1:]))
		}
	}
	
	return issues
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Ollama API types

type OllamaGenerateRequest struct {
	Model       string         `json:"model"`
	Prompt      string         `json:"prompt"`
	Temperature float64        `json:"temperature"`
	Stream      bool           `json:"stream"`
	Options     OllamaOptions  `json:"options"`
}

type OllamaOptions struct {
	NumCtx     int     `json:"num_ctx"`
	NumPredict int     `json:"num_predict"`
	TopK       int     `json:"top_k"`
	TopP       float64 `json:"top_p"`
}

type OllamaGenerateResponse struct {
	Response         string    `json:"response"`
	Done             bool      `json:"done"`
	Context          []int     `json:"context"`
	TotalDuration    int64     `json:"total_duration"`
	LoadDuration     int64     `json:"load_duration"`
	PromptEvalCount  int       `json:"prompt_eval_count"`
	EvalCount        int       `json:"eval_count"`
	EvalDuration     int64     `json:"eval_duration"`
}

type OllamaStreamResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Additional types for provider interface

type ProviderCapabilities struct {
	SupportsStreaming  bool     `json:"supports_streaming"`
	MaxContextLength   int      `json:"max_context_length"`
	SupportedLanguages []string `json:"supported_languages"`
	SupportsFineTuning bool     `json:"supports_fine_tuning"`
	LocalExecution     bool     `json:"local_execution"`
	RequiresAPIKey     bool     `json:"requires_api_key"`
	SupportsBatching   bool     `json:"supports_batching"`
	MaxTokensPerMinute int      `json:"max_tokens_per_minute"`
}

type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type CostEstimate struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	InputCost    float64 `json:"input_cost"`
	OutputCost   float64 `json:"output_cost"`
	TotalCost    float64 `json:"total_cost"`
	Currency     string  `json:"currency"`
	CostPerToken float64 `json:"cost_per_token"`
	FreeModel    bool    `json:"free_model"`
}

type StreamResponse struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
	Error   error  `json:"error,omitempty"`
}

// NewOllamaLLMGenerator creates a new Ollama LLM generator with default configuration
func NewOllamaLLMGenerator() (LLMRecipeGenerator, error) {
	// Load configuration from environment or use defaults
	config := OllamaConfig{
		BaseURL: os.Getenv("OLLAMA_BASE_URL"),
		Model:   os.Getenv("OLLAMA_MODEL"),
	}
	
	// Parse temperature if set
	if tempStr := os.Getenv("OLLAMA_TEMPERATURE"); tempStr != "" {
		if temp, err := strconv.ParseFloat(tempStr, 64); err == nil {
			config.Temperature = temp
		}
	}
	
	// Parse max tokens if set
	if tokensStr := os.Getenv("OLLAMA_MAX_TOKENS"); tokensStr != "" {
		if tokens, err := strconv.Atoi(tokensStr); err == nil {
			config.ContextLength = tokens
		}
	}
	
	return NewOllamaProvider(config)
}

// NewOllamaLLMGeneratorWithConfig creates a new Ollama LLM generator with explicit configuration
func NewOllamaLLMGeneratorWithConfig(model, baseURL string, temperature float64, contextLength int) (LLMRecipeGenerator, error) {
	config := OllamaConfig{
		BaseURL:       baseURL,
		Model:         model,
		Temperature:   temperature,
		ContextLength: contextLength,
	}
	
	// Set defaults if not provided
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434"
	}
	if config.Temperature == 0 {
		config.Temperature = 0.1
	}
	if config.ContextLength == 0 {
		config.ContextLength = 4096
	}
	
	return NewOllamaProvider(config)
}