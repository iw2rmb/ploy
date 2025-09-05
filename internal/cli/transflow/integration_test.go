package transflow

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
)

func TestTransflowEndToEndIntegration(t *testing.T) {
	// Skip integration tests in short mode
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name        string
		config      *TransflowConfig
		testMode    bool
		expectError bool
	}{
		{
			name: "successful_workflow_with_mocks",
			config: &TransflowConfig{
				Version:      "v1alpha1",
				ID:           "test-integration-success",
				TargetRepo:   "https://github.com/iw2rmb/ploy.git",
				TargetBranch: "main",
				BaseRef:      "main",
				Lane:         "C",
				BuildTimeout: "10m",
				Steps: []TransflowStep{
					{
						Type:    "recipe",
						ID:      "test-recipe",
						Engine:  "openrewrite",
						Recipes: []string{"org.openrewrite.java.format.AutoFormat"},
					},
				},
				SelfHeal: GetDefaultSelfHealConfig(),
			},
			testMode:    true,
			expectError: false,
		},
		{
			name: "workflow_with_build_failure",
			config: &TransflowConfig{
				Version:      "v1alpha1",
				ID:           "test-integration-fail",
				TargetRepo:   "https://github.com/iw2rmb/ploy.git",
				TargetBranch: "main",
				BaseRef:      "main",
				Lane:         "C",
				BuildTimeout: "10m",
				Steps: []TransflowStep{
					{
						Type:    "recipe",
						ID:      "test-recipe",
						Engine:  "openrewrite",
						Recipes: []string{"org.openrewrite.java.format.AutoFormat"},
					},
				},
				SelfHeal: GetDefaultSelfHealConfig(),
			},
			testMode:    true, // We'll configure mock to fail
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary workspace
			workspaceDir, err := os.MkdirTemp("", "transflow-integration-test-*")
			if err != nil {
				t.Fatalf("failed to create temp directory: %v", err)
			}
			defer os.RemoveAll(workspaceDir)

			// Create integrations with test mode
			integrations := NewTransflowIntegrationsWithTestMode("http://localhost:8080", workspaceDir, tt.testMode)
			
			// For build failure test, override with failing mock
			if tt.name == "workflow_with_build_failure" {
				integrations.TestMode = true
				failingIntegrations := &TransflowIntegrations{
					ControllerURL: integrations.ControllerURL,
					WorkDir:       integrations.WorkDir,
					TestMode:      true,
				}
				// This will test the failure path
				runner, err := failingIntegrations.CreateConfiguredRunner(tt.config)
				if err != nil {
					t.Fatalf("failed to create runner: %v", err)
				}
				
				// Override with failing build checker
				runner.SetBuildChecker(NewTestModeBuildChecker(true))
				
				// Execute workflow
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				
				result, err := runner.Run(ctx)
				if !tt.expectError {
					t.Errorf("expected error but got none, result: %v", result)
				}
				if err == nil {
					t.Error("expected error but workflow succeeded")
				}
				return
			}

			// Create configured runner
			runner, err := integrations.CreateConfiguredRunner(tt.config)
			if err != nil {
				t.Fatalf("failed to create runner: %v", err)
			}

			// Set environment for GitLab integration testing
			os.Setenv("GITLAB_TOKEN", "test-token-for-integration")
			defer os.Unsetenv("GITLAB_TOKEN")

			// Execute workflow
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			result, err := runner.Run(ctx)

			// Verify results
			if tt.expectError {
				if err == nil {
					t.Error("expected error but workflow succeeded")
				}
				return
			}

			if err != nil {
				t.Fatalf("workflow failed unexpectedly: %v", err)
			}

			if result == nil {
				t.Fatal("expected result but got nil")
			}

			// Verify workflow components
			if result.BranchName == "" {
				t.Error("expected branch name but got empty string")
			}

			if result.BuildVersion == "" {
				t.Error("expected build version but got empty string")  
			}

			// Check that all expected steps completed
			expectedSteps := []string{"clone", "create-branch", "test-recipe", "commit", "build", "push", "mr"}
			if len(result.StepResults) < len(expectedSteps) {
				t.Errorf("expected at least %d steps but got %d", len(expectedSteps), len(result.StepResults))
			}

			// Verify all steps succeeded
			for _, stepResult := range result.StepResults {
				if !stepResult.Success {
					t.Errorf("step %s failed: %s", stepResult.StepID, stepResult.Message)
				}
			}

			// Verify workspace was used
			if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
				t.Error("workspace directory was not created")
			}

			t.Logf("Workflow completed successfully:")
			t.Logf("  Branch: %s", result.BranchName)
			t.Logf("  Build: %s", result.BuildVersion)
			t.Logf("  Steps: %d completed", len(result.StepResults))
		})
	}
}

func TestTransflowConfigurationValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *TransflowConfig
		expectError bool
	}{
		{
			name: "valid_minimal_config",
			config: &TransflowConfig{
				Version:      "v1alpha1",
				ID:           "test-valid",
				TargetRepo:   "https://github.com/test/repo.git",
				TargetBranch: "main",
				BaseRef:      "main",
				Steps: []TransflowStep{
					{
						Type:    "recipe",
						ID:      "test",
						Engine:  "openrewrite",
						Recipes: []string{"org.test.Recipe"},
					},
				},
			},
			expectError: false,
		},
		{
			name: "missing_required_fields",
			config: &TransflowConfig{
				Version: "v1alpha1",
				// Missing ID, TargetRepo, etc.
			},
			expectError: true,
		},
		{
			name: "invalid_build_timeout",
			config: &TransflowConfig{
				Version:      "v1alpha1",
				ID:           "test-timeout",
				TargetRepo:   "https://github.com/test/repo.git",
				TargetBranch: "main",
				BaseRef:      "main",
				BuildTimeout: "invalid-timeout",
				Steps: []TransflowStep{
					{
						Type:    "recipe",
						ID:      "test",
						Engine:  "openrewrite",  
						Recipes: []string{"org.test.Recipe"},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			
			if tt.expectError {
				if err == nil {
					t.Error("expected validation error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("expected no validation error but got: %v", err)
				}
			}
		})
	}
}

func TestTransflowIntegrationsFactory(t *testing.T) {
	workspaceDir, err := os.MkdirTemp("", "transflow-factory-test-*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(workspaceDir)

	t.Run("production_mode_integrations", func(t *testing.T) {
		integrations := NewTransflowIntegrationsWithTestMode("http://localhost:8080", workspaceDir, false)
		
		if integrations.TestMode {
			t.Error("expected TestMode to be false")
		}

		// Test that production implementations are created
		gitOps := integrations.CreateGitOperations()
		if gitOps == nil {
			t.Error("expected git operations but got nil")
		}

		recipeExec := integrations.CreateRecipeExecutor()
		if recipeExec == nil {
			t.Error("expected recipe executor but got nil")
		}

		buildChecker := integrations.CreateBuildChecker()
		if buildChecker == nil {
			t.Error("expected build checker but got nil")
		}

		gitProvider := integrations.CreateGitProvider()
		if gitProvider == nil {
			t.Error("expected git provider but got nil")
		}
	})

	t.Run("test_mode_integrations", func(t *testing.T) {
		integrations := NewTransflowIntegrationsWithTestMode("http://localhost:8080", workspaceDir, true)
		
		if !integrations.TestMode {
			t.Error("expected TestMode to be true")
		}

		// Test that mock implementations are created for testable components
		buildChecker := integrations.CreateBuildChecker()
		if buildChecker == nil {
			t.Error("expected build checker but got nil")
		}
		
		// Verify it's a mock by testing behavior
		ctx := context.Background()
		result, err := buildChecker.CheckBuild(ctx, common.DeployConfig{
			App:         "test-app",
			Lane:        "C",
			Environment: "dev",
		})
		
		if err != nil {
			t.Errorf("mock build checker should not fail: %v", err)
		}
		
		if result == nil || !result.Success {
			t.Error("mock build checker should return successful result")
		}

		// Test mock git provider
		gitProvider := integrations.CreateGitProvider()
		if gitProvider == nil {
			t.Error("expected git provider but got nil")
		}
		
		// Verify it's a mock
		if err := gitProvider.ValidateConfiguration(); err != nil {
			t.Errorf("mock git provider validation should succeed: %v", err)
		}
	})
}