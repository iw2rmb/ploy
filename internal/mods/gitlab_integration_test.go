package mods

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/git/provider"
)

// MockGitLabProvider implements GitProvider for testing
type MockGitLabProvider struct {
	validateError    error
	createUpdateFunc func(ctx context.Context, config provider.MRConfig) (*provider.MRResult, error)
}

func (m *MockGitLabProvider) ValidateConfiguration() error {
	return m.validateError
}

func (m *MockGitLabProvider) CreateOrUpdateMR(ctx context.Context, config provider.MRConfig) (*provider.MRResult, error) {
	if m.createUpdateFunc != nil {
		return m.createUpdateFunc(ctx, config)
	}
	return &provider.MRResult{
		MRURL:   "https://gitlab.example.com/namespace/project/-/merge_requests/123",
		MRID:    123,
		Created: true,
	}, nil
}

func TestTransflowRunner_GitLabIntegration(t *testing.T) {
	// Stub external integrations and provide exec ID
	oldSubmit := submitAndWaitTerminal
	oldDL := downloadToFileFn
	oldHas := hasRepoChangesFn
	oldVDP := validateDiffPathsFn
	oldVUD := validateUnifiedDiffFn
	oldAD := applyUnifiedDiffFn
	baseValidate := func(string) error { return nil }
	oldValidate := validateJob
	validateJob = baseValidate
	submitAndWaitTerminal = func(string, time.Duration) error { return nil }
	downloadToFileFn = func(_ string, dest string) error {
		_ = os.MkdirAll(filepath.Dir(dest), 0755)
		diff := "--- a/pom.xml\n+++ b/pom.xml\n@@ -1 +1 @@\n-<project></project>\n+<project><modelVersion>4.0.0</modelVersion></project>\n"
		return os.WriteFile(dest, []byte(diff), 0644)
	}
	hasRepoChangesFn = func(string) (bool, error) { return true, nil }
	validateDiffPathsFn = func(string, []string) error { return nil }
	validateUnifiedDiffFn = func(context.Context, string, string) error { return nil }
	applyUnifiedDiffFn = func(context.Context, string, string) error { return nil }
	os.Setenv("PLOY_TRANSFLOW_EXECUTION_ID", "gitlab-test")
	defer func() {
		submitAndWaitTerminal = oldSubmit
		downloadToFileFn = oldDL
		hasRepoChangesFn = oldHas
		validateDiffPathsFn = oldVDP
		validateUnifiedDiffFn = oldVUD
		applyUnifiedDiffFn = oldAD
		validateJob = oldValidate
		os.Unsetenv("PLOY_TRANSFLOW_EXECUTION_ID")
	}()
	// Create a temporary workspace
	workspaceDir := t.TempDir()

	// Create test configuration
	config := &TransflowConfig{
		ID:         "test-gitlab-integration",
		TargetRepo: "https://gitlab.example.com/namespace/project.git",
		BaseRef:    "refs/heads/main",
		Lane:       "C",
		Steps: []TransflowStep{
			{
				ID:                 "java-migration",
				Type:               "orw-apply",
				Recipes:            []string{"org.openrewrite.java.migrate.UpgradeToJava17"},
				RecipeGroup:        "org.openrewrite.recipe",
				RecipeArtifact:     "rewrite-migrate-java",
				RecipeVersion:      "3.17.0",
				MavenPluginVersion: "6.18.0",
			},
		},
	}

	// Create runner
	runner, err := NewTransflowRunner(config, workspaceDir)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}

	// Set up mock dependencies (using existing mock pattern)
	mockGitOps := &MockGitOperations{
		// All errors are nil for successful flow
	}

	mockRecipeExecutor := &MockRecipeExecutor{
		// ExecuteError is nil for successful execution
	}

	mockBuildChecker := &MockBuildChecker{
		BuildResult: &common.DeployResult{
			Success: true,
			Version: "v1.0.0-test",
			Message: "Build completed successfully",
		},
	}

	// Set up GitLab provider with test expectations
	mockGitLabProvider := &MockGitLabProvider{
		createUpdateFunc: func(ctx context.Context, config provider.MRConfig) (*provider.MRResult, error) {
			// Verify the MR config has expected values
			if config.RepoURL != "https://gitlab.example.com/namespace/project.git" {
				t.Errorf("Expected repo URL %s, got %s", "https://gitlab.example.com/namespace/project.git", config.RepoURL)
			}
			if config.TargetBranch != "refs/heads/main" {
				t.Errorf("Expected target branch %s, got %s", "refs/heads/main", config.TargetBranch)
			}
			if !strings.Contains(config.SourceBranch, "workflow/test-gitlab-integration") {
				t.Errorf("Expected source branch to contain workflow ID, got %s", config.SourceBranch)
			}
			if config.Title != "Transflow: test-gitlab-integration" {
				t.Errorf("Expected title %s, got %s", "Transflow: test-gitlab-integration", config.Title)
			}
			if len(config.Labels) != 2 || config.Labels[0] != "ploy" || config.Labels[1] != "tfl" {
				t.Errorf("Expected labels [ploy, tfl], got %v", config.Labels)
			}
			if !strings.Contains(config.Description, "Transflow Workflow: test-gitlab-integration") {
				t.Errorf("Expected description to contain workflow ID, got %s", config.Description)
			}

			return &provider.MRResult{
				MRURL:   "https://gitlab.example.com/namespace/project/-/merge_requests/123",
				MRID:    123,
				Created: true,
			}, nil
		},
	}

	// Inject dependencies
	runner.SetGitOperations(mockGitOps)
	runner.SetRecipeExecutor(mockRecipeExecutor)
	runner.SetBuildChecker(mockBuildChecker)
	runner.SetGitProvider(mockGitLabProvider)

	// Execute transflow
	ctx := context.Background()
	result, err := runner.Run(ctx)

	// Verify results
	if err != nil {
		t.Fatalf("Transflow execution failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("Expected transflow to succeed, but got failure: %s", result.ErrorMessage)
	}

	if result.MRURL != "https://gitlab.example.com/namespace/project/-/merge_requests/123" {
		t.Errorf("Expected MR URL to be set, got %s", result.MRURL)
	}

	// Verify MR step was executed
	var mrStepFound bool
	for _, step := range result.StepResults {
		if step.StepID == "mr" {
			mrStepFound = true
			if !step.Success {
				t.Errorf("Expected MR step to succeed, got failure: %s", step.Message)
			}
			if !strings.Contains(step.Message, "created") {
				t.Errorf("Expected MR step message to indicate creation, got: %s", step.Message)
			}
			break
		}
	}

	if !mrStepFound {
		t.Errorf("Expected MR step to be executed")
	}

	// Verify summary includes MR URL
	summary := result.Summary()
	if !strings.Contains(summary, "Merge Request: https://gitlab.example.com/namespace/project/-/merge_requests/123") {
		t.Errorf("Expected summary to include MR URL, got: %s", summary)
	}
}

