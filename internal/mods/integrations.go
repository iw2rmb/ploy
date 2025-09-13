package mods

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/iw2rmb/ploy/api/arf"
	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/git/provider"
	"github.com/iw2rmb/ploy/internal/orchestration"
	"github.com/iw2rmb/ploy/internal/storage"
)

// Note: ARF-based Git/Recipe execution wrappers removed with ARF transforms HTTP removal.

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
	config.IsPlatform = false // Mods uses ploy mode, not ployman mode
	// Mods build gate must be ephemeral; ask API to tear down sandboxed app after gate
	config.BuildOnly = true

	// Propagate working directory hint from metadata if present
	if config.Metadata != nil {
		if wd, ok := config.Metadata["working_dir"]; ok && wd != "" {
			config.WorkingDir = wd
		}
	}
	// Optional: emit via controller reporter if exec ID present
	if execID := os.Getenv("PLOY_MODS_EXECUTION_ID"); execID != "" {
		rep := NewControllerEventReporter(b.controllerURL, execID)
		_ = rep.Report(ctx, Event{Phase: "build", Step: "build-gate", Level: "info", Message: fmt.Sprintf("start app=%s lane=%s env=%s wd=%s", config.App, config.Lane, config.Environment, config.WorkingDir)})
	}
	log.Printf("[Mods Build] Starting build check: controller=%s app=%s lane=%s env=%s wd=%s", b.controllerURL, config.App, config.Lane, config.Environment, config.WorkingDir)

	// Use SharedPush to perform the build check
	// SharedPush already supports build-only mode when used with specific endpoints
	result, err := common.SharedPush(config)
	if err != nil {
		// Emit additional context for debugging 500s
		return nil, fmt.Errorf("build check failed (controller=%s app=%s lane=%s env=%s): %w", b.controllerURL, config.App, config.Lane, config.Environment, err)
	}

	if result != nil && !result.Success {
		if execID := os.Getenv("PLOY_MODS_EXECUTION_ID"); execID != "" {
			rep := NewControllerEventReporter(b.controllerURL, execID)
			_ = rep.Report(ctx, Event{Phase: "build", Step: "build-gate", Level: "error", Message: fmt.Sprintf("unsuccessful: %s", result.Message)})
		}
		log.Printf("[Mods Build] Unsuccessful build: controller=%s app=%s lane=%s env=%s msg=%s", b.controllerURL, config.App, config.Lane, config.Environment, result.Message)
		return result, fmt.Errorf("build check unsuccessful (controller=%s app=%s lane=%s env=%s): %s", b.controllerURL, config.App, config.Lane, config.Environment, result.Message)
	}
	if execID := os.Getenv("PLOY_MODS_EXECUTION_ID"); execID != "" {
		rep := NewControllerEventReporter(b.controllerURL, execID)
		_ = rep.Report(ctx, Event{Phase: "build", Step: "build-gate", Level: "info", Message: fmt.Sprintf("succeeded version=%s", result.Version)})
	}
	log.Printf("[Mods Build] Build check succeeded: controller=%s app=%s lane=%s env=%s version=%s", b.controllerURL, config.App, config.Lane, config.Environment, result.Version)
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

// ModsIntegrations provides factory methods for creating concrete implementations
type ModIntegrations struct {
	ControllerURL string
	WorkDir       string
	TestMode      bool // Use mock implementations when true
}

// NewModIntegrations creates a new integrations factory
func NewModIntegrations(controllerURL, workDir string) *ModIntegrations {
	return &ModIntegrations{
		ControllerURL: controllerURL,
		WorkDir:       workDir,
		TestMode:      false,
	}
}

// NewModIntegrationsWithTestMode creates a new integrations factory with test mode option
func NewModIntegrationsWithTestMode(controllerURL, workDir string, testMode bool) *ModIntegrations {
	return &ModIntegrations{
		ControllerURL: controllerURL,
		WorkDir:       workDir,
		TestMode:      testMode,
	}
}

// ARFGitOperations wraps the ARF Git operations to satisfy the interface
type ARFGitOperations struct{ *arf.GitOperations }

func NewARFGitOperations(workDir string) *ARFGitOperations {
	return &ARFGitOperations{GitOperations: arf.NewGitOperations(workDir)}
}

// CreateGitOperations creates a Git operations implementation
func (i *ModIntegrations) CreateGitOperations() GitOperationsInterface {
	if i.TestMode {
		return NewMockGitOperations() // Use mock implementation for testing
	}
	return NewARFGitOperations(i.WorkDir)
}

// CreateRecipeExecutor creates a recipe executor implementation
func (i *ModIntegrations) CreateRecipeExecutor() RecipeExecutorInterface {
	if i.TestMode {
		return NewMockRecipeExecutor() // Use mock implementation for testing
	}
	// No-op in production for now; Mods does not rely on ARF recipe executor
	return NewMockRecipeExecutor()
}

// CreateBuildChecker creates a build checker implementation
func (i *ModIntegrations) CreateBuildChecker() BuildCheckerInterface {
	if i.TestMode {
		return NewMockBuildChecker() // Use mock implementation for testing
	}
	return NewSharedPushBuildChecker(i.ControllerURL)
}

// CreateGitProvider creates a Git provider implementation for MR operations
func (i *ModIntegrations) CreateGitProvider() provider.GitProvider {
	if i.TestMode {
		return NewMockGitProvider() // Use mock implementation for testing
	}
	return provider.NewGitLabProvider()
}

// CreateKBIntegration creates a KB integration for learning from healing attempts
func (i *ModIntegrations) CreateKBIntegration() KBIntegrator {
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

// CreateConfiguredRunner creates a fully configured ModRunner with KB learning integration
func (i *ModIntegrations) CreateConfiguredRunner(config *ModConfig) (*ModRunner, error) {
	// Create KB integration
	kbIntegration := i.CreateKBIntegration()

	// Create KB-enhanced runner
	kbRunner, err := NewKBModRunner(config, i.WorkDir, kbIntegration)
	if err != nil {
		return nil, err
	}

	// Get the embedded ModRunner for dependency injection
	runner := kbRunner.ModRunner

	// Inject the concrete implementations
	runner.SetGitOperations(i.CreateGitOperations())
	runner.SetRecipeExecutor(i.CreateRecipeExecutor())
	runner.SetBuildChecker(i.CreateBuildChecker())
	runner.SetGitProvider(i.CreateGitProvider())
	// Wire default modular adapters
	runner.SetTransformationExecutor(NewTransformationExecutorAdapter(runner))
	// BuildGate is already provided via SetBuildChecker; Repo/MR modules via SetGitOperations/SetGitProvider

	// Enable self-healing by providing a no-op submitter; production path prefers runner.
	runner.SetJobSubmitter(NoopJobSubmitter{})
	// Wire default production healing orchestrator (wraps existing fanout)
	runner.SetHealingOrchestrator(NewProdHealingOrchestrator(runner.jobSubmitter, runner))

	// Return the embedded ModRunner; KB integration remains wired inside kbRunner
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
