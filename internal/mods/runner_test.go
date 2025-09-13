package mods

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/stretchr/testify/assert"
)

// Mock interfaces for testing are now in mocks.go

func TestModRunner_Run(t *testing.T) {
	// Stub external integrations for all subtests
	oldSubmit := submitAndWaitTerminal
	oldDL := downloadToFileFn
	oldHas := hasRepoChangesFn
	oldVDP := validateDiffPathsFn
	oldVUD := validateUnifiedDiffFn
	oldAD := applyUnifiedDiffFn
	baseValidate := func(string) error { return nil }
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
	// Provide exec ID so diff fetch proceeds
	_ = os.Setenv("PLOY_MODS_EXECUTION_ID", "t-runner")
	defer func() { _ = os.Unsetenv("PLOY_MODS_EXECUTION_ID") }()

	defer func() {
		validateJob = baseValidate
		submitAndWaitTerminal = oldSubmit
		downloadToFileFn = oldDL
		hasRepoChangesFn = oldHas
		validateDiffPathsFn = oldVDP
		validateUnifiedDiffFn = oldVUD
		applyUnifiedDiffFn = oldAD
	}()

	tests := []struct {
		name           string
		config         *ModConfig
		setupMocks     func(*MockGitOperations, *MockRecipeExecutor, *MockBuildChecker)
		expectError    bool
		expectedErrMsg string
		verifyMocks    func(*testing.T, *MockGitOperations, *MockRecipeExecutor, *MockBuildChecker)
	}{
		{
			name: "successful complete workflow",
			config: &ModConfig{
				ID:           "test-workflow",
				TargetRepo:   "https://github.com/org/project",
				BaseRef:      "refs/heads/main",
				BuildTimeout: "10m",
				Steps: []ModStep{
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
			},
			setupMocks: func(git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				// All operations succeed
			},
			expectError: false,
			verifyMocks: func(t *testing.T, git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				assert.True(t, git.CloneCalled, "CloneRepository should be called")
				assert.True(t, git.CreateBranchCalled, "CreateBranchAndCheckout should be called")
				assert.True(t, git.CommitCalled, "CommitChanges should be called")
				assert.True(t, build.BuildCalled, "CheckBuild should be called")
				assert.True(t, git.PushCalled, "PushBranch should be called")

				// Verify parameters
				assert.Equal(t, "https://github.com/org/project", git.CloneRepo)
				assert.Equal(t, "refs/heads/main", git.CloneBranch)
				assert.Contains(t, git.BranchName, "workflow/test-workflow/")
				// Commit message may be from apply(diff) or later commit step; just ensure a commit was attempted
				assert.NotEqual(t, "", git.CommitMessage)
				assert.Contains(t, build.BuildConfig.App, "tfw-test-workflow-")
				assert.Equal(t, "https://github.com/org/project", git.PushRemoteURL)
			},
		},
		{
			name: "git clone failure",
			config: &ModConfig{
				ID:         "test-workflow",
				TargetRepo: "https://github.com/org/project",
				BaseRef:    "refs/heads/main",
				Steps: []ModStep{
					{Type: "orw-apply", ID: "java-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}, RecipeGroup: "org.openrewrite.recipe", RecipeArtifact: "rewrite-migrate-java", RecipeVersion: "3.17.0", MavenPluginVersion: "6.18.0"},
				},
			},
			setupMocks: func(git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				git.CloneError = errors.New("clone failed: repository not found")
			},
			expectError:    true,
			expectedErrMsg: "failed to clone repository",
			verifyMocks: func(t *testing.T, git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				assert.True(t, git.CloneCalled)
				assert.False(t, git.CreateBranchCalled)
				assert.False(t, build.BuildCalled)
				assert.False(t, git.PushCalled)
			},
		},
		{
			name: "orw-apply validation failure",
			config: &ModConfig{
				ID:         "test-workflow",
				TargetRepo: "https://github.com/org/project",
				BaseRef:    "refs/heads/main",
				Steps: []ModStep{
					{Type: "orw-apply", ID: "java-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}, RecipeGroup: "org.openrewrite.recipe", RecipeArtifact: "rewrite-migrate-java", RecipeVersion: "3.17.0", MavenPluginVersion: "6.18.0"},
				},
			},
			setupMocks: func(git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				// Force HCL validation failure
				validateJob = func(string) error { return fmt.Errorf("job parse/validate failed: bad HCL") }
			},
			expectError:    true,
			expectedErrMsg: "orw-apply HCL validation failed",
			verifyMocks: func(t *testing.T, git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				assert.True(t, git.CloneCalled)
				assert.True(t, git.CreateBranchCalled)
				assert.False(t, git.CommitCalled)
				assert.False(t, build.BuildCalled)
				assert.False(t, git.PushCalled)
				// restore for following tests
				validateJob = baseValidate
			},
		},
		{
			name: "build check failure",
			config: &ModConfig{
				ID:         "test-workflow",
				TargetRepo: "https://github.com/org/project",
				BaseRef:    "refs/heads/main",
				Steps: []ModStep{
					{Type: "orw-apply", ID: "java-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}, RecipeGroup: "org.openrewrite.recipe", RecipeArtifact: "rewrite-migrate-java", RecipeVersion: "3.17.0", MavenPluginVersion: "6.18.0"},
				},
			},
			setupMocks: func(git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				build.BuildError = errors.New("build failed: compilation errors")
				build.BuildResult = &common.DeployResult{
					Success: false,
					Message: "Build failed with compilation errors",
				}
			},
			expectError:    true,
			expectedErrMsg: "build gate failed",
			verifyMocks: func(t *testing.T, git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				assert.True(t, git.CloneCalled)
				assert.True(t, git.CreateBranchCalled)
				assert.True(t, git.CommitCalled)
				assert.True(t, build.BuildCalled)
				assert.False(t, git.PushCalled) // Should not push on build failure
			},
		},
		{
			name: "push failure",
			config: &ModConfig{
				ID:         "test-workflow",
				TargetRepo: "https://github.com/org/project",
				BaseRef:    "refs/heads/main",
				Steps: []ModStep{
					{Type: "orw-apply", ID: "java-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}, RecipeGroup: "org.openrewrite.recipe", RecipeArtifact: "rewrite-migrate-java", RecipeVersion: "3.17.0", MavenPluginVersion: "6.18.0"},
				},
			},
			setupMocks: func(git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				git.PushError = errors.New("push failed: authentication failed")
			},
			expectError:    true,
			expectedErrMsg: "failed to push branch",
			verifyMocks: func(t *testing.T, git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				assert.True(t, git.CloneCalled)
				assert.True(t, git.CreateBranchCalled)
				assert.True(t, git.CommitCalled)
				assert.True(t, build.BuildCalled)
				assert.True(t, git.PushCalled)
			},
		},
		{
			name: "timeout configuration applied",
			config: &ModConfig{
				ID:           "test-workflow",
				TargetRepo:   "https://github.com/org/project",
				BaseRef:      "refs/heads/main",
				BuildTimeout: "5m",
				Steps: []ModStep{
					{Type: "orw-apply", ID: "java-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}, RecipeGroup: "org.openrewrite.recipe", RecipeArtifact: "rewrite-migrate-java", RecipeVersion: "3.17.0", MavenPluginVersion: "6.18.0"},
				},
			},
			setupMocks: func(git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				// Success case
			},
			expectError: false,
			verifyMocks: func(t *testing.T, git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				assert.Equal(t, 5*time.Minute, build.BuildConfig.Timeout)
			},
		},
		{
			name: "lane override applied",
			config: &ModConfig{
				ID:         "test-workflow",
				TargetRepo: "https://github.com/org/project",
				BaseRef:    "refs/heads/main",
				Lane:       "D",
				Steps: []ModStep{
					{Type: "orw-apply", ID: "java-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}, RecipeGroup: "org.openrewrite.recipe", RecipeArtifact: "rewrite-migrate-java", RecipeVersion: "3.17.0", MavenPluginVersion: "6.18.0"},
				},
			},
			setupMocks: func(git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				// Success case
			},
			expectError: false,
			verifyMocks: func(t *testing.T, git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				assert.Equal(t, "D", build.BuildConfig.Lane)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp workspace
			workspaceDir := t.TempDir()

			// Setup mocks
			mockGit := NewMockGitOperations()
			mockRecipe := NewMockRecipeExecutor()
			mockBuild := NewMockBuildChecker()
			tt.setupMocks(mockGit, mockRecipe, mockBuild)

			// Create runner with mocks
			runner := &ModRunner{
				config:         tt.config,
				workspaceDir:   workspaceDir,
				gitOps:         mockGit,
				recipeExecutor: mockRecipe,
				buildChecker:   mockBuild,
			}

			// Run the workflow
			ctx := context.Background()
			result, err := runner.Run(ctx)

			// Check error expectation
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrMsg)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}

			// Verify mock calls
			if tt.verifyMocks != nil {
				tt.verifyMocks(t, mockGit, mockRecipe, mockBuild)
			}
		})
	}
}

// RED: When orw-apply fails inside the container with a clear error (e.g., "No build file found"),
// the runner must surface a terminal error so the API status becomes failed instead of staying running.
func TestModRunner_ORWApplyNoBuildFileError(t *testing.T) {
	// Save and restore submitter
	orig := submitAndWaitTerminal
	defer func() { submitAndWaitTerminal = orig }()

	// Stub submission to simulate container failure with meaningful message
	submitAndWaitTerminal = func(hclPath string, timeout time.Duration) error {
		return fmt.Errorf("No build file found (pom.xml, build.gradle) in project root")
	}

	// Create temp workspace and minimal template
	workspaceDir := t.TempDir()
	jobsDir := filepath.Join(workspaceDir, "roadmap", "mods", "jobs")
	_ = os.MkdirAll(jobsDir, 0755)
	// Minimal HCL content is enough for rendering/substitution path
	hcl := []byte("job \"orw-apply-test\" { group \"orw\" { task \"openrewrite-apply\" { driver=\"docker\" config { volumes=[\"${CONTEXT_HOST_DIR}:/workspace/context:ro\",\"${OUT_HOST_DIR}:/workspace/out\"] } } } }")
	_ = os.WriteFile(filepath.Join(jobsDir, "orw_apply.hcl"), hcl, 0644)

	// Config with a single orw-apply step
	config := &ModConfig{
		ID:         "java11to17-test",
		TargetRepo: "https://example.com/org/repo.git",
		BaseRef:    "main",
		Steps:      []ModStep{{Type: "orw-apply", ID: "java11to17-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}, RecipeGroup: "org.openrewrite.recipe", RecipeArtifact: "rewrite-migrate-java", RecipeVersion: "3.17.0"}},
	}

	runner, err := NewModRunner(config, workspaceDir)
	assert.NoError(t, err)

	// Mocks: clone creates repo directory so tar succeeds
	mockGit := NewMockGitOperations()
	mockBuild := NewMockBuildChecker() // not reached
	runner.SetGitOperations(mockGit)
	runner.SetBuildChecker(mockBuild)

	// Ensure a minimal pom.xml exists to bypass build-file precheck and hit submit stub
	repoDir := filepath.Join(workspaceDir, "repo-apply")
	_ = os.MkdirAll(repoDir, 0755)
	_ = os.WriteFile(filepath.Join(repoDir, "pom.xml"), []byte("<project></project>"), 0644)

	// Run
	_, runErr := runner.Run(context.Background())
	assert.Error(t, runErr)
	// Depending on pre-checks, we may fail before submission on missing build files
	assert.Contains(t, runErr.Error(), "no build file found")
}

func TestModResult_Summary(t *testing.T) {
	result := &ModResult{
		Success:      true,
		WorkflowID:   "test-workflow",
		BranchName:   "workflow/test-workflow/1234567890",
		CommitSHA:    "abcdef123456",
		BuildVersion: "v1.0.0",
		StepResults: []StepResult{
			{StepID: "recipe-step", Success: true, Message: "Recipe applied successfully"},
			{StepID: "build-step", Success: true, Message: "Build completed"},
		},
	}

	summary := result.Summary()
	assert.Contains(t, summary, "Workflow: test-workflow")
	assert.Contains(t, summary, "Status: SUCCESS")
	assert.Contains(t, summary, "Branch: workflow/test-workflow/1234567890")
	assert.Contains(t, summary, "Build: v1.0.0")
	assert.Contains(t, summary, "Recipe applied successfully")
	assert.Contains(t, summary, "Build completed")

	// Test failure case
	result.Success = false
	result.ErrorMessage = "Build failed"
	summary = result.Summary()
	assert.Contains(t, summary, "Status: FAILED")
	assert.Contains(t, summary, "Error: Build failed")
}

// Test individual function coverage for runner methods
func TestModRunner_Setters(t *testing.T) {
	config := &ModConfig{
		ID:         "test",
		TargetRepo: "https://github.com/org/repo",
		BaseRef:    "main",
		Steps:      []ModStep{{Type: "recipe", ID: "test", Engine: "openrewrite", Recipes: []string{"com.acme.Recipe"}}},
	}

	runner, err := NewModRunner(config, t.TempDir())
	assert.NoError(t, err)

	// Test setters
	mockGit := NewMockGitOperations()
	mockRecipe := NewMockRecipeExecutor()
	mockBuild := NewMockBuildChecker()
	mockProvider := NewMockGitProvider()

	runner.SetGitOperations(mockGit)
	runner.SetRecipeExecutor(mockRecipe)
	runner.SetBuildChecker(mockBuild)
	runner.SetGitProvider(mockProvider)
	runner.SetJobSubmitter(NoopJobSubmitter{})

	// Test getters
	assert.Equal(t, mockProvider, runner.GetGitProvider())
	assert.Equal(t, mockBuild, runner.GetBuildChecker())
	assert.NotEmpty(t, runner.GetWorkspaceDir())
	assert.Equal(t, config.TargetRepo, runner.GetTargetRepo())
}

func TestModRunner_PrepareRepo(t *testing.T) {
	tests := []struct {
		name        string
		setupMocks  func(*MockGitOperations)
		expectError bool
	}{
		{
			name: "successful preparation",
			setupMocks: func(git *MockGitOperations) {
				// Success case - no errors
			},
			expectError: false,
		},
		{
			name: "clone failure",
			setupMocks: func(git *MockGitOperations) {
				git.CloneError = errors.New("clone failed")
			},
			expectError: true,
		},
		{
			name: "branch creation failure",
			setupMocks: func(git *MockGitOperations) {
				git.CreateBranchError = errors.New("branch creation failed")
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ModConfig{
				ID:         "test",
				TargetRepo: "https://github.com/org/repo",
				BaseRef:    "main",
				Steps:      []ModStep{{Type: "recipe", ID: "test", Engine: "openrewrite", Recipes: []string{"com.acme.Recipe"}}},
			}

			runner, err := NewModRunner(config, t.TempDir())
			assert.NoError(t, err)

			mockGit := NewMockGitOperations()
			tt.setupMocks(mockGit)
			runner.SetGitOperations(mockGit)

			ctx := context.Background()
			repoPath, branchName, err := runner.PrepareRepo(ctx)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, repoPath)
				assert.Empty(t, branchName)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, repoPath)
				assert.Contains(t, branchName, "workflow/test/")
			}
		})
	}
}

func TestModRunner_ApplyDiffAndBuild(t *testing.T) {
	// Test basic function error paths - we don't test full functionality as it depends on external files
	config := &ModConfig{
		ID:         "test",
		TargetRepo: "https://github.com/org/repo",
		BaseRef:    "main",
		Steps:      []ModStep{{Type: "recipe", ID: "test", Engine: "openrewrite", Recipes: []string{"com.acme.Recipe"}}},
	}

	runner, err := NewModRunner(config, t.TempDir())
	assert.NoError(t, err)

	mockRecipe := NewMockRecipeExecutor()
	mockGit := NewMockGitOperations()
	mockBuild := NewMockBuildChecker()

	runner.SetRecipeExecutor(mockRecipe)
	runner.SetGitOperations(mockGit)
	runner.SetBuildChecker(mockBuild)

	ctx := context.Background()

	// Test with non-existent diff file to get coverage of error path
	err = runner.ApplyDiffAndBuild(ctx, "/nonexistent/path", "/nonexistent/diff.patch")
	assert.Error(t, err) // Should fail to read diff file
}

func TestModRunner_RenderAssets(t *testing.T) {
	config := &ModConfig{
		ID:         "test-workflow",
		TargetRepo: "https://github.com/org/repo",
		BaseRef:    "main",
		Steps:      []ModStep{{Type: "recipe", ID: "test", Engine: "openrewrite", Recipes: []string{"com.acme.Recipe"}}},
	}

	runner, err := NewModRunner(config, t.TempDir())
	assert.NoError(t, err)

	// These tests just verify the functions can be called - they'll error due to missing template files
	// but that's expected and still provides coverage for error handling paths

	t.Run("RenderPlannerAssets", func(t *testing.T) {
		assets, err := runner.RenderPlannerAssets()
		// Expected to fail due to missing template files
		assert.Error(t, err)
		assert.Nil(t, assets)
	})

	t.Run("RenderLLMExecAssets", func(t *testing.T) {
		jobSpec, err := runner.RenderLLMExecAssets("test-option")
		// Expected to fail due to missing template files
		assert.Error(t, err)
		assert.Empty(t, jobSpec)
	})

	t.Run("RenderORWApplyAssets", func(t *testing.T) {
		jobSpec, err := runner.RenderORWApplyAssets("test-option")
		// Expected to fail due to missing template files
		assert.Error(t, err)
		assert.Empty(t, jobSpec)
	})

	t.Run("RenderReducerAssets", func(t *testing.T) {
		assets, err := runner.RenderReducerAssets()
		// Expected to fail due to missing template files
		assert.Error(t, err)
		assert.Nil(t, assets)
	})
}

func TestModRunner_GenerateMRDescription(t *testing.T) {
	config := &ModConfig{
		ID:         "test-workflow",
		TargetRepo: "https://github.com/org/repo",
		BaseRef:    "main",
		Steps:      []ModStep{{Type: "recipe", ID: "test", Engine: "openrewrite", Recipes: []string{"com.acme.Recipe"}}},
	}

	runner, err := NewModRunner(config, t.TempDir())
	assert.NoError(t, err)

	result := &ModResult{
		Success:      true,
		WorkflowID:   "test-workflow",
		BranchName:   "workflow/test/12345",
		BuildVersion: "v1.0.0",
		StepResults: []StepResult{
			{StepID: "recipe", Success: true, Message: "Applied successfully"},
		},
	}

	description := renderMRDescription(runner, result)
	assert.Contains(t, description, "Workflow")
	assert.Contains(t, description, "test-workflow")
	assert.Contains(t, description, "Applied successfully")
}

func TestModRunner_CleanupWorkspace(t *testing.T) {
	// Create a temporary directory structure
	tempDir := t.TempDir()
	workspaceDir := filepath.Join(tempDir, "workspace")
	err := os.MkdirAll(workspaceDir, 0755)
	assert.NoError(t, err)

	// Create a test file
	testFile := filepath.Join(workspaceDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	assert.NoError(t, err)

	config := &ModConfig{
		ID:         "test",
		TargetRepo: "https://github.com/org/repo",
		BaseRef:    "main",
		Steps:      []ModStep{{Type: "recipe", ID: "test", Engine: "openrewrite", Recipes: []string{"com.acme.Recipe"}}},
	}

	runner, err := NewModRunner(config, workspaceDir)
	assert.NoError(t, err)

	// Verify file exists before cleanup
	_, err = os.Stat(testFile)
	assert.NoError(t, err)

	// Cleanup
	err = runner.CleanupWorkspace()
	assert.NoError(t, err)

	// Verify file is removed after cleanup
	_, err = os.Stat(testFile)
	assert.True(t, os.IsNotExist(err))
}
