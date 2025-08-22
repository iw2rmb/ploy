package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// OpenRewriteEngine implements ARFEngine using OpenRewrite Java libraries
type OpenRewriteEngine struct {
	jarPath       string
	javaHome      string
	workingDir    string
	cache         ASTCache
	sandboxMgr    SandboxManager
	recipes       map[string]Recipe
	maxMemory     string
	timeout       time.Duration
}

// NewOpenRewriteEngine creates a new OpenRewrite-based ARF engine
func NewOpenRewriteEngine(jarPath, javaHome, workingDir string, cache ASTCache, sandboxMgr SandboxManager) *OpenRewriteEngine {
	return &OpenRewriteEngine{
		jarPath:    jarPath,
		javaHome:   javaHome,
		workingDir: workingDir,
		cache:      cache,
		sandboxMgr: sandboxMgr,
		recipes:    make(map[string]Recipe),
		maxMemory:  "4G",
		timeout:    10 * time.Minute,
	}
}

// ExecuteRecipe executes an OpenRewrite recipe on the given codebase
func (e *OpenRewriteEngine) ExecuteRecipe(ctx context.Context, recipe Recipe, codebase Codebase) (*TransformationResult, error) {
	startTime := time.Now()

	// Create sandbox for secure execution
	sandboxConfig := SandboxConfig{
		Repository:    codebase.Repository,
		Branch:        codebase.Branch,
		Language:      codebase.Language,
		BuildTool:     codebase.BuildTool,
		TTL:           30 * time.Minute,
		MemoryLimit:   "4G",
		CPULimit:      "2",
		NetworkAccess: false,
		TempSpace:     "2G",
	}

	sandbox, err := e.sandboxMgr.CreateSandbox(ctx, sandboxConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox: %w", err)
	}
	defer e.sandboxMgr.DestroySandbox(ctx, sandbox.ID)

	// Prepare OpenRewrite execution environment
	workspaceDir := filepath.Join(sandbox.RootPath, "workspace")
	configPath := filepath.Join(workspaceDir, "rewrite.yml")
	outputPath := filepath.Join(workspaceDir, "rewrite-results.json")

	// Generate OpenRewrite configuration
	config := e.generateOpenRewriteConfig(recipe, codebase)
	if err := e.writeConfigFile(configPath, config); err != nil {
		return nil, fmt.Errorf("failed to write OpenRewrite config: %w", err)
	}

	// Execute OpenRewrite in sandbox
	_, err = e.executeInSandbox(ctx, sandbox, configPath, outputPath, workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("OpenRewrite execution failed: %w", err)
	}

	// Parse results
	transformResult, err := e.parseResults(outputPath, startTime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenRewrite results: %w", err)
	}

	transformResult.RecipeID = recipe.ID
	return transformResult, nil
}

// ValidateRecipe validates that a recipe is properly configured
func (e *OpenRewriteEngine) ValidateRecipe(recipe Recipe) error {
	if recipe.ID == "" {
		return fmt.Errorf("recipe ID is required")
	}
	if recipe.Source == "" {
		return fmt.Errorf("recipe source (OpenRewrite class) is required")
	}
	if recipe.Language == "" {
		return fmt.Errorf("recipe language is required")
	}

	// Validate OpenRewrite class name format
	if !strings.Contains(recipe.Source, ".") {
		return fmt.Errorf("invalid OpenRewrite class name: %s", recipe.Source)
	}

	// Check for required options based on recipe type
	switch recipe.Category {
	case CategoryMigration:
		if _, hasFrom := recipe.Options["fromVersion"]; !hasFrom {
			return fmt.Errorf("migration recipes require 'fromVersion' option")
		}
		if _, hasTo := recipe.Options["toVersion"]; !hasTo {
			return fmt.Errorf("migration recipes require 'toVersion' option")
		}
	case CategorySecurity:
		// Security recipes may have specific validation requirements
		if recipe.Confidence < 0.8 {
			return fmt.Errorf("security recipes must have confidence >= 0.8")
		}
	}

	return nil
}

// ListAvailableRecipes returns all available OpenRewrite recipes
func (e *OpenRewriteEngine) ListAvailableRecipes() ([]Recipe, error) {
	// In a full implementation, this would query the OpenRewrite JAR for available recipes
	// For now, return predefined recipes
	recipes := []Recipe{
		{
			ID:          "cleanup.unused-imports",
			Name:        "Remove Unused Imports",
			Description: "Removes unused import statements",
			Language:    "java",
			Category:    CategoryCleanup,
			Confidence:  0.95,
			Source:      "org.openrewrite.java.cleanup.RemoveUnusedImports",
			Version:     "1.0.0",
			Tags:        []string{"cleanup", "imports"},
		},
		{
			ID:          "modernize.junit4-to-junit5",
			Name:        "Migrate JUnit 4 to JUnit 5",
			Description: "Migrates JUnit 4 tests to JUnit 5",
			Language:    "java",
			Category:    CategoryModernize,
			Confidence:  0.85,
			Source:      "org.openrewrite.java.testing.junit5.JUnit4to5Migration",
			Version:     "1.0.0",
			Tags:        []string{"testing", "junit", "migration"},
		},
		{
			ID:          "security.fix-deprecated-apis",
			Name:        "Fix Deprecated Security APIs",
			Description: "Replaces deprecated security APIs with modern alternatives",
			Language:    "java",
			Category:    CategorySecurity,
			Confidence:  0.90,
			Source:      "org.openrewrite.java.security.FixDeprecatedApis",
			Version:     "1.0.0",
			Tags:        []string{"security", "deprecated"},
		},
	}

	var result []Recipe
	for _, recipe := range recipes {
		e.recipes[recipe.ID] = recipe
		result = append(result, recipe)
	}

	return result, nil
}

