package arf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
)

// LLMRecipeGenerator defines the interface for LLM-assisted recipe generation
type LLMRecipeGenerator interface {
	GenerateRecipe(ctx context.Context, request RecipeGenerationRequest) (*GeneratedRecipe, error)
	ValidateGenerated(ctx context.Context, recipe GeneratedRecipe) (*EvolutionValidationResult, error)
	OptimizeRecipe(ctx context.Context, recipe *models.Recipe, feedback TransformationFeedback) (*models.Recipe, error)
}

// RecipeGenerationRequest contains all context needed for LLM recipe generation
type RecipeGenerationRequest struct {
	ErrorContext     ErrorContext             `json:"error_context"`
	CodebaseContext  CodebaseContext          `json:"codebase_context"`
	SimilarPatterns  []TransformationPattern  `json:"similar_patterns"`
	Constraints      []RecipeConstraint       `json:"constraints"`
	TargetFramework  string                   `json:"target_framework"`
	Language         string                   `json:"language"`
	ASTParser        string                   `json:"ast_parser"`
}

// GeneratedRecipe represents an LLM-generated transformation recipe
type GeneratedRecipe struct {
	Recipe       *models.Recipe    `json:"recipe"`
	Confidence   float64           `json:"confidence"`
	Explanation  string            `json:"explanation"`
	LLMMetadata  LLMGenerationData `json:"llm_metadata"`
	Validation   ValidationResult  `json:"validation"`
}

// LLMGenerationData contains metadata about the LLM generation process
type LLMGenerationData struct {
	Model           string        `json:"model"`
	PromptTokens    int           `json:"prompt_tokens"`
	ResponseTokens  int           `json:"response_tokens"`
	Temperature     float64       `json:"temperature"`
	RequestTime     time.Time     `json:"request_time"`
	ProcessingTime  time.Duration `json:"processing_time"`
}

