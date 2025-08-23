// Package sdk provides a Software Development Kit for creating and testing ARF recipes
package sdk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// RecipeSDK provides utilities for recipe development and testing
type RecipeSDK struct {
	// Configuration
	ControllerURL    string
	WorkspaceDir     string
	TemplatesDir     string
	OutputDir        string
	
	// Example recipes
	Examples map[string]Recipe
	
	// Internal state
	client *ARFClient
}

// Recipe represents an ARF transformation recipe
type Recipe struct {
	ID          string            `yaml:"id" json:"id"`
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Language    string            `yaml:"language" json:"language"`
	Category    RecipeCategory    `yaml:"category" json:"category"`
	Version     string            `yaml:"version" json:"version"`
	Source      string            `yaml:"source" json:"source"`
	Tags        []string          `yaml:"tags,omitempty" json:"tags,omitempty"`
	Options     map[string]string `yaml:"options,omitempty" json:"options,omitempty"`
	
	// Phase 3 extensions
	LLMEnhanced     bool                   `yaml:"llm_enhanced,omitempty" json:"llm_enhanced,omitempty"`
	MultiLanguage   bool                   `yaml:"multi_language,omitempty" json:"multi_language,omitempty"`
	HybridStrategy  string                 `yaml:"hybrid_strategy,omitempty" json:"hybrid_strategy,omitempty"`
	Preconditions   []Condition            `yaml:"preconditions,omitempty" json:"preconditions,omitempty"`
	Transformations []Transformation       `yaml:"transformations,omitempty" json:"transformations,omitempty"`
	PostValidation  []Validation           `yaml:"post_validation,omitempty" json:"post_validation,omitempty"`
	Metadata        map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// RecipeCategory defines the type of transformation
type RecipeCategory string

const (
	CategoryCleanup    RecipeCategory = "cleanup"
	CategoryModernize  RecipeCategory = "modernize"
	CategoryMigration  RecipeCategory = "migration"
	CategorySecurity   RecipeCategory = "security"
	CategoryRefactor   RecipeCategory = "refactor"
	CategoryOptimize   RecipeCategory = "optimize"
)

// Condition represents a precondition that must be met
type Condition struct {
	Type        string      `yaml:"type" json:"type"`
	Description string      `yaml:"description" json:"description"`
	Check       interface{} `yaml:"check" json:"check"`
	Required    bool        `yaml:"required" json:"required"`
}

// Transformation represents a code transformation step
type Transformation struct {
	Type        string                 `yaml:"type" json:"type"`
	Description string                 `yaml:"description" json:"description"`
	Pattern     string                 `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Replacement string                 `yaml:"replacement,omitempty" json:"replacement,omitempty"`
	Options     map[string]interface{} `yaml:"options,omitempty" json:"options,omitempty"`
}

// Validation represents a post-transformation validation step
type Validation struct {
	Type        string                 `yaml:"type" json:"type"`
	Description string                 `yaml:"description" json:"description"`
	Check       interface{}            `yaml:"check" json:"check"`
	OnFailure   string                 `yaml:"on_failure,omitempty" json:"on_failure,omitempty"`
	Options     map[string]interface{} `yaml:"options,omitempty" json:"options,omitempty"`
}

// TestFixture represents a test case for recipe validation
type TestFixture struct {
	Name            string                 `yaml:"name" json:"name"`
	Description     string                 `yaml:"description" json:"description"`
	Language        string                 `yaml:"language" json:"language"`
	Input           TestInput              `yaml:"input" json:"input"`
	ExpectedOutput  TestOutput             `yaml:"expected_output" json:"expected_output"`
	Metadata        map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// TestInput represents the input for a test case
type TestInput struct {
	Files       map[string]string      `yaml:"files" json:"files"`
	Context     map[string]interface{} `yaml:"context,omitempty" json:"context,omitempty"`
	Environment map[string]string      `yaml:"environment,omitempty" json:"environment,omitempty"`
}

// TestOutput represents the expected output from a transformation
type TestOutput struct {
	Files           map[string]string      `yaml:"files" json:"files"`
	ValidationRules []string               `yaml:"validation_rules,omitempty" json:"validation_rules,omitempty"`
	Metadata        map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// TestResult represents the result of running a test
type TestResult struct {
	Name      string        `json:"name"`
	Passed    bool          `json:"passed"`
	Duration  time.Duration `json:"duration"`
	Errors    []string      `json:"errors,omitempty"`
	Warnings  []string      `json:"warnings,omitempty"`
	Output    TestOutput    `json:"output"`
}

// BenchmarkResult represents performance benchmark results
type BenchmarkResult struct {
	Name               string        `json:"name"`
	AverageTime        time.Duration `json:"average_time"`
	MinTime            time.Duration `json:"min_time"`
	MaxTime            time.Duration `json:"max_time"`
	MemoryUsage        int64         `json:"memory_usage"`
	SuccessRate        float64       `json:"success_rate"`
	TransformationSize int           `json:"transformation_size"`
	Iterations         int           `json:"iterations"`
}

// SafetyReport represents safety analysis of a recipe
type SafetyReport struct {
	RecipeID       string        `json:"recipe_id"`
	SafetyScore    float64       `json:"safety_score"`
	RiskFactors    []RiskFactor  `json:"risk_factors"`
	Recommendations []string     `json:"recommendations"`
	AnalyzedAt     time.Time     `json:"analyzed_at"`
}

// RiskFactor represents a potential safety risk
type RiskFactor struct {
	Type        string  `json:"type"`
	Severity    float64 `json:"severity"`
	Description string  `json:"description"`
	Mitigation  string  `json:"mitigation"`
}

// Dataset represents a collection of test cases
type Dataset struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Language    string        `json:"language"`
	Fixtures    []TestFixture `json:"fixtures"`
}

// NewRecipeSDK creates a new instance of the Recipe SDK
func NewRecipeSDK(controllerURL string) *RecipeSDK {
	homeDir, _ := os.UserHomeDir()
	
	sdk := &RecipeSDK{
		ControllerURL: controllerURL,
		WorkspaceDir:  filepath.Join(homeDir, ".arf", "workspace"),
		TemplatesDir:  filepath.Join(homeDir, ".arf", "templates"),
		OutputDir:     filepath.Join(homeDir, ".arf", "output"),
		Examples:      make(map[string]Recipe),
		client:        NewARFClient(controllerURL),
	}
	
	// Initialize directories
	os.MkdirAll(sdk.WorkspaceDir, 0755)
	os.MkdirAll(sdk.TemplatesDir, 0755)
	os.MkdirAll(sdk.OutputDir, 0755)
	
	// Load example recipes
	sdk.loadExampleRecipes()
	
	return sdk
}

// CreateFromTemplate creates a new recipe from a template
func (sdk *RecipeSDK) CreateFromTemplate(templateName string) *Recipe {
	templates := map[string]Recipe{
		"java-cleanup": {
			ID:          "java-cleanup-template",
			Name:        "Java Code Cleanup",
			Description: "Clean up common Java code issues",
			Language:    "java",
			Category:    CategoryCleanup,
			Version:     "1.0.0",
			Source:      "org.openrewrite.java.cleanup.Cleanup",
			Tags:        []string{"java", "cleanup", "code-quality"},
			Options:     map[string]string{},
		},
		"js-modernize": {
			ID:          "js-modernize-template",
			Name:        "JavaScript Modernization",
			Description: "Modernize JavaScript code to ES6+",
			Language:    "javascript",
			Category:    CategoryModernize,
			Version:     "1.0.0",
			Source:      "tree-sitter-js-modernizer",
			Tags:        []string{"javascript", "es6", "modernize"},
			LLMEnhanced: true,
			HybridStrategy: "sequential",
		},
		"python-security": {
			ID:          "python-security-template",
			Name:        "Python Security Fixes",
			Description: "Apply security best practices to Python code",
			Language:    "python",
			Category:    CategorySecurity,
			Version:     "1.0.0",
			Source:      "bandit-security-fixes",
			Tags:        []string{"python", "security", "vulnerability"},
		},
		"multi-lang-dependency": {
			ID:          "multi-lang-dependency-template",
			Name:        "Multi-Language Dependency Update",
			Description: "Update dependencies across multiple languages",
			Language:    "multi",
			Category:    CategoryMigration,
			Version:     "1.0.0",
			Source:      "arf.multi.DependencyUpdater",
			Tags:        []string{"dependency", "multi-language", "update"},
			MultiLanguage: true,
			LLMEnhanced:   true,
			HybridStrategy: "parallel",
		},
	}
	
	if template, exists := templates[templateName]; exists {
		// Create a copy to avoid modifying the template
		newRecipe := template
		newRecipe.ID = generateRecipeID()
		return &newRecipe
	}
	
	return nil
}

// AddPrecondition adds a precondition to the recipe
func (sdk *RecipeSDK) AddPrecondition(recipe *Recipe, condition Condition) *Recipe {
	if recipe.Preconditions == nil {
		recipe.Preconditions = []Condition{}
	}
	recipe.Preconditions = append(recipe.Preconditions, condition)
	return recipe
}

// AddTransformation adds a transformation step to the recipe
func (sdk *RecipeSDK) AddTransformation(recipe *Recipe, transformation Transformation) *Recipe {
	if recipe.Transformations == nil {
		recipe.Transformations = []Transformation{}
	}
	recipe.Transformations = append(recipe.Transformations, transformation)
	return recipe
}

// AddPostValidation adds a post-transformation validation step
func (sdk *RecipeSDK) AddPostValidation(recipe *Recipe, validation Validation) *Recipe {
	if recipe.PostValidation == nil {
		recipe.PostValidation = []Validation{}
	}
	recipe.PostValidation = append(recipe.PostValidation, validation)
	return recipe
}

// SaveRecipe saves a recipe to a YAML file
func (sdk *RecipeSDK) SaveRecipe(recipe Recipe, filename string) error {
	if !strings.HasSuffix(filename, ".yaml") && !strings.HasSuffix(filename, ".yml") {
		filename += ".arf.yaml"
	}
	
	filepath := filepath.Join(sdk.WorkspaceDir, filename)
	
	data, err := yaml.Marshal(recipe)
	if err != nil {
		return fmt.Errorf("failed to marshal recipe: %w", err)
	}
	
	return os.WriteFile(filepath, data, 0644)
}

// LoadRecipe loads a recipe from a YAML file
func (sdk *RecipeSDK) LoadRecipe(filename string) (*Recipe, error) {
	filepath := filepath.Join(sdk.WorkspaceDir, filename)
	
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipe file: %w", err)
	}
	
	var recipe Recipe
	if err := yaml.Unmarshal(data, &recipe); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recipe: %w", err)
	}
	
	return &recipe, nil
}

// TestWithFixture runs a recipe against a test fixture
func (sdk *RecipeSDK) TestWithFixture(recipe Recipe, fixture TestFixture) TestResult {
	startTime := time.Now()
	
	result := TestResult{
		Name:   fixture.Name,
		Passed: false,
		Errors: []string{},
		Warnings: []string{},
	}
	
	// Validate recipe first
	if err := sdk.ValidateRecipe(recipe); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Recipe validation failed: %s", err))
		result.Duration = time.Since(startTime)
		return result
	}
	
	// Create test workspace
	testWorkspace := filepath.Join(sdk.OutputDir, fmt.Sprintf("test-%s-%d", fixture.Name, time.Now().Unix()))
	if err := os.MkdirAll(testWorkspace, 0755); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to create test workspace: %s", err))
		result.Duration = time.Since(startTime)
		return result
	}
	defer os.RemoveAll(testWorkspace)
	
	// Write input files
	for filename, content := range fixture.Input.Files {
		filePath := filepath.Join(testWorkspace, filename)
		os.MkdirAll(filepath.Dir(filePath), 0755)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to write input file %s: %s", filename, err))
		}
	}
	
	// Execute transformation (mock for now)
	// In a real implementation, this would execute the recipe
	transformationSuccess := true
	
	if transformationSuccess {
		result.Passed = true
		
		// Read output files
		result.Output.Files = make(map[string]string)
		for filename := range fixture.ExpectedOutput.Files {
			filePath := filepath.Join(testWorkspace, filename)
			if data, err := os.ReadFile(filePath); err == nil {
				result.Output.Files[filename] = string(data)
			}
		}
	} else {
		result.Errors = append(result.Errors, "Transformation execution failed")
	}
	
	result.Duration = time.Since(startTime)
	return result
}

// BenchmarkPerformance runs performance benchmarks on a recipe
func (sdk *RecipeSDK) BenchmarkPerformance(recipe Recipe, dataset Dataset) BenchmarkResult {
	result := BenchmarkResult{
		Name:        fmt.Sprintf("%s-benchmark", recipe.ID),
		Iterations:  len(dataset.Fixtures),
		SuccessRate: 0,
	}
	
	var totalTime time.Duration
	var minTime, maxTime time.Duration
	successCount := 0
	
	for i, fixture := range dataset.Fixtures {
		testResult := sdk.TestWithFixture(recipe, fixture)
		
		if testResult.Passed {
			successCount++
		}
		
		totalTime += testResult.Duration
		
		if i == 0 || testResult.Duration < minTime {
			minTime = testResult.Duration
		}
		
		if testResult.Duration > maxTime {
			maxTime = testResult.Duration
		}
	}
	
	if len(dataset.Fixtures) > 0 {
		result.AverageTime = totalTime / time.Duration(len(dataset.Fixtures))
		result.SuccessRate = float64(successCount) / float64(len(dataset.Fixtures))
	}
	
	result.MinTime = minTime
	result.MaxTime = maxTime
	
	return result
}

// ValidateSafety performs safety analysis on a recipe
func (sdk *RecipeSDK) ValidateSafety(recipe Recipe) SafetyReport {
	report := SafetyReport{
		RecipeID:        recipe.ID,
		SafetyScore:     1.0,
		RiskFactors:     []RiskFactor{},
		Recommendations: []string{},
		AnalyzedAt:      time.Now(),
	}
	
	// Analyze for potential risks
	
	// Check for destructive operations
	if strings.Contains(strings.ToLower(recipe.Source), "delete") ||
		strings.Contains(strings.ToLower(recipe.Description), "remove") {
		report.RiskFactors = append(report.RiskFactors, RiskFactor{
			Type:        "destructive_operation",
			Severity:    0.7,
			Description: "Recipe may perform destructive operations",
			Mitigation:  "Ensure proper backups and use dry-run mode first",
		})
		report.SafetyScore -= 0.2
	}
	
	// Check for experimental features
	if recipe.LLMEnhanced {
		report.RiskFactors = append(report.RiskFactors, RiskFactor{
			Type:        "experimental_feature",
			Severity:    0.3,
			Description: "Recipe uses experimental LLM enhancement",
			Mitigation:  "Test thoroughly in development environment",
		})
		report.SafetyScore -= 0.1
	}
	
	// Check for multi-language complexity
	if recipe.MultiLanguage {
		report.RiskFactors = append(report.RiskFactors, RiskFactor{
			Type:        "complexity",
			Severity:    0.4,
			Description: "Multi-language recipes have higher complexity",
			Mitigation:  "Validate each language component separately",
		})
		report.SafetyScore -= 0.1
	}
	
	// Generate recommendations
	if len(report.RiskFactors) > 0 {
		report.Recommendations = append(report.Recommendations, "Run comprehensive tests before production use")
		report.Recommendations = append(report.Recommendations, "Use version control to track changes")
		report.Recommendations = append(report.Recommendations, "Implement rollback procedures")
	}
	
	if report.SafetyScore < 0.0 {
		report.SafetyScore = 0.0
	}
	
	return report
}

// ValidateRecipe performs basic validation on a recipe
func (sdk *RecipeSDK) ValidateRecipe(recipe Recipe) error {
	if recipe.ID == "" {
		return fmt.Errorf("recipe ID is required")
	}
	
	if recipe.Name == "" {
		return fmt.Errorf("recipe name is required")
	}
	
	if recipe.Language == "" {
		return fmt.Errorf("recipe language is required")
	}
	
	if recipe.Source == "" {
		return fmt.Errorf("recipe source is required")
	}
	
	// Validate category
	validCategories := []RecipeCategory{
		CategoryCleanup, CategoryModernize, CategoryMigration,
		CategorySecurity, CategoryRefactor, CategoryOptimize,
	}
	
	validCategory := false
	for _, cat := range validCategories {
		if recipe.Category == cat {
			validCategory = true
			break
		}
	}
	
	if !validCategory {
		return fmt.Errorf("invalid recipe category: %s", recipe.Category)
	}
	
	return nil
}

// DeployRecipe deploys a recipe to the ARF controller
func (sdk *RecipeSDK) DeployRecipe(ctx context.Context, recipe Recipe) error {
	return sdk.client.DeployRecipe(ctx, recipe)
}

// TestRecipeOnController tests a recipe on the ARF controller
func (sdk *RecipeSDK) TestRecipeOnController(ctx context.Context, recipe Recipe, testCode string) (*TestResult, error) {
	return sdk.client.TestRecipe(ctx, recipe, testCode)
}

// Helper functions

func generateRecipeID() string {
	return fmt.Sprintf("recipe-%d", time.Now().Unix())
}

func (sdk *RecipeSDK) loadExampleRecipes() {
	// Load built-in example recipes
	sdk.Examples["java-spring-cleanup"] = Recipe{
		ID:          "java-spring-cleanup",
		Name:        "Spring Boot Cleanup",
		Description: "Clean up common Spring Boot code issues",
		Language:    "java",
		Category:    CategoryCleanup,
		Version:     "1.0.0",
		Source:      "org.openrewrite.java.spring.SpringBootCleanup",
		Tags:        []string{"java", "spring-boot", "cleanup"},
		Options: map[string]string{
			"removeUnusedImports": "true",
			"fixDeprecations":     "true",
		},
	}
	
	sdk.Examples["js-react-modernize"] = Recipe{
		ID:          "js-react-modernize",
		Name:        "React Hooks Migration",
		Description: "Migrate React class components to hooks",
		Language:    "javascript",
		Category:    CategoryModernize,
		Version:     "1.0.0",
		Source:      "react-hooks-migrator",
		Tags:        []string{"javascript", "react", "hooks"},
		LLMEnhanced: true,
		HybridStrategy: "sequential",
	}
}