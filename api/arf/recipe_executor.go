package arf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
	"github.com/iw2rmb/ploy/api/arf/storage"
	"gopkg.in/yaml.v3"
)

// RecipeExecutor executes transformation recipes
type RecipeExecutor struct {
	storage              storage.RecipeStorage
	sandboxMgr           SandboxManager
	openRewriteEngine    *OpenRewriteEngine
	openRewriteDispatcher *OpenRewriteDispatcher
}

// NewRecipeExecutor creates a new recipe executor
func NewRecipeExecutor(storage storage.RecipeStorage, sandboxMgr SandboxManager) *RecipeExecutor {
	return &RecipeExecutor{
		storage:           storage,
		sandboxMgr:        sandboxMgr,
		openRewriteEngine: NewOpenRewriteEngine(),
	}
}

// NewRecipeExecutorWithDispatcher creates a new recipe executor with OpenRewrite dispatcher
func NewRecipeExecutorWithDispatcher(
	storage storage.RecipeStorage,
	sandboxMgr SandboxManager,
	dispatcher *OpenRewriteDispatcher,
) *RecipeExecutor {
	return &RecipeExecutor{
		storage:              storage,
		sandboxMgr:           sandboxMgr,
		openRewriteEngine:    NewOpenRewriteEngine(),
		openRewriteDispatcher: dispatcher,
	}
}

// ExecuteRecipeByID executes a recipe by ID against a repository
func (e *RecipeExecutor) ExecuteRecipeByID(ctx context.Context, recipeID string, repoPath string) (*TransformationResult, error) {
	// Try to load recipe from storage
	recipe, err := e.storage.GetRecipe(ctx, recipeID)
	if err != nil {
		// Check if this is an OpenRewrite recipe and we have a dispatcher
		if e.isOpenRewriteRecipe(recipeID) && e.openRewriteDispatcher != nil {
			fmt.Printf("[RecipeExecutor] Recipe %s not found in cache, triggering dynamic download via Nomad\n", recipeID)
			
			// Parse OpenRewrite recipe ID to get Maven coordinates
			req, parseErr := ParseOpenRewriteRecipeID(recipeID)
			if parseErr != nil {
				return nil, fmt.Errorf("failed to parse OpenRewrite recipe ID %s: %w", recipeID, parseErr)
			}
			
			// Set the repository path
			req.RepoPath = repoPath
			
			fmt.Printf("[RecipeExecutor] Dispatching recipe %s to OpenRewrite engine for discovery and execution\n", recipeID)
			
			// Dispatch to Nomad for dynamic download and execution
			result, execErr := e.openRewriteDispatcher.ExecuteOpenRewriteRecipe(ctx, req)
			if execErr != nil {
				fmt.Printf("[RecipeExecutor] Failed to execute recipe %s: %v\n", recipeID, execErr)
				return nil, fmt.Errorf("failed to execute OpenRewrite recipe %s via dispatcher: %w", recipeID, execErr)
			}
			
			fmt.Printf("[RecipeExecutor] Recipe %s executed successfully, result: %+v\n", recipeID, result)
			
			// Recipe was successfully downloaded and executed, optionally cache it
			// Note: The runner.sh script already registers the recipe with the API
			// so it will be available in storage for future use
			
			return result, nil
		}
		
		// Not an OpenRewrite recipe or no dispatcher available
		fmt.Printf("[RecipeExecutor] Recipe %s not found and no fallback available (isOpenRewrite=%v, hasDispatcher=%v)\n", 
			recipeID, e.isOpenRewriteRecipe(recipeID), e.openRewriteDispatcher != nil)
		return nil, fmt.Errorf("failed to load recipe %s: %w", recipeID, err)
	}

	fmt.Printf("[RecipeExecutor] Recipe %s found in cache, executing from storage\n", recipeID)
	return e.ExecuteRecipeObject(ctx, recipe, repoPath)
}

// isOpenRewriteRecipe checks if a recipe ID is an OpenRewrite recipe
func (e *RecipeExecutor) isOpenRewriteRecipe(recipeID string) bool {
	// OpenRewrite recipes typically start with "org.openrewrite"
	// or are in the standard Java migration format
	return len(recipeID) > 0 && 
		(recipeID[:min(len(recipeID), 15)] == "org.openrewrite" ||
		 recipeID[:min(len(recipeID), 8)] == "rewrite." ||
		 // Also check for common OpenRewrite recipe patterns
		 containsOpenRewritePattern(recipeID))
}

// containsOpenRewritePattern checks for common OpenRewrite patterns
func containsOpenRewritePattern(recipeID string) bool {
	patterns := []string{
		"Java", "Spring", "Junit", "Maven", "Gradle",
		"migrate", "upgrade", "modernize", "refactor",
	}
	for _, pattern := range patterns {
		if containsIgnoreCase(recipeID, pattern) {
			return true
		}
	}
	return false
}

