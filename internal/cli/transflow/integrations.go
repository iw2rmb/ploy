package transflow

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/iw2rmb/ploy/api/arf"
	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/git/provider"
)

// ARFGitOperations wraps the existing ARF Git operations
type ARFGitOperations struct {
	*arf.GitOperations
}

// NewARFGitOperations creates a new ARF Git operations wrapper
func NewARFGitOperations(workDir string) *ARFGitOperations {
	return &ARFGitOperations{
		GitOperations: arf.NewGitOperations(workDir),
	}
}

// ARFRecipeExecutor implements recipe execution via the ARF system
type ARFRecipeExecutor struct {
	controllerURL string
}

// NewARFRecipeExecutor creates a new ARF recipe executor
func NewARFRecipeExecutor(controllerURL string) *ARFRecipeExecutor {
	return &ARFRecipeExecutor{
		controllerURL: controllerURL,
	}
}

// ExecuteRecipes executes OpenRewrite recipes using the ARF system
func (e *ARFRecipeExecutor) ExecuteRecipes(ctx context.Context, workspacePath string, recipeIDs []string) error {
	// For MVP, we'll invoke the ploy arf command directly
	// This reuses the existing ARF pipeline without duplicating logic

	for _, recipeID := range recipeIDs {
		args := []string{"arf", "recipes", "transform", "--recipe", recipeID}
		if e.controllerURL != "" {
			args = append(args, "--controller", e.controllerURL)
		}

		// Execute ploy arf transform command
		cmd := exec.CommandContext(ctx, "ploy", args...)
		cmd.Dir = workspacePath

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to execute recipe %s: %w", recipeID, err)
		}
	}

	return nil
}

// SharedPushBuildChecker implements build checking using SharedPush
type SharedPushBuildChecker struct {
	controllerURL string
}

// NewSharedPushBuildChecker creates a new build checker using SharedPush
func NewSharedPushBuildChecker(controllerURL string) *SharedPushBuildChecker {
	return &SharedPushBuildChecker{
		controllerURL: controllerURL,
	}
}

// CheckBuild performs a build check using the existing SharedPush infrastructure
func (b *SharedPushBuildChecker) CheckBuild(ctx context.Context, config common.DeployConfig) (*common.DeployResult, error) {
	// Set the controller URL in the config
	config.ControllerURL = b.controllerURL
	config.IsPlatform = false // transflow uses ploy mode, not ployman mode

	// Use SharedPush to perform the build check
	// SharedPush already supports build-only mode when used with specific endpoints
	result, err := common.SharedPush(config)
	if err != nil {
		return nil, fmt.Errorf("build check failed: %w", err)
	}

	return result, nil
}

// TransflowIntegrations provides factory methods for creating concrete implementations
type TransflowIntegrations struct {
	ControllerURL string
	WorkDir       string
}

// NewTransflowIntegrations creates a new integrations factory
func NewTransflowIntegrations(controllerURL, workDir string) *TransflowIntegrations {
	return &TransflowIntegrations{
		ControllerURL: controllerURL,
		WorkDir:       workDir,
	}
}

// CreateGitOperations creates a Git operations implementation
func (i *TransflowIntegrations) CreateGitOperations() GitOperationsInterface {
	return NewARFGitOperations(i.WorkDir)
}

// CreateRecipeExecutor creates a recipe executor implementation
func (i *TransflowIntegrations) CreateRecipeExecutor() RecipeExecutorInterface {
	return NewARFRecipeExecutor(i.ControllerURL)
}

// CreateBuildChecker creates a build checker implementation
func (i *TransflowIntegrations) CreateBuildChecker() BuildCheckerInterface {
	return NewSharedPushBuildChecker(i.ControllerURL)
}

// CreateGitProvider creates a Git provider implementation for MR operations
func (i *TransflowIntegrations) CreateGitProvider() provider.GitProvider {
	return provider.NewGitLabProvider()
}

// CreateConfiguredRunner creates a fully configured TransflowRunner with real integrations
func (i *TransflowIntegrations) CreateConfiguredRunner(config *TransflowConfig) (*TransflowRunner, error) {
	runner, err := NewTransflowRunner(config, i.WorkDir)
	if err != nil {
		return nil, err
	}

	// Inject the concrete implementations
	runner.SetGitOperations(i.CreateGitOperations())
	runner.SetRecipeExecutor(i.CreateRecipeExecutor())
	runner.SetBuildChecker(i.CreateBuildChecker())
	runner.SetGitProvider(i.CreateGitProvider())

	return runner, nil
}

// GetDefaultControllerURL returns the default controller URL for transflow operations
func GetDefaultControllerURL() string {
	// This would typically come from environment or config
	// For now, return a placeholder that matches the ARF system defaults
	return "http://localhost:8080" // This should be configurable
}