// ErrorContext provides details about the error that triggered recipe generation
type ErrorContext struct {
	ErrorType       string            `json:"error_type"`
	ErrorMessage    string            `json:"error_message"`
	SourceFile      string            `json:"source_file"`
	LineNumber      int               `json:"line_number"`
	StackTrace      []string          `json:"stack_trace"`
	CompilerOutput  string            `json:"compiler_output"`
	BuildLogs       string            `json:"build_logs"`
	RelatedErrors   []string          `json:"related_errors"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// CodebaseContext provides information about the codebase being transformed
type CodebaseContext struct {
	Language        string            `json:"language"`
	Framework       string            `json:"framework"`
	Version         string            `json:"version"`
	Dependencies    []string          `json:"dependencies"`
	BuildTool       string            `json:"build_tool"`
	ProjectStructure map[string]interface{} `json:"project_structure"`
	RecentChanges   []string          `json:"recent_changes"`
	TestSuite       TestSuiteInfo     `json:"test_suite"`
}

// TestSuiteInfo describes the test setup for validation
type TestSuiteInfo struct {
	Framework      string   `json:"framework"`
	TestFiles      []string `json:"test_files"`
	Coverage       float64  `json:"coverage"`
	PassingTests   int      `json:"passing_tests"`
	FailingTests   int      `json:"failing_tests"`
}

// TransformationPattern represents a previously successful transformation pattern
type TransformationPattern struct {
	Signature       string                 `json:"signature"`
	SuccessRate     float64                `json:"success_rate"`
	Language        string                 `json:"language"`
	Framework       string                 `json:"framework"`
	TransformationType string              `json:"transformation_type"`
	RecipeTemplate  map[string]interface{} `json:"recipe_template"`
	ContextFactors  []string               `json:"context_factors"`
}

// RecipeConstraint defines limitations or requirements for recipe generation
type RecipeConstraint struct {
	Type        string      `json:"type"`
	Value       interface{} `json:"value"`
	Description string      `json:"description"`
	Required    bool        `json:"required"`
}

// TransformationFeedback contains information about recipe performance
type TransformationFeedback struct {
	Success             bool                   `json:"success"`
	ErrorMessages       []string               `json:"error_messages"`
	CompilationResults  CompilationResult      `json:"compilation_results"`
	TestResults         TestResult             `json:"test_results"`
	PerformanceMetrics  LLMPerformanceMetrics  `json:"performance_metrics"`
	UserFeedback        map[string]interface{} `json:"user_feedback"`
}

// CompilationResult contains build/compilation information
type CompilationResult struct {
	Success         bool     `json:"success"`
	Errors          []string `json:"errors"`
	Warnings        []string `json:"warnings"`
	CompileTime     time.Duration `json:"compile_time"`
	ArtifactSize    int64    `json:"artifact_size"`
}

// TestResult contains test execution results
type TestResult struct {
	Success         bool      `json:"success"`
	PassedTests     int       `json:"passed_tests"`
	FailedTests     int       `json:"failed_tests"`
	Coverage        float64   `json:"coverage"`
	ExecutionTime   time.Duration `json:"execution_time"`
	FailureDetails  []string  `json:"failure_details"`
}

// LLMPerformanceMetrics contains runtime performance data for LLM operations
type LLMPerformanceMetrics struct {
	ExecutionTime    time.Duration `json:"execution_time"`
	MemoryUsage      int64         `json:"memory_usage"`
	CPUUtilization   float64       `json:"cpu_utilization"`
	ResourceCost     float64       `json:"resource_cost"`
}

// OpenAILLMGenerator implements LLM recipe generation using OpenAI API
type OpenAILLMGenerator struct {
	apiKey      string
	model       string
	temperature float64
	maxTokens   int
	timeout     time.Duration
	httpClient  *http.Client
	cache       LLMCache
}

// LLMCache defines interface for caching LLM responses
type LLMCache interface {
	Get(ctx context.Context, key string) (*GeneratedRecipe, bool)
	Put(ctx context.Context, key string, recipe *GeneratedRecipe, ttl time.Duration) error
	Clear(ctx context.Context) error
}

// NewOpenAILLMGenerator creates a new OpenAI-based LLM generator
func NewOpenAILLMGenerator() (*OpenAILLMGenerator, error) {
	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("LLM_API_KEY environment variable is required")
	}

	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "gpt-4"
	}

	temperatureStr := os.Getenv("LLM_TEMPERATURE")
	temperature := 0.1
	if temperatureStr != "" {
		if temp, err := strconv.ParseFloat(temperatureStr, 64); err == nil {
			temperature = temp
		}
	}

	maxTokensStr := os.Getenv("LLM_MAX_TOKENS")
	maxTokens := 2048
	if maxTokensStr != "" {
		if tokens, err := strconv.Atoi(maxTokensStr); err == nil {
			maxTokens = tokens
		}
	}

	return &OpenAILLMGenerator{
		apiKey:      apiKey,
		model:       model,
		temperature: temperature,
		maxTokens:   maxTokens,
		timeout:     30 * time.Second,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		cache: NewInMemoryLLMCache(),
	}, nil
}

// GenerateRecipe generates a transformation recipe using OpenAI API
func (g *OpenAILLMGenerator) GenerateRecipe(ctx context.Context, request RecipeGenerationRequest) (*GeneratedRecipe, error) {
	startTime := time.Now()

	// Check cache first
	cacheKey := g.generateCacheKey(request)
	if cached, found := g.cache.Get(ctx, cacheKey); found {
		return cached, nil
	}

	// Build prompt for recipe generation
	prompt := g.buildRecipeGenerationPrompt(request)

	// Make API request
	openAIRequest := OpenAIRequest{
		Model: g.model,
		Messages: []OpenAIMessage{
			{
				Role:    "system",
				Content: g.getSystemPrompt(request.Language),
			},
			{
				Role:    "user", 
				Content: prompt,
			},
		},
		Temperature: g.temperature,
		MaxTokens:   g.maxTokens,
	}

	response, err := g.callOpenAI(ctx, openAIRequest)
	if err != nil {
		return nil, fmt.Errorf("OpenAI API call failed: %w", err)
	}

	// Parse response into recipe
	generatedRecipe, err := g.parseOpenAIResponse(response, startTime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}

	// Cache the result
	if err := g.cache.Put(ctx, cacheKey, generatedRecipe, 24*time.Hour); err != nil {
		// Log error but don't fail the request
		fmt.Printf("Failed to cache LLM response: %v\n", err)
	}

	return generatedRecipe, nil
}

// ValidateGenerated validates a generated recipe through compilation and testing
func (g *OpenAILLMGenerator) ValidateGenerated(ctx context.Context, recipe GeneratedRecipe) (*EvolutionValidationResult, error) {
	result := &EvolutionValidationResult{
		Valid:           true,
		SafetyScore:     1.0,
		Warnings:        []string{},
		CriticalIssues:  []string{},
		TestResults:     []EvolutionValidationTest{},
		RecommendAction: ActionApprove,
	}

	// Syntax validation
	if err := g.validateRecipeSyntax(recipe.Recipe); err != nil {
		result.Valid = false
		result.CriticalIssues = append(result.CriticalIssues, fmt.Sprintf("Syntax error: %s", err.Error()))
		result.SafetyScore -= 0.3
	}

	// Semantic validation
	if err := g.validateRecipeSemantics(recipe.Recipe); err != nil {
		result.Valid = false
		result.CriticalIssues = append(result.CriticalIssues, fmt.Sprintf("Semantic error: %s", err.Error()))
		result.SafetyScore -= 0.4
	}

	// Confidence threshold check
	if recipe.Confidence < 0.7 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Low confidence: %.2f below threshold 0.70", recipe.Confidence))
		result.SafetyScore -= 0.1
	}

	return result, nil
}

// OptimizeRecipe improves a recipe based on feedback from previous executions
func (g *OpenAILLMGenerator) OptimizeRecipe(ctx context.Context, recipe *models.Recipe, feedback TransformationFeedback) (*models.Recipe, error) {
	if feedback.Success && feedback.TestResults.Success {
		// Recipe is working well, minimal optimization needed
		return recipe, nil
	}

	// Build optimization prompt based on feedback
	prompt := g.buildOptimizationPrompt(recipe, feedback)

	openAIRequest := OpenAIRequest{
		Model: g.model,
		Messages: []OpenAIMessage{
			{
				Role:    "system",
				Content: "You are an expert code transformation optimizer. Improve recipes based on execution feedback.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.1, // Lower temperature for optimization
		MaxTokens:   g.maxTokens,
	}

	response, err := g.callOpenAI(ctx, openAIRequest)
	if err != nil {
		return nil, fmt.Errorf("OpenAI optimization call failed: %w", err)
	}

	optimizedRecipe, err := g.parseOptimizedRecipe(response, recipe)
	if err != nil {
		return nil, fmt.Errorf("failed to parse optimized recipe: %w", err)
	}

	return optimizedRecipe, nil
}

// OpenAI API types
type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens"`
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIResponse struct {
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

type OpenAIChoice struct {
	Message OpenAIMessage `json:"message"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Helper methods for OpenAI integration

func (g *OpenAILLMGenerator) callOpenAI(ctx context.Context, request OpenAIRequest) (*OpenAIResponse, error) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API returned status %d", resp.StatusCode)
	}

	var response OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

func (g *OpenAILLMGenerator) generateCacheKey(request RecipeGenerationRequest) string {
	// Create a deterministic cache key from request content
	keyData := fmt.Sprintf("%s-%s-%s-%s",
		request.ErrorContext.ErrorType,
		request.CodebaseContext.Language,
		request.CodebaseContext.Framework,
		request.ErrorContext.ErrorMessage,
	)
	return fmt.Sprintf("llm-recipe-%x", []byte(keyData))
}

func (g *OpenAILLMGenerator) buildRecipeGenerationPrompt(request RecipeGenerationRequest) string {
	var prompt strings.Builder

	prompt.WriteString("Generate a code transformation recipe to fix the following issue:\n\n")

	// Error context
	prompt.WriteString(fmt.Sprintf("ERROR: %s\n", request.ErrorContext.ErrorMessage))
	prompt.WriteString(fmt.Sprintf("Error Type: %s\n", request.ErrorContext.ErrorType))
	if request.ErrorContext.SourceFile != "" {
		prompt.WriteString(fmt.Sprintf("File: %s (line %d)\n", request.ErrorContext.SourceFile, request.ErrorContext.LineNumber))
	}

	// Codebase context
	prompt.WriteString(fmt.Sprintf("\nCODEBASE CONTEXT:\n"))
	prompt.WriteString(fmt.Sprintf("Language: %s\n", request.CodebaseContext.Language))
	prompt.WriteString(fmt.Sprintf("Framework: %s (version %s)\n", request.CodebaseContext.Framework, request.CodebaseContext.Version))
	prompt.WriteString(fmt.Sprintf("Build Tool: %s\n", request.CodebaseContext.BuildTool))
	if len(request.CodebaseContext.Dependencies) > 0 {
		prompt.WriteString(fmt.Sprintf("Dependencies: %s\n", strings.Join(request.CodebaseContext.Dependencies, ", ")))
	}

	// Similar patterns
	if len(request.SimilarPatterns) > 0 {
		prompt.WriteString("\nSIMILAR SUCCESSFUL PATTERNS:\n")
		for _, pattern := range request.SimilarPatterns {
			prompt.WriteString(fmt.Sprintf("- %s (success rate: %.1f%%)\n", pattern.Signature, pattern.SuccessRate*100))
		}
	}

	// Constraints
	if len(request.Constraints) > 0 {
		prompt.WriteString("\nCONSTRAINTS:\n")
		for _, constraint := range request.Constraints {
			prompt.WriteString(fmt.Sprintf("- %s: %s\n", constraint.Type, constraint.Description))
		}
	}

	// Target framework
	if request.TargetFramework != "" {
		prompt.WriteString(fmt.Sprintf("\nTarget Framework: %s\n", request.TargetFramework))
	}

	// Instructions
	prompt.WriteString("\nPlease generate a transformation recipe in the following JSON format:\n")
	prompt.WriteString(`{
  "id": "generated-recipe-name",
  "name": "Human Readable Name",
  "description": "Description of what this recipe does",
  "language": "` + request.Language + `",
  "category": "cleanup|modernize|migration|security",
  "confidence": 0.0-1.0,
  "source": "transformation-class-or-command",
  "options": {},
  "tags": ["tag1", "tag2"],
  "explanation": "Detailed explanation of the transformation approach"
}`)

	return prompt.String()
}

func (g *OpenAILLMGenerator) getSystemPrompt(language string) string {
	return fmt.Sprintf(`You are an expert code transformation specialist for %s. 
Your task is to generate precise, safe, and effective transformation recipes that:
1. Fix the specific error described
2. Maintain code functionality and semantics
3. Follow %s best practices and conventions
4. Are compatible with the specified framework and dependencies
5. Can be executed automatically with high confidence

Generate only valid, executable transformation recipes. Focus on accuracy and safety over complexity.`, language, language)
}

func (g *OpenAILLMGenerator) parseOpenAIResponse(response *OpenAIResponse, startTime time.Time) (*GeneratedRecipe, error) {
	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no choices in OpenAI response")
	}

	content := response.Choices[0].Message.Content

	// Extract JSON from response (handle markdown code blocks)
	jsonStr := g.extractJSONFromResponse(content)

	var recipeData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &recipeData); err != nil {
		return nil, fmt.Errorf("failed to parse recipe JSON: %w", err)
	}

	// Build recipe from parsed data
	recipe := &models.Recipe{
		ID: g.getStringField(recipeData, "id"),
		Metadata: models.RecipeMetadata{
			Name:        g.getStringField(recipeData, "name"),
			Description: g.getStringField(recipeData, "description"),
			Version:     "1.0.0", // Default version for generated recipes
			Languages:   []string{g.getStringField(recipeData, "language")},
			Categories:  []string{g.getStringField(recipeData, "category")},
			Tags:        g.getStringSliceField(recipeData, "tags"),
		},
		Steps: []models.RecipeStep{}, // Will be populated based on source
	}

	confidence := g.getFloatField(recipeData, "confidence")
	explanation := g.getStringField(recipeData, "explanation")

	llmMetadata := LLMGenerationData{
		Model:           g.model,
		PromptTokens:    response.Usage.PromptTokens,
		ResponseTokens:  response.Usage.CompletionTokens,
		Temperature:     g.temperature,
		RequestTime:     startTime,
		ProcessingTime:  time.Since(startTime),
	}

	return &GeneratedRecipe{
		Recipe:      recipe,
		Confidence:  confidence,
		Explanation: explanation,
		LLMMetadata: llmMetadata,
		Validation:  ValidationResult{}, // Will be populated by validation
	}, nil
}

// Helper methods for parsing recipe fields
func (g *OpenAILLMGenerator) getStringField(data map[string]interface{}, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func (g *OpenAILLMGenerator) getFloatField(data map[string]interface{}, key string) float64 {
	if val, ok := data[key]; ok {
		if f, ok := val.(float64); ok {
			return f
		}
	}
	return 0.0
}

func (g *OpenAILLMGenerator) getStringSliceField(data map[string]interface{}, key string) []string {
	if val, ok := data[key]; ok {
		if slice, ok := val.([]interface{}); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
	}
	return []string{}
}

func (g *OpenAILLMGenerator) getMapField(data map[string]interface{}, key string) map[string]interface{} {
	if val, ok := data[key]; ok {
		if m, ok := val.(map[string]interface{}); ok {
			return m
		}
	}
	return make(map[string]interface{})
}

func (g *OpenAILLMGenerator) getStringMapField(data map[string]interface{}, key string) map[string]string {
	if val, ok := data[key]; ok {
		if m, ok := val.(map[string]interface{}); ok {
			result := make(map[string]string)
			for k, v := range m {
				if s, ok := v.(string); ok {
					result[k] = s
				}
			}
			return result
		}
	}
	return make(map[string]string)
}

func (g *OpenAILLMGenerator) extractJSONFromResponse(content string) string {
	// Handle markdown code blocks
	lines := strings.Split(content, "\n")
	var jsonLines []string
	inCodeBlock := false
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock || strings.HasPrefix(trimmed, "{") || strings.Contains(line, ":") {
			jsonLines = append(jsonLines, line)
		}
	}
	
	jsonStr := strings.Join(jsonLines, "\n")
	if jsonStr == "" {
		jsonStr = content // Fallback to original content
	}
	
	return jsonStr
}

func (g *OpenAILLMGenerator) buildOptimizationPrompt(recipe *models.Recipe, feedback TransformationFeedback) string {
	var prompt strings.Builder

	prompt.WriteString("Optimize the following transformation recipe based on execution feedback:\n\n")

	// Original recipe
	recipeJSON, _ := json.MarshalIndent(recipe, "", "  ")
	prompt.WriteString(fmt.Sprintf("ORIGINAL RECIPE:\n%s\n\n", string(recipeJSON)))

	// Feedback
	prompt.WriteString("EXECUTION FEEDBACK:\n")
	prompt.WriteString(fmt.Sprintf("Success: %t\n", feedback.Success))

	if !feedback.Success {
		prompt.WriteString("Errors:\n")
		for _, err := range feedback.ErrorMessages {
			prompt.WriteString(fmt.Sprintf("- %s\n", err))
		}
	}

	if !feedback.CompilationResults.Success {
		prompt.WriteString("\nCompilation Issues:\n")
		for _, err := range feedback.CompilationResults.Errors {
			prompt.WriteString(fmt.Sprintf("- %s\n", err))
		}
	}

	if !feedback.TestResults.Success {
		prompt.WriteString(fmt.Sprintf("\nTest Results: %d passed, %d failed\n", 
			feedback.TestResults.PassedTests, feedback.TestResults.FailedTests))
		for _, failure := range feedback.TestResults.FailureDetails {
			prompt.WriteString(fmt.Sprintf("- %s\n", failure))
		}
	}

	prompt.WriteString("\nProvide an optimized version of the recipe that addresses these issues.")
	return prompt.String()
}

func (g *OpenAILLMGenerator) parseOptimizedRecipe(response *OpenAIResponse, originalRecipe *models.Recipe) (*models.Recipe, error) {
	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no choices in optimization response")
	}

	content := response.Choices[0].Message.Content
	jsonStr := g.extractJSONFromResponse(content)

	var recipeData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &recipeData); err != nil {
		return nil, fmt.Errorf("failed to parse optimized recipe JSON: %w", err)
	}

	// Update original recipe with optimizations
	optimizedRecipe := originalRecipe
	
	if name := g.getStringField(recipeData, "name"); name != "" {
		optimizedRecipe.Metadata.Name = name
	}
	if desc := g.getStringField(recipeData, "description"); desc != "" {
		optimizedRecipe.Metadata.Description = desc
	}
	if version := g.getStringField(recipeData, "version"); version != "" {
		optimizedRecipe.Metadata.Version = version
	}
	if tags := g.getStringSliceField(recipeData, "tags"); len(tags) > 0 {
		optimizedRecipe.Metadata.Tags = tags
	}

	return optimizedRecipe, nil
}

func (g *OpenAILLMGenerator) validateRecipeSyntax(recipe *models.Recipe) error {
	// Basic syntax validation
	if recipe.ID == "" {
		return fmt.Errorf("recipe ID is required")
	}
	if recipe.Metadata.Name == "" {
		return fmt.Errorf("recipe name is required")
	}
	if len(recipe.Metadata.Languages) == 0 {
		return fmt.Errorf("recipe languages are required")
	}
	if len(recipe.Steps) == 0 {
		return fmt.Errorf("recipe must have at least one step")
	}

	return nil
}

func (g *OpenAILLMGenerator) validateRecipeSemantics(recipe *models.Recipe) error {
	// Semantic validation based on language and category
	if len(recipe.Metadata.Languages) == 0 {
		return fmt.Errorf("no languages specified for recipe")
	}
	
	// Check first language for validation
	switch recipe.Metadata.Languages[0] {
	case "java":
		return g.validateJavaRecipe(recipe)
	case "javascript", "typescript":
		return g.validateJavaScriptRecipe(recipe)
	case "python":
		return g.validatePythonRecipe(recipe)
	case "go":
		return g.validateGoRecipe(recipe)
	default:
		return fmt.Errorf("unsupported language for semantic validation: %s", recipe.Metadata.Languages[0])
	}
}

func (g *OpenAILLMGenerator) validateJavaRecipe(recipe *models.Recipe) error {
	// Java-specific semantic validation
	// Check if this is a migration recipe
	for _, cat := range recipe.Metadata.Categories {
		if cat == "migration" {
			// Ensure at least one OpenRewrite step exists
			hasOpenRewrite := false
			for _, step := range recipe.Steps {
				if step.Type == models.StepTypeOpenRewrite {
					hasOpenRewrite = true
					break
				}
			}
			if !hasOpenRewrite {
				return fmt.Errorf("Java migration recipes should have at least one OpenRewrite step")
			}
		}
	}
	return nil
}

func (g *OpenAILLMGenerator) validateJavaScriptRecipe(recipe *models.Recipe) error {
	// JavaScript-specific semantic validation
	return nil
}

func (g *OpenAILLMGenerator) validatePythonRecipe(recipe *models.Recipe) error {
	// Python-specific semantic validation
	return nil
}

func (g *OpenAILLMGenerator) validateGoRecipe(recipe *models.Recipe) error {
	// Go-specific semantic validation
	return nil
}

// InMemoryLLMCache provides a simple in-memory cache for LLM responses
type InMemoryLLMCache struct {
	cache map[string]*CachedRecipe
}

type CachedRecipe struct {
	Recipe    *GeneratedRecipe
	ExpiresAt time.Time
}

func NewInMemoryLLMCache() *InMemoryLLMCache {
	return &InMemoryLLMCache{
		cache: make(map[string]*CachedRecipe),
	}
}

func (c *InMemoryLLMCache) Get(ctx context.Context, key string) (*GeneratedRecipe, bool) {
	cached, exists := c.cache[key]
	if !exists || time.Now().After(cached.ExpiresAt) {
		delete(c.cache, key)
		return nil, false
	}
	return cached.Recipe, true
}

func (c *InMemoryLLMCache) Put(ctx context.Context, key string, recipe *GeneratedRecipe, ttl time.Duration) error {
	c.cache[key] = &CachedRecipe{
		Recipe:    recipe,
		ExpiresAt: time.Now().Add(ttl),
	}
	return nil
}

func (c *InMemoryLLMCache) Clear(ctx context.Context) error {
	c.cache = make(map[string]*CachedRecipe)
	return nil
}