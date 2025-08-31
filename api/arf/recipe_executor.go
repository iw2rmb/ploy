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
// The dispatcher parameter is optional and can be nil if OpenRewrite support is not needed
func NewRecipeExecutor(storage storage.RecipeStorage, sandboxMgr SandboxManager, dispatcher *OpenRewriteDispatcher) *RecipeExecutor {
	return &RecipeExecutor{
		storage:              storage,
		sandboxMgr:           sandboxMgr,
		openRewriteEngine:    NewOpenRewriteEngine(),
		openRewriteDispatcher: dispatcher,
	}
}

// ExecuteRecipeByID executes a recipe by ID against a repository
// Uses unified execution path based on recipe type
func (e *RecipeExecutor) ExecuteRecipeByID(ctx context.Context, recipeID string, repoPath string, recipeType string, transformationID string) (*TransformationResult, error) {
	fmt.Printf("[RecipeExecutor] Starting unified recipe execution: recipe=%s, type=%s, repoPath=%s\n", recipeID, recipeType, repoPath)
	
	// Unified execution path: OpenRewrite recipes ALWAYS go through dispatcher
	if recipeType == "openrewrite" {
		if e.openRewriteDispatcher == nil {
			fmt.Printf("[RecipeExecutor] ERROR: Recipe %s is OpenRewrite but dispatcher is nil - check server initialization\n", recipeID)
			return nil, fmt.Errorf("OpenRewrite recipe %s cannot be executed: dispatcher not initialized (check Nomad/SeaweedFS connectivity)", recipeID)
		}
		
		fmt.Printf("[RecipeExecutor] OpenRewrite recipe - using unified dispatcher execution path\n")
		
		// Parse OpenRewrite recipe ID to get Maven coordinates
		req, parseErr := ParseOpenRewriteRecipeID(recipeID)
		if parseErr != nil {
			fmt.Printf("[RecipeExecutor] Failed to parse OpenRewrite recipe ID %s: %v\n", recipeID, parseErr)
			return nil, fmt.Errorf("failed to parse OpenRewrite recipe ID %s: %w", recipeID, parseErr)
		}
		
		// Set the repository path and transformation ID
		req.RepoPath = repoPath
		req.TransformationID = transformationID
		
		fmt.Printf("[RecipeExecutor] Dispatching recipe %s to OpenRewrite engine using openrewrite-jvm:latest (transformationID: %s)\n", recipeID, transformationID)
		
		// Add timeout context for dispatcher call (inherit from parent context)
		dispatcherCtx, cancel := context.WithTimeout(ctx, 25*time.Minute)
		defer cancel()
		
		dispatcherStart := time.Now()
		
		// Dispatch to Nomad for unified execution using openrewrite-jvm:latest
		result, execErr := e.openRewriteDispatcher.ExecuteOpenRewriteRecipe(dispatcherCtx, req)
		if execErr != nil {
			if dispatcherCtx.Err() == context.DeadlineExceeded {
				fmt.Printf("[RecipeExecutor] OpenRewrite dispatcher timed out after 25m for recipe %s\n", recipeID)
				return nil, fmt.Errorf("OpenRewrite dispatcher timed out for recipe %s after 25 minutes", recipeID)
			}
			fmt.Printf("[RecipeExecutor] Failed to execute recipe %s: %v\n", recipeID, execErr)
			return nil, fmt.Errorf("failed to execute OpenRewrite recipe %s via dispatcher: %w", recipeID, execErr)
		}
		
		fmt.Printf("[RecipeExecutor] OpenRewrite recipe %s executed successfully after %v\n", recipeID, time.Since(dispatcherStart))
		return result, nil
	}
	
	// Non-OpenRewrite recipes: try storage first
	recipe, err := e.storage.GetRecipe(ctx, recipeID)
	if err != nil {
		fmt.Printf("[RecipeExecutor] Recipe %s not found in storage and not OpenRewrite type (type=%s)\n", recipeID, recipeType)
		return nil, fmt.Errorf("failed to load recipe %s: %w", recipeID, err)
	}

	fmt.Printf("[RecipeExecutor] Non-OpenRewrite recipe %s found in storage, executing\n", recipeID)
	return e.ExecuteRecipeObject(ctx, recipe, repoPath)
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