// containsIgnoreCase checks if a string contains a substring ignoring case
func containsIgnoreCase(s, substr string) bool {
	// Simple case-insensitive contains check
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if i+j >= len(s) {
				return false
			}
			if s[i+j] != substr[j] && s[i+j] != substr[j]+32 && s[i+j] != substr[j]-32 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ExecuteRecipeObject executes a recipe object against a repository
func (e *RecipeExecutor) ExecuteRecipeObject(ctx context.Context, recipe *models.Recipe, repoPath string) (*TransformationResult, error) {
	startTime := time.Now()

	result := &TransformationResult{
		RecipeID:       recipe.ID,
		Success:        true,
		ChangesApplied: 0,
		FilesModified:  []string{},
	}

	// Execute each step
	for i, step := range recipe.Steps {
		stepResult, err := e.executeStep(ctx, &step, repoPath)
		if err != nil {
			// Handle error based on step's error policy
			switch step.OnError {
			case models.ErrorActionFail:
				result.Success = false
				result.Errors = append(result.Errors, TransformationError{
					Type:    "step_failure",
					Message: fmt.Sprintf("Step %d (%s) failed: %v", i+1, step.Name, err),
				})
				return result, nil
			case models.ErrorActionContinue:
				// Log and continue
				fmt.Printf("Warning: Step %d (%s) failed: %v\n", i+1, step.Name, err)
				continue
			case models.ErrorActionRollback:
				// TODO: Implement rollback in Phase 5.3
				result.Success = false
				result.Errors = append(result.Errors, TransformationError{
					Type:    "rollback_failed",
					Message: fmt.Sprintf("Step %d (%s) failed, rollback not yet implemented: %v", i+1, step.Name, err),
				})
				return result, nil
			}
		}

		// Aggregate results
		if stepResult != nil {
			result.ChangesApplied += stepResult.ChangesApplied
			result.FilesModified = append(result.FilesModified, stepResult.FilesModified...)
			if stepResult.Diff != "" {
				if result.Diff != "" {
					result.Diff += "\n"
				}
				result.Diff += stepResult.Diff
			}
		}
	}

	result.ExecutionTime = time.Since(startTime)
	return result, nil
}

// executeStep executes a single recipe step
func (e *RecipeExecutor) executeStep(ctx context.Context, step *models.RecipeStep, repoPath string) (*TransformationResult, error) {
	// Check conditions
	if !e.checkConditions(step.Conditions, repoPath) {
		return nil, nil // Skip step
	}

	// Apply timeout if specified
	if step.Timeout.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, step.Timeout.Duration)
		defer cancel()
	}

	// Execute based on step type
	switch step.Type {
	case models.StepTypeOpenRewrite:
		// Use real OpenRewrite engine
		return e.openRewriteEngine.Execute(ctx, step, repoPath)
	case models.StepTypeShellScript:
		// Shell script execution placeholder
		return nil, fmt.Errorf("shell script execution not yet implemented")
	case models.StepTypeFileOperation:
		// File operation placeholder
		return nil, fmt.Errorf("file operations not yet implemented")
	case models.StepTypeRegexReplace:
		// Regex replacement placeholder
		return nil, fmt.Errorf("regex replacement not yet implemented")
	case models.StepTypeASTTransform:
		// AST transformation placeholder
		return nil, fmt.Errorf("AST transformation not yet implemented")
	case models.StepTypeComposite:
		// Composite recipe placeholder
		return nil, fmt.Errorf("composite recipes not yet implemented")
	default:
		return nil, fmt.Errorf("unknown step type: %s", step.Type)
	}
}

// executeMockOpenRewrite provides temporary mock execution for OpenRewrite steps
func (e *RecipeExecutor) executeMockOpenRewrite(ctx context.Context, step *models.RecipeStep, repoPath string) (*TransformationResult, error) {
	recipe, ok := step.Config["recipe"].(string)
	if !ok {
		return nil, fmt.Errorf("OpenRewrite step missing recipe configuration")
	}

	// Return a simple mock result
	result := &TransformationResult{
		RecipeID:       recipe,
		Success:        true,
		ChangesApplied: 1,
		FilesModified:  []string{"MockFile.java"},
		Diff:           fmt.Sprintf("Applied mock transformation for recipe: %s", recipe),
		ExecutionTime:  100 * time.Millisecond,
	}

	return result, nil
}

// checkConditions evaluates step execution conditions
func (e *RecipeExecutor) checkConditions(conditions []models.ExecutionCondition, repoPath string) bool {
	for _, condition := range conditions {
		switch condition.Type {
		case models.ConditionFileExists:
			path := filepath.Join(repoPath, condition.Value.(string))
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return false
			}
		case models.ConditionFileNotExists:
			path := filepath.Join(repoPath, condition.Value.(string))
			if _, err := os.Stat(path); err == nil {
				return false
			}
			// Add more condition types as needed
		}
	}
	return true
}

// LoadRecipeFromFile loads a recipe from a YAML file
func LoadRecipeFromFile(path string) (*models.Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipe file: %w", err)
	}

	var recipe models.Recipe
	if err := yaml.Unmarshal(data, &recipe); err != nil {
		return nil, fmt.Errorf("failed to parse recipe YAML: %w", err)
	}

	// Set system fields
	recipe.SetSystemFields("system")

	return &recipe, nil
}