func TestTransflowRunner_GitLabIntegration_ConfigurationInvalid(t *testing.T) {
	// Stubs as above to keep unit-only
	oldSubmit := submitAndWaitTerminal
	oldDL := downloadToFileFn
	oldHas := hasRepoChangesFn
	oldVDP := validateDiffPathsFn
	oldVUD := validateUnifiedDiffFn
	oldAD := applyUnifiedDiffFn
	baseValidate := func(string) error { return nil }
	oldValidate := validateJob
	validateJob = baseValidate
	submitAndWaitTerminal = func(string, time.Duration) error { return nil }
	downloadToFileFn = func(_ string, dest string) error {
		_ = os.MkdirAll(filepath.Dir(dest), 0755)
		diff := "--- a/pom.xml\n+++ b/pom.xml\n@@ -1 +1 @@\n-<project></project>\n+<project><modelVersion>4.0.0</modelVersion></project>\n"
		return os.WriteFile(dest, []byte(diff), 0644)
	}
	hasRepoChangesFn = func(string) (bool, error) { return true, nil }
	validateDiffPathsFn = func(string, []string) error { return nil }
	validateUnifiedDiffFn = func(context.Context, string, string) error { return nil }
	applyUnifiedDiffFn = func(context.Context, string, string) error { return nil }
	os.Setenv("PLOY_TRANSFLOW_EXECUTION_ID", "gitlab-test")
	defer func() {
		submitAndWaitTerminal = oldSubmit
		downloadToFileFn = oldDL
		hasRepoChangesFn = oldHas
		validateDiffPathsFn = oldVDP
		validateUnifiedDiffFn = oldVUD
		applyUnifiedDiffFn = oldAD
		validateJob = oldValidate
		os.Unsetenv("PLOY_TRANSFLOW_EXECUTION_ID")
	}()
	// Test that MR creation fails gracefully when GitLab configuration is invalid
	workspaceDir := t.TempDir()

	config := &TransflowConfig{
		ID:         "test-gitlab-config-invalid",
		TargetRepo: "https://gitlab.example.com/namespace/project.git",
		BaseRef:    "refs/heads/main",
		Lane:       "C",
		Steps: []TransflowStep{
			{
				ID:                 "java-migration",
				Type:               "orw-apply",
				Recipes:            []string{"org.openrewrite.java.migrate.UpgradeToJava17"},
				RecipeGroup:        "org.openrewrite.recipe",
				RecipeArtifact:     "rewrite-migrate-java",
				RecipeVersion:      "3.17.0",
				MavenPluginVersion: "6.18.0",
			},
		},
	}

	runner, err := NewTransflowRunner(config, workspaceDir)
	if err != nil {
		t.Fatalf("Failed to create runner: %v", err)
	}

	// Set up basic mocks for successful workflow
	runner.SetGitOperations(&MockGitOperations{
		// All errors are nil for successful flow
	})
	runner.SetRecipeExecutor(&MockRecipeExecutor{
		// ExecuteError is nil for successful execution
	})
	runner.SetBuildChecker(&MockBuildChecker{
		BuildResult: &common.DeployResult{
			Success: true,
			Version: "v1.0.0-test",
			Message: "Build completed successfully",
		},
	})

	// Set GitLab provider with invalid configuration
	mockGitLabProvider := &MockGitLabProvider{
		validateError: os.ErrPermission, // Simulate configuration error
	}
	runner.SetGitProvider(mockGitLabProvider)

	// Execute transflow
	ctx := context.Background()
	result, err := runner.Run(ctx)

	// Verify that transflow still succeeds (MR creation is optional)
	if err != nil {
		t.Fatalf("Expected transflow to succeed despite MR configuration error, got: %v", err)
	}

	if !result.Success {
		t.Fatalf("Expected transflow to succeed despite MR configuration error")
	}

	// Verify MR step failed but didn't break the workflow
	var mrStepFound bool
	for _, step := range result.StepResults {
		if step.StepID == "mr" {
			mrStepFound = true
			if step.Success {
				t.Errorf("Expected MR step to fail due to configuration error")
			}
			if !strings.Contains(step.Message, "configuration invalid") {
				t.Errorf("Expected MR step message to mention configuration error, got: %s", step.Message)
			}
			break
		}
	}

	if !mrStepFound {
		t.Errorf("Expected MR step to be attempted even with invalid configuration")
	}

	// Verify no MR URL was set
	if result.MRURL != "" {
		t.Errorf("Expected no MR URL when configuration is invalid, got: %s", result.MRURL)
	}
}

// Note: MockGitOperations, MockRecipeExecutor, and MockBuildChecker are defined in runner_test.go
