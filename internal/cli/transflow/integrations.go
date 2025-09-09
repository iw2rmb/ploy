package transflow

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

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

	// Resolve origin URL from the cloned repository to satisfy ARF CLI input requirements
	// (ARF transform requires either --repo or --archive)
	originURL := ""
	{
		cmd := exec.CommandContext(ctx, "git", "config", "--get", "remote.origin.url")
		cmd.Dir = workspacePath
		if out, err := cmd.Output(); err == nil {
			originURL = strings.TrimSpace(string(out))
		}
	}

	for _, recipeID := range recipeIDs {
		// Use robust ARF transform path; prefer repository input inferred from local clone
		args := []string{"arf", "transform", "--recipe", recipeID}
		if originURL != "" {
			args = append(args, "--repo", originURL)
		}
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

	log.Printf("[Transflow Build] Starting build check: controller=%s app=%s lane=%s env=%s", b.controllerURL, config.App, config.Lane, config.Environment)

	// Use SharedPush to perform the build check
	// SharedPush already supports build-only mode when used with specific endpoints
	result, err := common.SharedPush(config)
	if err != nil {
		// Emit additional context for debugging 500s
		return nil, fmt.Errorf("build check failed (controller=%s app=%s lane=%s env=%s): %w", b.controllerURL, config.App, config.Lane, config.Environment, err)
	}

	if result != nil && !result.Success {
		log.Printf("[Transflow Build] Unsuccessful build: controller=%s app=%s lane=%s env=%s msg=%s", b.controllerURL, config.App, config.Lane, config.Environment, result.Message)
		return result, fmt.Errorf("build check unsuccessful (controller=%s app=%s lane=%s env=%s): %s", b.controllerURL, config.App, config.Lane, config.Environment, result.Message)
	}
	log.Printf("[Transflow Build] Build check succeeded: controller=%s app=%s lane=%s env=%s version=%s", b.controllerURL, config.App, config.Lane, config.Environment, result.Version)
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
func (i *TransflowIntegrations) CreateConfiguredRunner(config *TransflowConfig) (*TransflowRunner, error) {
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

	// Ensure self-healing path is enabled by providing a non-nil submitter marker.
	// The production submission path uses the runner directly; this marker only
	// signals that healing should be attempted when builds fail.
	runner.SetJobSubmitter(struct{}{})

    // Return the embedded TransflowRunner; KB integration remains wired inside kbRunner
    return runner, nil
}

// GetDefaultControllerURL returns the default controller URL for transflow operations
func GetDefaultControllerURL() string {
	// First check if PLOY_CONTROLLER is explicitly set
	if url := os.Getenv("PLOY_CONTROLLER"); url != "" {
		return url
	}

	// Check if PLOY_APPS_DOMAIN is set for SSL endpoint
	if domain := os.Getenv("PLOY_APPS_DOMAIN"); domain != "" {
		// Check for environment-specific subdomain
		if env := os.Getenv("PLOY_ENVIRONMENT"); env == "dev" {
			return fmt.Sprintf("https://api.dev.%s/v1", domain)
		}
		return fmt.Sprintf("https://api.%s/v1", domain)
	}

	// Default fallback for development
	return "https://api.dev.ployman.app/v1"
}
