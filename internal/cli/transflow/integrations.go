package transflow

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/iw2rmb/ploy/api/arf"
	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/git/provider"
	"github.com/iw2rmb/ploy/internal/orchestration"
	"github.com/iw2rmb/ploy/internal/storage"
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

	// Get current executable path to avoid PATH dependency issues
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable path: %w", err)
	}

	for _, recipeID := range recipeIDs {
		args := []string{"arf", "recipes", "transform", "--recipe", recipeID}
		if e.controllerURL != "" {
			args = append(args, "--controller", e.controllerURL)
		}

		// Execute ploy arf transform command using full executable path
		cmd := exec.CommandContext(ctx, execPath, args...)
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

// TestModeBuildChecker implements build checking for testing without external dependencies
type TestModeBuildChecker struct {
	shouldFail bool
}

// NewTestModeBuildChecker creates a new test mode build checker
func NewTestModeBuildChecker(shouldFail bool) *TestModeBuildChecker {
	return &TestModeBuildChecker{
		shouldFail: shouldFail,
	}
}

// CheckBuild performs a mock build check
func (m *TestModeBuildChecker) CheckBuild(ctx context.Context, config common.DeployConfig) (*common.DeployResult, error) {
	if m.shouldFail {
		return &common.DeployResult{
			Success: false,
			Message: "Mock build failed for testing",
		}, nil
	}

	return &common.DeployResult{
		Success:      true,
		Message:      "Mock build succeeded",
		Version:      "mock-v1.0.0",
		DeploymentID: "mock-deployment-123",
		URL:          "mock://test-image:latest",
	}, nil
}

// TransflowIntegrations provides factory methods for creating concrete implementations
type TransflowIntegrations struct {
	ControllerURL string
	WorkDir       string
	TestMode      bool // Use mock implementations when true
}

// NewTransflowIntegrations creates a new integrations factory
func NewTransflowIntegrations(controllerURL, workDir string) *TransflowIntegrations {
	return &TransflowIntegrations{
		ControllerURL: controllerURL,
		WorkDir:       workDir,
		TestMode:      false,
	}
}

// NewTransflowIntegrationsWithTestMode creates a new integrations factory with test mode option
func NewTransflowIntegrationsWithTestMode(controllerURL, workDir string, testMode bool) *TransflowIntegrations {
	return &TransflowIntegrations{
		ControllerURL: controllerURL,
		WorkDir:       workDir,
		TestMode:      testMode,
	}
}

// CreateGitOperations creates a Git operations implementation
func (i *TransflowIntegrations) CreateGitOperations() GitOperationsInterface {
	if i.TestMode {
		return NewMockGitOperations() // Use mock implementation for testing
	}
	return NewARFGitOperations(i.WorkDir)
}

// CreateRecipeExecutor creates a recipe executor implementation
func (i *TransflowIntegrations) CreateRecipeExecutor() RecipeExecutorInterface {
	if i.TestMode {
		return NewMockRecipeExecutor() // Use mock implementation for testing
	}
	return NewARFRecipeExecutor(i.ControllerURL)
}

// CreateBuildChecker creates a build checker implementation
func (i *TransflowIntegrations) CreateBuildChecker() BuildCheckerInterface {
	if i.TestMode {
		return NewMockBuildChecker() // Use mock implementation for testing
	}
	return NewSharedPushBuildChecker(i.ControllerURL)
}

// CreateGitProvider creates a Git provider implementation for MR operations
func (i *TransflowIntegrations) CreateGitProvider() provider.GitProvider {
	if i.TestMode {
		return NewMockGitProvider() // Use mock implementation for testing
	}
	return provider.NewGitLabProvider()
}

// CreateKBIntegration creates a KB integration for learning from healing attempts
func (i *TransflowIntegrations) CreateKBIntegration() KBIntegrator {
	if i.TestMode {
		// Return mock KB integration for testing
		return NewMockKBIntegration()
	}

	// Create production KB integration
	storageConfig := storage.Config{
		Master:      "localhost:9333", // Default SeaweedFS master
		Filer:       "localhost:8888", // Default SeaweedFS filer
		Collection:  "kb",
		Replication: "000", // No replication for development
		Timeout:     30,
	}

	storageClient, err := storage.New(storageConfig)
	if err != nil {
		// If storage creation fails, use a mock KB integration to prevent breaking the workflow
		return NewMockKBIntegration()
	}

	// Adapt StorageProvider to Storage interface
	storageBackend := storage.NewStorageAdapter(storageClient)

	kvStore := orchestration.NewKV()

	kbConfig := DefaultKBConfig()
	return NewKBIntegration(storageBackend, kvStore, kbConfig)
}

// CreateConfiguredRunner creates a fully configured TransflowRunner with KB learning integration
func (i *TransflowIntegrations) CreateConfiguredRunner(config *TransflowConfig) (*KBTransflowRunner, error) {
	// Create KB integration
	kbIntegration := i.CreateKBIntegration()

	// Create KB-enhanced runner
	kbRunner, err := NewKBTransflowRunner(config, i.WorkDir, kbIntegration)
	if err != nil {
		return nil, err
	}

	// Get the embedded TransflowRunner for dependency injection
	runner := kbRunner.TransflowRunner

	// Inject the concrete implementations
	runner.SetGitOperations(i.CreateGitOperations())
	runner.SetRecipeExecutor(i.CreateRecipeExecutor())
	runner.SetBuildChecker(i.CreateBuildChecker())
	runner.SetGitProvider(i.CreateGitProvider())

	// Return the KB-enhanced runner which will use KB learning when healing is needed
	return kbRunner, nil
}

// GetDefaultControllerURL returns the default controller URL for transflow operations
func GetDefaultControllerURL() string {
	// This would typically come from environment or config
	// For now, return a placeholder that matches the ARF system defaults
	return "http://localhost:8080" // This should be configurable
}
