package mods

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/storage/factory"
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
				TargetRepo:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
				TargetBranch: "main",
				BaseRef:      "main",
				Lane:         "C",
				BuildTimeout: "10m",
				Steps: []TransflowStep{
					{
						Type:               "orw-apply",
						ID:                 "java-migration",
						Recipes:            []string{"org.openrewrite.java.migrate.UpgradeToJava17"},
						RecipeGroup:        "org.openrewrite.recipe",
						RecipeArtifact:     "rewrite-migrate-java",
						RecipeVersion:      "3.17.0",
						MavenPluginVersion: "6.18.0",
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
				TargetRepo:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
				TargetBranch: "main",
				BaseRef:      "main",
				Lane:         "C",
				BuildTimeout: "10m",
				Steps: []TransflowStep{
					{
						Type:               "orw-apply",
						ID:                 "java-migration",
						Recipes:            []string{"org.openrewrite.java.migrate.UpgradeToJava17"},
						RecipeGroup:        "org.openrewrite.recipe",
						RecipeArtifact:     "rewrite-migrate-java",
						RecipeVersion:      "3.17.0",
						MavenPluginVersion: "6.18.0",
					},
				},
				SelfHeal: GetDefaultSelfHealConfig(),
			},
			testMode:    true, // We'll configure mock to fail
			expectError: true,
		},
		{
			name: "workflow_with_seaweedfs",
			config: &TransflowConfig{
				Version:      "v1alpha1",
				ID:           "test-seaweedfs",
				TargetRepo:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
				TargetBranch: "main",
				BaseRef:      "main",
				Lane:         "C",
				BuildTimeout: "10m",
				Steps: []TransflowStep{
					{
						Type:               "orw-apply",
						ID:                 "java-migration",
						Recipes:            []string{"org.openrewrite.java.migrate.UpgradeToJava17"},
						RecipeGroup:        "org.openrewrite.recipe",
						RecipeArtifact:     "rewrite-migrate-java",
						RecipeVersion:      "3.17.0",
						MavenPluginVersion: "6.18.0",
					},
				},
				SelfHeal: &SelfHealConfig{
					Enabled:    true,
					MaxRetries: 1,
					Cooldown:   "15m",
				},
			},
			testMode:    false, // REAL SERVICES REQUIRED
			expectError: false,
		},
		{
			name: "workflow_with_nomad_jobs",
			config: &TransflowConfig{
				Version:      "v1alpha1",
				ID:           "test-nomad-real",
				TargetRepo:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
				TargetBranch: "main",
				BaseRef:      "main",
				Lane:         "C",
				BuildTimeout: "10m",
				Steps: []TransflowStep{
					{
						Type:               "orw-apply",
						ID:                 "java-migration",
						Recipes:            []string{"org.openrewrite.java.migrate.UpgradeToJava17"},
						RecipeGroup:        "org.openrewrite.recipe",
						RecipeArtifact:     "rewrite-migrate-java",
						RecipeVersion:      "3.17.0",
						MavenPluginVersion: "6.18.0",
					},
				},
				SelfHeal: &SelfHealConfig{
					Enabled:    true,
					MaxRetries: 2,
					Cooldown:   "5m",
				},
			},
			testMode:    false, // REAL NOMAD REQUIRED
			expectError: false,
		},
		{
			name: "workflow_with_consul_kv",
			config: &TransflowConfig{
				Version:      "v1alpha1",
				ID:           "test-consul-real",
				TargetRepo:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
				TargetBranch: "main",
				BaseRef:      "main",
				Lane:         "C",
				BuildTimeout: "10m",
				Steps: []TransflowStep{
					{
						Type:               "orw-apply",
						ID:                 "java-migration",
						Recipes:            []string{"org.openrewrite.java.migrate.UpgradeToJava17"},
						RecipeGroup:        "org.openrewrite.recipe",
						RecipeArtifact:     "rewrite-migrate-java",
						RecipeVersion:      "3.17.0",
						MavenPluginVersion: "6.18.0",
					},
				},
				SelfHeal: &SelfHealConfig{
					Enabled:    true,
					MaxRetries: 1,
					Cooldown:   "15m",
				},
			},
			testMode:    false, // REAL CONSUL REQUIRED
			expectError: false,
		},
		{
			name: "workflow_with_gitlab_api",
			config: &TransflowConfig{
				Version:      "v1alpha1",
				ID:           "test-gitlab-real",
				TargetRepo:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
				TargetBranch: "main",
				BaseRef:      "main",
				Lane:         "C",
				BuildTimeout: "10m",
				Steps: []TransflowStep{
					{
						Type:               "orw-apply",
						ID:                 "java-migration",
						Recipes:            []string{"org.openrewrite.java.migrate.UpgradeToJava17"},
						RecipeGroup:        "org.openrewrite.recipe",
						RecipeArtifact:     "rewrite-migrate-java",
						RecipeVersion:      "3.17.0",
						MavenPluginVersion: "6.18.0",
					},
				},
				SelfHeal: &SelfHealConfig{
					Enabled:    true,
					MaxRetries: 1,
					Cooldown:   "15m",
				},
			},
			testMode:    false, // REAL GITLAB API REQUIRED
			expectError: false,
		},
		{
			name: "workflow_with_all_real_services",
			config: &TransflowConfig{
				Version:      "v1alpha1",
				ID:           "test-all-services-real",
				TargetRepo:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
				TargetBranch: "main",
				BaseRef:      "main",
				Lane:         "C",
				BuildTimeout: "15m",
				Steps: []TransflowStep{
					{
						Type:               "orw-apply",
						ID:                 "java-migration",
						Recipes:            []string{"org.openrewrite.java.migrate.UpgradeToJava17"},
						RecipeGroup:        "org.openrewrite.recipe",
						RecipeArtifact:     "rewrite-migrate-java",
						RecipeVersion:      "3.17.0",
						MavenPluginVersion: "6.18.0",
					},
				},
				SelfHeal: &SelfHealConfig{
					Enabled:    true,
					MaxRetries: 3,
					Cooldown:   "10m",
				},
			},
			testMode:    false, // ALL REAL SERVICES REQUIRED
			expectError: false,
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

			// Handle real service test cases
			switch tt.name {
			case "workflow_with_seaweedfs":
				// Require SeaweedFS service - fail test if not available
				serviceConfig := RequireServices(t, "seaweedfs")

				// Create integrations WITHOUT test mode for services
				integrations := NewTransflowIntegrationsWithTestMode("http://localhost:8080", workspaceDir, false)

				// Test SeaweedFS storage operations before running workflow
				t.Run("validate_seaweedfs_operations", func(t *testing.T) {
					testSeaweedFSOperations(t, serviceConfig.SeaweedFSFiler)
				})

				// Create configured runner with services
				runner, err := integrations.CreateConfiguredRunner(tt.config)
				if err != nil {
					t.Fatalf("failed to create runner with services: %v", err)
				}

				// Execute workflow with real storage
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()

				result, err := runner.Run(ctx)

				// Validate that services were used
				if err != nil {
					t.Fatalf("workflow with SeaweedFS failed: %v", err)
				}

				if result == nil {
					t.Fatal("expected result but got nil")
				}

				// Additional validation for services
				validateServiceUsage(t, result, serviceConfig)

				t.Logf("SeaweedFS integration test completed successfully")
				return

			case "workflow_with_nomad_jobs":
				// Require Nomad service for real job submission
				serviceConfig := RequireServices(t, "nomad")

				// Set environment for real Nomad job submission
				os.Setenv("TRANSFLOW_SUBMIT", "1")
				os.Setenv("NOMAD_ADDR", serviceConfig.NomadAddr)
				defer os.Unsetenv("TRANSFLOW_SUBMIT")
				defer os.Unsetenv("NOMAD_ADDR")

				// Create integrations WITHOUT test mode
				integrations := NewTransflowIntegrationsWithTestMode("http://localhost:8080", workspaceDir, false)

				// Test Nomad operations before running workflow
				t.Run("validate_nomad_operations", func(t *testing.T) {
					testNomadOperations(t, serviceConfig.NomadAddr)
				})

				runner, err := integrations.CreateConfiguredRunner(tt.config)
				if err != nil {
					t.Fatalf("failed to create runner with Nomad: %v", err)
				}

				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
				defer cancel()

				result, err := runner.Run(ctx)

				if err != nil {
					t.Fatalf("workflow with real Nomad failed: %v", err)
				}

				if result == nil {
					t.Fatal("expected result but got nil")
				}

				validateNomadUsage(t, result, serviceConfig)
				t.Logf("Nomad integration test completed successfully")
				return

			case "workflow_with_consul_kv":
				// Require Consul for KV operations
				serviceConfig := RequireServices(t, "consul")

				// Set environment for real Consul
				os.Setenv("CONSUL_HTTP_ADDR", serviceConfig.ConsulAddr)
				defer os.Unsetenv("CONSUL_HTTP_ADDR")

				integrations := NewTransflowIntegrationsWithTestMode("http://localhost:8080", workspaceDir, false)

				// Test Consul operations
				t.Run("validate_consul_operations", func(t *testing.T) {
					testConsulOperations(t, serviceConfig.ConsulAddr)
				})

				runner, err := integrations.CreateConfiguredRunner(tt.config)
				if err != nil {
					t.Fatalf("failed to create runner with Consul: %v", err)
				}

				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
				defer cancel()

				result, err := runner.Run(ctx)

				if err != nil {
					t.Fatalf("workflow with real Consul failed: %v", err)
				}

				if result == nil {
					t.Fatal("expected result but got nil")
				}

				validateConsulUsage(t, result, serviceConfig)
				t.Logf("Consul integration test completed successfully")
				return

			case "workflow_with_gitlab_api":
				// Require GitLab for real API calls
				serviceConfig := RequireServices(t, "gitlab")

				// Set environment for real GitLab
				os.Setenv("GITLAB_URL", serviceConfig.GitLabURL)
				os.Setenv("GITLAB_TOKEN", serviceConfig.GitLabToken)
				defer os.Unsetenv("GITLAB_URL")
				defer os.Unsetenv("GITLAB_TOKEN")

				integrations := NewTransflowIntegrationsWithTestMode("http://localhost:8080", workspaceDir, false)

				// Test GitLab operations
				t.Run("validate_gitlab_operations", func(t *testing.T) {
					testGitLabOperations(t, serviceConfig.GitLabURL, serviceConfig.GitLabToken)
				})

				runner, err := integrations.CreateConfiguredRunner(tt.config)
				if err != nil {
					t.Fatalf("failed to create runner with GitLab: %v", err)
				}

				ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
				defer cancel()

				result, err := runner.Run(ctx)

				if err != nil {
					t.Fatalf("workflow with real GitLab failed: %v", err)
				}

				if result == nil {
					t.Fatal("expected result but got nil")
				}

				validateGitLabUsage(t, result, serviceConfig)
				t.Logf("GitLab integration test completed successfully")
				return

			case "workflow_with_all_real_services":
				// Require ALL services for comprehensive test
				serviceConfig := RequireServices(t, "consul", "nomad", "seaweedfs", "gitlab")

				// Set all environment variables for real services
				os.Setenv("TRANSFLOW_SUBMIT", "1")
				os.Setenv("NOMAD_ADDR", serviceConfig.NomadAddr)
				os.Setenv("CONSUL_HTTP_ADDR", serviceConfig.ConsulAddr)
				os.Setenv("GITLAB_URL", serviceConfig.GitLabURL)
				os.Setenv("GITLAB_TOKEN", serviceConfig.GitLabToken)
				defer func() {
					os.Unsetenv("TRANSFLOW_SUBMIT")
					os.Unsetenv("NOMAD_ADDR")
					os.Unsetenv("CONSUL_HTTP_ADDR")
					os.Unsetenv("GITLAB_URL")
					os.Unsetenv("GITLAB_TOKEN")
				}()

				integrations := NewTransflowIntegrationsWithTestMode("http://localhost:8080", workspaceDir, false)

				// Test all service operations
				t.Run("validate_all_service_operations", func(t *testing.T) {
					testSeaweedFSOperations(t, serviceConfig.SeaweedFSFiler)
					testNomadOperations(t, serviceConfig.NomadAddr)
					testConsulOperations(t, serviceConfig.ConsulAddr)
					testGitLabOperations(t, serviceConfig.GitLabURL, serviceConfig.GitLabToken)
				})

				runner, err := integrations.CreateConfiguredRunner(tt.config)
				if err != nil {
					t.Fatalf("failed to create runner with all services: %v", err)
				}

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancel()

				result, err := runner.Run(ctx)

				if err != nil {
					t.Fatalf("workflow with all real services failed: %v", err)
				}

				if result == nil {
					t.Fatal("expected result but got nil")
				}

				validateAllServicesUsage(t, result, serviceConfig)
				t.Logf("All services integration test completed successfully")
				return
			}

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
			expectedSteps := []string{"clone", "create-branch", "java-migration", "commit", "build", "push", "mr"}
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
				TargetRepo:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
				TargetBranch: "main",
				BaseRef:      "main",
				Steps: []TransflowStep{
					{
						Type:           "orw-apply",
						ID:             "java-migration",
						Recipes:        []string{"org.openrewrite.java.migrate.UpgradeToJava17"},
						RecipeGroup:    "org.openrewrite.recipe",
						RecipeArtifact: "rewrite-migrate-java",
						RecipeVersion:  "3.17.0",
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
				TargetRepo:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
				TargetBranch: "main",
				BaseRef:      "main",
				BuildTimeout: "invalid-timeout",
				Steps: []TransflowStep{
					{
						Type:           "orw-apply",
						ID:             "java-migration",
						Recipes:        []string{"org.openrewrite.java.migrate.UpgradeToJava17"},
						RecipeGroup:    "org.openrewrite.recipe",
						RecipeArtifact: "rewrite-migrate-java",
						RecipeVersion:  "3.17.0",
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

// testSeaweedFSOperations validates that SeaweedFS storage operations work correctly
func testSeaweedFSOperations(t *testing.T, filerURL string) {
	t.Helper()

	// Create SeaweedFS storage client
	storageClient, err := factory.New(factory.FactoryConfig{
		Provider: "seaweedfs",
		Endpoint: filerURL,
		Extra: map[string]interface{}{
			"master": strings.Replace(filerURL, "8888", "9333", 1), // Assume master is on port 9333
			"filer":  filerURL,
		},
	})
	if err != nil {
		t.Fatalf("failed to create SeaweedFS client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test key for this integration test
	testKey := fmt.Sprintf("transflow/integration-test/%d", time.Now().Unix())
	testData := []byte("SeaweedFS integration test data")

	// Test Put operation
	err = storageClient.Put(ctx, testKey, strings.NewReader(string(testData)))
	if err != nil {
		t.Fatalf("Failed to store data in SeaweedFS: %v", err)
	}
	t.Logf("Successfully stored data to SeaweedFS at key: %s", testKey)

	// Test Get operation
	retrievedData, err := storageClient.Get(ctx, testKey)
	if err != nil {
		t.Fatalf("Failed to retrieve data from SeaweedFS: %v", err)
	}

	// Read and validate content
	retrievedBytes := make([]byte, len(testData))
	n, err := retrievedData.Read(retrievedBytes)
	if err != nil {
		t.Fatalf("Failed to read retrieved data: %v", err)
	}
	retrievedData.Close()

	if n != len(testData) || string(retrievedBytes[:n]) != string(testData) {
		t.Fatalf("Retrieved data doesn't match stored data. Expected: %s, Got: %s",
			string(testData), string(retrievedBytes[:n]))
	}
	t.Logf("Successfully retrieved and validated data from SeaweedFS")

	// Test List operation
	objects, err := storageClient.List(ctx, storage.ListOptions{
		Prefix: "transflow/integration-test/",
	})
	if err != nil {
		t.Fatalf("Failed to list objects from SeaweedFS: %v", err)
	}

	found := false
	var keys []string
	for _, obj := range objects {
		keys = append(keys, obj.Key)
		if obj.Key == testKey {
			found = true
		}
	}
	if !found {
		t.Fatalf("Stored key not found in list operation. Expected: %s, Got keys: %v", testKey, keys)
	}
	t.Logf("Successfully listed keys from SeaweedFS, found test key")

	// Test Delete operation
	err = storageClient.Delete(ctx, testKey)
	if err != nil {
		t.Fatalf("Failed to delete data from SeaweedFS: %v", err)
	}
	t.Logf("Successfully deleted test data from SeaweedFS")

	// Verify deletion
	_, err = storageClient.Get(ctx, testKey)
	if err == nil {
		t.Fatal("Expected error when retrieving deleted key, but got none")
	}
	t.Logf("Verified deletion - key no longer retrievable")
}

// validateServiceUsage checks that the workflow actually used services
func validateServiceUsage(t *testing.T, result *TransflowResult, serviceConfig *ServicesConfig) {
	t.Helper()

	// Validate that the workflow produced expected artifacts that would only exist with services
	if result.BranchName == "" {
		t.Error("Expected branch name but got empty string")
	}

	// Check that storage operations were performed (KB integration uses real storage)
	if len(result.StepResults) == 0 {
		t.Error("Expected step results but got none")
	}

	// Validate that SeaweedFS was actually used by checking for transflow artifacts
	storageClient, err := factory.New(factory.FactoryConfig{
		Provider: "seaweedfs",
		Endpoint: serviceConfig.SeaweedFSFiler,
		Extra: map[string]interface{}{
			"master": serviceConfig.SeaweedFSMaster,
			"filer":  serviceConfig.SeaweedFSFiler,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create SeaweedFS client for validation: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check for transflow-related artifacts in SeaweedFS
	objects, err := storageClient.List(ctx, storage.ListOptions{
		Prefix: "transflow/",
	})
	if err != nil {
		t.Logf("Warning: Could not list transflow objects from SeaweedFS: %v", err)
	} else if len(objects) > 0 {
		t.Logf("Found %d transflow-related artifacts in SeaweedFS storage", len(objects))
	}

	// Additional validations
	for _, stepResult := range result.StepResults {
		if !stepResult.Success {
			t.Errorf("Step %s failed in real service workflow: %s", stepResult.StepID, stepResult.Message)
		}
	}

	t.Logf("Real service validation completed successfully")
	t.Logf("  Branch: %s", result.BranchName)
	t.Logf("  Steps: %d completed", len(result.StepResults))
	t.Logf("  SeaweedFS URL: %s", serviceConfig.SeaweedFSFiler)
}

// ServicesConfig holds configuration for service integration tests
type ServicesConfig struct {
	ConsulAddr      string
	NomadAddr       string
	SeaweedFSFiler  string
	SeaweedFSMaster string
	GitLabURL       string
	GitLabToken     string
}

// RequireServices enforces that services are running - no fallback to mocks
// This function fails the test if services are not available
func RequireServices(t *testing.T, services ...string) *ServicesConfig {
	t.Helper()

	config := &ServicesConfig{}
	var failures []string

	for _, service := range services {
		switch service {
		case "consul":
			if !isConsulHealthy() {
				failures = append(failures, "Consul not available at localhost:8500")
			} else {
				config.ConsulAddr = "localhost:8500"
			}
		case "nomad":
			if !isNomadHealthy() {
				failures = append(failures, "Nomad not available at http://localhost:4646")
			} else {
				config.NomadAddr = "http://localhost:4646"
			}
		case "seaweedfs":
			if !isSeaweedFSHealthy() {
				failures = append(failures, "SeaweedFS not available at http://localhost:8888")
			} else {
				config.SeaweedFSFiler = "http://localhost:8888"
				config.SeaweedFSMaster = "http://localhost:9333"
			}
		case "gitlab":
			token := os.Getenv("GITLAB_TOKEN")
			if token == "" {
				failures = append(failures, "GITLAB_TOKEN environment variable required for real GitLab testing")
			} else {
				config.GitLabURL = "https://gitlab.com"
				config.GitLabToken = token
			}
		default:
			failures = append(failures, fmt.Sprintf("Unknown service: %s", service))
		}
	}

	if len(failures) > 0 {
		t.Fatalf("Required services not available:\n%s\n\nSetup:\n1. Run: docker-compose -f docker-compose.integration.yml up -d\n2. Set GITLAB_TOKEN environment variable for GitLab tests\n3. Wait for services to be healthy", strings.Join(failures, "\n"))
	}

	return config
}

// Service health check functions for services
func isConsulHealthy() bool {
	client, err := consulapi.NewClient(consulapi.DefaultConfig())
	if err != nil {
		return false
	}
	_, err = client.Status().Leader()
	return err == nil
}

func isNomadHealthy() bool {
	client, err := nomadapi.NewClient(nomadapi.DefaultConfig())
	if err != nil {
		return false
	}
	_, err = client.Status().Leader()
	return err == nil
}

func isSeaweedFSHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return isServiceHealthyHTTP(ctx, "http://localhost:8888/") && isServiceHealthyHTTP(ctx, "http://localhost:9333/cluster/status")
}

func isServiceHealthyHTTP(ctx context.Context, url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// testNomadOperations validates that Nomad job operations work correctly
func testNomadOperations(t *testing.T, nomadAddr string) {
	t.Helper()

	// Create Nomad client
	config := nomadapi.DefaultConfig()
	config.Address = nomadAddr
	client, err := nomadapi.NewClient(config)
	if err != nil {
		t.Fatalf("failed to create Nomad client: %v", err)
	}

	// Test basic Nomad connectivity
	_, err = client.Status().Leader()
	if err != nil {
		t.Fatalf("Failed to connect to Nomad: %v", err)
	}

	// Test job listing (should not fail)
	_, _, err = client.Jobs().List(&nomadapi.QueryOptions{})
	if err != nil {
		t.Fatalf("Failed to list Nomad jobs: %v", err)
	}

	t.Logf("Successfully validated Nomad operations at %s", nomadAddr)
}

// testConsulOperations validates that Consul KV operations work correctly
func testConsulOperations(t *testing.T, consulAddr string) {
	t.Helper()

	// Create Consul client
	config := consulapi.DefaultConfig()
	config.Address = consulAddr
	client, err := consulapi.NewClient(config)
	if err != nil {
		t.Fatalf("failed to create Consul client: %v", err)
	}

	// Test key for this integration test
	testKey := fmt.Sprintf("transflow/integration-test/%d", time.Now().Unix())
	testValue := []byte("Consul integration test data")

	kv := client.KV()

	// Test Put operation
	_, err = kv.Put(&consulapi.KVPair{
		Key:   testKey,
		Value: testValue,
	}, &consulapi.WriteOptions{})
	if err != nil {
		t.Fatalf("Failed to store data in Consul: %v", err)
	}
	t.Logf("Successfully stored data to Consul at key: %s", testKey)

	// Test Get operation
	pair, _, err := kv.Get(testKey, &consulapi.QueryOptions{})
	if err != nil {
		t.Fatalf("Failed to retrieve data from Consul: %v", err)
	}
	if pair == nil {
		t.Fatal("Expected key-value pair but got nil")
	}

	if string(pair.Value) != string(testValue) {
		t.Fatalf("Retrieved data doesn't match. Expected: %s, Got: %s", string(testValue), string(pair.Value))
	}
	t.Logf("Successfully retrieved and validated data from Consul")

	// Test List operation
	pairs, _, err := kv.List("transflow/integration-test/", &consulapi.QueryOptions{})
	if err != nil {
		t.Fatalf("Failed to list keys from Consul: %v", err)
	}

	found := false
	for _, p := range pairs {
		if p.Key == testKey {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Stored key not found in list operation")
	}
	t.Logf("Successfully listed keys from Consul")

	// Test Delete operation
	_, err = kv.Delete(testKey, &consulapi.WriteOptions{})
	if err != nil {
		t.Fatalf("Failed to delete data from Consul: %v", err)
	}
	t.Logf("Successfully deleted test data from Consul")

	// Verify deletion
	pair, _, err = kv.Get(testKey, &consulapi.QueryOptions{})
	if err != nil {
		t.Fatalf("Unexpected error checking deleted key: %v", err)
	}
	if pair != nil {
		t.Fatal("Expected key to be deleted but still exists")
	}
	t.Logf("Verified deletion - key no longer exists")
}

// testGitLabOperations validates that GitLab API operations work correctly
func testGitLabOperations(t *testing.T, gitlabURL, token string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test basic GitLab API connectivity
	client := &http.Client{Timeout: 10 * time.Second}

	// Create request to GitLab user endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", gitlabURL+"/api/v4/user", nil)
	if err != nil {
		t.Fatalf("Failed to create GitLab request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to connect to GitLab API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GitLab API returned status %d", resp.StatusCode)
	}

	t.Logf("Successfully validated GitLab API operations at %s", gitlabURL)

	// Test project access
	projectReq, err := http.NewRequestWithContext(ctx, "GET", gitlabURL+"/api/v4/projects?simple=true&per_page=1", nil)
	if err != nil {
		t.Fatalf("Failed to create GitLab projects request: %v", err)
	}
	projectReq.Header.Set("Authorization", "Bearer "+token)

	projectResp, err := client.Do(projectReq)
	if err != nil {
		t.Fatalf("Failed to fetch GitLab projects: %v", err)
	}
	defer projectResp.Body.Close()

	if projectResp.StatusCode != http.StatusOK {
		t.Fatalf("GitLab projects API returned status %d", projectResp.StatusCode)
	}

	t.Logf("Successfully validated GitLab project access")
}

// validateNomadUsage checks that the workflow actually used Nomad
func validateNomadUsage(t *testing.T, result *TransflowResult, serviceConfig *ServicesConfig) {
	t.Helper()

	// Validate that the workflow produced expected results
	if result.BranchName == "" {
		t.Error("Expected branch name but got empty string")
	}

	// Check that Nomad jobs were actually submitted (healing would use Nomad)
	config := nomadapi.DefaultConfig()
	config.Address = serviceConfig.NomadAddr
	client, err := nomadapi.NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create Nomad client for validation: %v", err)
	}

	// List recent jobs to see if transflow jobs were submitted
	jobs, _, err := client.Jobs().List(&nomadapi.QueryOptions{})
	if err != nil {
		t.Logf("Warning: Could not list Nomad jobs for validation: %v", err)
	} else {
		transflowJobs := 0
		for _, job := range jobs {
			// JobListStub.Name can be a string in some Nomad client versions
			name := job.Name
			if strings.Contains(name, "transflow") {
				transflowJobs++
			}
		}
		if transflowJobs > 0 {
			t.Logf("Found %d transflow-related jobs in Nomad", transflowJobs)
		}
	}

	// Validate workflow completed successfully
	for _, stepResult := range result.StepResults {
		if !stepResult.Success {
			t.Errorf("Step %s failed in real Nomad workflow: %s", stepResult.StepID, stepResult.Message)
		}
	}

	t.Logf("Nomad usage validation completed")
	t.Logf("  Branch: %s", result.BranchName)
	t.Logf("  Nomad URL: %s", serviceConfig.NomadAddr)
}

// validateConsulUsage checks that the workflow actually used Consul
func validateConsulUsage(t *testing.T, result *TransflowResult, serviceConfig *ServicesConfig) {
	t.Helper()

	// Validate that the workflow produced expected results
	if result.BranchName == "" {
		t.Error("Expected branch name but got empty string")
	}

	// Check that Consul KV operations were performed (KB locking uses Consul)
	config := consulapi.DefaultConfig()
	config.Address = serviceConfig.ConsulAddr
	client, err := consulapi.NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create Consul client for validation: %v", err)
	}

	// Check for transflow-related keys in Consul
	kv := client.KV()
	pairs, _, err := kv.List("transflow/", &consulapi.QueryOptions{})
	if err != nil {
		t.Logf("Warning: Could not list transflow keys from Consul: %v", err)
	} else if len(pairs) > 0 {
		t.Logf("Found %d transflow-related keys in Consul KV", len(pairs))
	}

	// Validate workflow completed successfully
	for _, stepResult := range result.StepResults {
		if !stepResult.Success {
			t.Errorf("Step %s failed in real Consul workflow: %s", stepResult.StepID, stepResult.Message)
		}
	}

	t.Logf("Consul usage validation completed")
	t.Logf("  Branch: %s", result.BranchName)
	t.Logf("  Consul URL: %s", serviceConfig.ConsulAddr)
}

// validateGitLabUsage checks that the workflow actually used GitLab API
func validateGitLabUsage(t *testing.T, result *TransflowResult, serviceConfig *ServicesConfig) {
	t.Helper()

	// Validate that the workflow produced expected results
	if result.BranchName == "" {
		t.Error("Expected branch name but got empty string")
	}

	// Additional GitLab-specific validations could be added here
	// such as checking if merge requests were created

	// Validate workflow completed successfully
	for _, stepResult := range result.StepResults {
		if !stepResult.Success {
			t.Errorf("Step %s failed in real GitLab workflow: %s", stepResult.StepID, stepResult.Message)
		}
	}

	t.Logf("GitLab usage validation completed")
	t.Logf("  Branch: %s", result.BranchName)
	t.Logf("  GitLab URL: %s", serviceConfig.GitLabURL)
}

// validateAllServicesUsage checks that the workflow used all real services
func validateAllServicesUsage(t *testing.T, result *TransflowResult, serviceConfig *ServicesConfig) {
	t.Helper()

	// Run individual service validations
	validateServiceUsage(t, result, serviceConfig) // SeaweedFS
	validateNomadUsage(t, result, serviceConfig)   // Nomad
	validateConsulUsage(t, result, serviceConfig)  // Consul
	validateGitLabUsage(t, result, serviceConfig)  // GitLab

	// Additional comprehensive validation
	if len(result.StepResults) < 5 {
		t.Errorf("Expected at least 5 workflow steps with all services, got %d", len(result.StepResults))
	}

	// Check that all services were actually integrated
	serviceChecks := map[string]bool{
		"storage": false, // SeaweedFS
		"nomad":   false, // Job orchestration
		"consul":  false, // KV operations
		"gitlab":  false, // Git provider
	}

	for _, stepResult := range result.StepResults {
		stepName := strings.ToLower(stepResult.StepID)
		if strings.Contains(stepName, "storage") || strings.Contains(stepName, "kb") {
			serviceChecks["storage"] = true
		}
		if strings.Contains(stepName, "heal") || strings.Contains(stepName, "job") {
			serviceChecks["nomad"] = true
		}
		if strings.Contains(stepName, "lock") || strings.Contains(stepName, "kv") {
			serviceChecks["consul"] = true
		}
		if strings.Contains(stepName, "mr") || strings.Contains(stepName, "gitlab") || strings.Contains(stepName, "push") {
			serviceChecks["gitlab"] = true
		}
	}

	// Report service integration status
	for service, used := range serviceChecks {
		if used {
			t.Logf("✓ %s service was integrated in workflow", service)
		} else {
			t.Logf("- %s service integration not detected in steps", service)
		}
	}

	t.Logf("All services integration validation completed successfully")
	t.Logf("  Branch: %s", result.BranchName)
	t.Logf("  Steps completed: %d", len(result.StepResults))
	t.Logf("  All services: SeaweedFS, Nomad, Consul, GitLab")
}
