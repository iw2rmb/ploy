package mods

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Mock interfaces for testing are now in mocks.go

// moved to runner_workflow_test.go

// RED: When orw-apply fails inside the container with a clear error (e.g., "No build file found"),
// the runner must surface a terminal error so the API status becomes failed instead of staying running.
func TestModRunner_ORWApplyNoBuildFileError(t *testing.T) {
	// Save and restore submitter
	orig := submitAndWaitTerminal
	oldValidate := validateJob
	defer func() {
		submitAndWaitTerminal = orig
		validateJob = oldValidate
	}()

	// Stub submission to simulate container failure with meaningful message
	submitAndWaitTerminal = func(hclPath string, timeout time.Duration) error {
		return fmt.Errorf("No build file found (pom.xml, build.gradle) in project root")
	}
	validateJob = func(string) error { return nil }

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
	assert.Contains(t, strings.ToLower(runErr.Error()), "no build file found")
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
// moved to runner_di_test.go

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