// GetRecipeMetadata returns detailed metadata for a recipe
func (e *OpenRewriteEngine) GetRecipeMetadata(recipeID string) (*RecipeMetadata, error) {
	recipe, exists := e.recipes[recipeID]
	if !exists {
		return nil, fmt.Errorf("recipe %s not found", recipeID)
	}

	metadata := &RecipeMetadata{
		Recipe:              recipe,
		ApplicableLanguages: []string{recipe.Language},
		RequiredOptions:     []string{},
		OptionalOptions:     []string{},
		Prerequisites:       []string{},
		CreatedAt:           time.Now().Add(-30 * 24 * time.Hour), // Placeholder
		UpdatedAt:           time.Now().Add(-7 * 24 * time.Hour),  // Placeholder
		UsageCount:          0,
		SuccessRate:         recipe.Confidence,
	}

	// Set required/optional options based on recipe type
	switch recipe.Category {
	case CategoryMigration:
		metadata.RequiredOptions = []string{"fromVersion", "toVersion"}
	case CategoryModernize:
		metadata.OptionalOptions = []string{"preserveComments", "updateTests"}
	}

	return metadata, nil
}

// CacheAST stores an AST in the cache
func (e *OpenRewriteEngine) CacheAST(key string, ast *AST) error {
	return e.cache.Put(key, ast)
}

// GetCachedAST retrieves an AST from the cache
func (e *OpenRewriteEngine) GetCachedAST(key string) (*AST, bool) {
	return e.cache.Get(key)
}

// Helper methods

func (e *OpenRewriteEngine) generateOpenRewriteConfig(recipe Recipe, codebase Codebase) map[string]interface{} {
	config := map[string]interface{}{
		"type": "specs.openrewrite.org/v1beta/recipe",
		"name": recipe.ID,
		"displayName": recipe.Name,
		"description": recipe.Description,
		"recipeList": []map[string]interface{}{
			{
				"org.openrewrite.Recipe": recipe.Source,
			},
		},
	}

	// Add recipe-specific options
	if len(recipe.Options) > 0 {
		recipeConfig := config["recipeList"].([]map[string]interface{})[0]
		for key, value := range recipe.Options {
			recipeConfig[key] = value
		}
	}

	return config
}

func (e *OpenRewriteEngine) writeConfigFile(configPath string, config map[string]interface{}) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

func (e *OpenRewriteEngine) executeInSandbox(ctx context.Context, sandbox *Sandbox, configPath, outputPath, workspaceDir string) (*exec.Cmd, error) {
	// Construct java command to run OpenRewrite
	javaCmd := filepath.Join(e.javaHome, "bin", "java")
	args := []string{
		"-Xmx" + e.maxMemory,
		"-jar", e.jarPath,
		"--config", configPath,
		"--source", workspaceDir,
		"--output-format", "json",
		"--output", outputPath,
	}

	// Execute within jail context
	jailCmd := exec.CommandContext(ctx, "jexec", sandbox.JailName, javaCmd)
	jailCmd.Args = append(jailCmd.Args, args...)
	jailCmd.Dir = workspaceDir

	// Set timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	jailCmd = exec.CommandContext(timeoutCtx, jailCmd.Path, jailCmd.Args[1:]...)

	return jailCmd, jailCmd.Run()
}

func (e *OpenRewriteEngine) parseResults(outputPath string, startTime time.Time) (*TransformationResult, error) {
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read results file: %w", err)
	}

	var rawResults map[string]interface{}
	if err := json.Unmarshal(data, &rawResults); err != nil {
		return nil, fmt.Errorf("failed to parse results JSON: %w", err)
	}

	result := &TransformationResult{
		Success:       true,
		ExecutionTime: time.Since(startTime),
		Metadata:      rawResults,
	}

	// Extract changes information
	if changes, ok := rawResults["changes"].([]interface{}); ok {
		result.ChangesApplied = len(changes)
		for _, change := range changes {
			if changeMap, ok := change.(map[string]interface{}); ok {
				if filePath, ok := changeMap["path"].(string); ok {
					result.FilesModified = append(result.FilesModified, filePath)
				}
			}
		}
	}

	// Extract diff information
	if diff, ok := rawResults["diff"].(string); ok {
		result.Diff = diff
	}

	// Calculate validation score based on successful changes
	if result.ChangesApplied > 0 {
		result.ValidationScore = 0.9 // Placeholder - would be more sophisticated
	}

	// Extract errors and warnings
	if errors, ok := rawResults["errors"].([]interface{}); ok {
		for _, errItem := range errors {
			if errMap, ok := errItem.(map[string]interface{}); ok {
				transformErr := TransformationError{
					Type:        "error",
					Recoverable: false,
				}
				if msg, ok := errMap["message"].(string); ok {
					transformErr.Message = msg
				}
				if file, ok := errMap["file"].(string); ok {
					transformErr.File = file
				}
				result.Errors = append(result.Errors, transformErr)
			}
		}
		if len(result.Errors) > 0 {
			result.Success = false
		}
	}

	return result, nil
}