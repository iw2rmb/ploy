package transflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/stretchr/testify/assert"
)

// Mock interfaces for testing are now in mocks.go

func TestTransflowRunner_Run(t *testing.T) {
	tests := []struct {
		name           string
		config         *TransflowConfig
		setupMocks     func(*MockGitOperations, *MockRecipeExecutor, *MockBuildChecker)
		expectError    bool
		expectedErrMsg string
		verifyMocks    func(*testing.T, *MockGitOperations, *MockRecipeExecutor, *MockBuildChecker)
	}{
		{
			name: "successful complete workflow",
			config: &TransflowConfig{
				ID:           "test-workflow",
				TargetRepo:   "https://github.com/org/project",
				BaseRef:      "refs/heads/main",
				BuildTimeout: "10m",
				Steps: []TransflowStep{
					{
						Type:    "recipe",
						ID:      "test-recipe",
						Engine:  "openrewrite",
						Recipes: []string{"com.acme.TestRecipe"},
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
				assert.True(t, recipe.ExecuteCalled, "ExecuteRecipes should be called")
				assert.True(t, git.CommitCalled, "CommitChanges should be called")
				assert.True(t, build.BuildCalled, "CheckBuild should be called")
				assert.True(t, git.PushCalled, "PushBranch should be called")

				// Verify parameters
				assert.Equal(t, "https://github.com/org/project", git.CloneRepo)
				assert.Equal(t, "refs/heads/main", git.CloneBranch)
				assert.Contains(t, git.BranchName, "workflow/test-workflow/")
				assert.Equal(t, []string{"com.acme.TestRecipe"}, recipe.RecipeIDs)
				assert.Contains(t, git.CommitMessage, "Applied recipe")
				assert.Contains(t, build.BuildConfig.App, "tfw-test-workflow-")
				assert.Equal(t, "https://github.com/org/project", git.PushRemoteURL)
			},
		},
		{
			name: "git clone failure",
			config: &TransflowConfig{
				ID:         "test-workflow",
				TargetRepo: "https://github.com/org/project",
				BaseRef:    "refs/heads/main",
				Steps: []TransflowStep{
					{Type: "recipe", ID: "test-recipe", Engine: "openrewrite", Recipes: []string{"com.acme.Recipe"}},
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
				assert.False(t, recipe.ExecuteCalled)
				assert.False(t, build.BuildCalled)
				assert.False(t, git.PushCalled)
			},
		},
		{
			name: "recipe execution failure",
			config: &TransflowConfig{
				ID:         "test-workflow",
				TargetRepo: "https://github.com/org/project",
				BaseRef:    "refs/heads/main",
				Steps: []TransflowStep{
					{Type: "recipe", ID: "test-recipe", Engine: "openrewrite", Recipes: []string{"com.acme.Recipe"}},
				},
			},
			setupMocks: func(git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				recipe.ExecuteError = errors.New("recipe execution failed")
			},
			expectError:    true,
			expectedErrMsg: "failed to execute recipes",
			verifyMocks: func(t *testing.T, git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				assert.True(t, git.CloneCalled)
				assert.True(t, git.CreateBranchCalled)
				assert.True(t, recipe.ExecuteCalled)
				assert.False(t, git.CommitCalled)
				assert.False(t, build.BuildCalled)
				assert.False(t, git.PushCalled)
			},
		},
		{
			name: "build check failure",
			config: &TransflowConfig{
				ID:         "test-workflow",
				TargetRepo: "https://github.com/org/project",
				BaseRef:    "refs/heads/main",
				Steps: []TransflowStep{
					{Type: "recipe", ID: "test-recipe", Engine: "openrewrite", Recipes: []string{"com.acme.Recipe"}},
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
			expectedErrMsg: "build check failed",
			verifyMocks: func(t *testing.T, git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				assert.True(t, git.CloneCalled)
				assert.True(t, git.CreateBranchCalled)
				assert.True(t, recipe.ExecuteCalled)
				assert.True(t, git.CommitCalled)
				assert.True(t, build.BuildCalled)
				assert.False(t, git.PushCalled) // Should not push on build failure
			},
		},
		{
			name: "push failure",
			config: &TransflowConfig{
				ID:         "test-workflow",
				TargetRepo: "https://github.com/org/project",
				BaseRef:    "refs/heads/main",
				Steps: []TransflowStep{
					{Type: "recipe", ID: "test-recipe", Engine: "openrewrite", Recipes: []string{"com.acme.Recipe"}},
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
				assert.True(t, recipe.ExecuteCalled)
				assert.True(t, git.CommitCalled)
				assert.True(t, build.BuildCalled)
				assert.True(t, git.PushCalled)
			},
		},
		{
			name: "timeout configuration applied",
			config: &TransflowConfig{
				ID:           "test-workflow",
				TargetRepo:   "https://github.com/org/project",
				BaseRef:      "refs/heads/main",
				BuildTimeout: "5m",
				Steps: []TransflowStep{
					{Type: "recipe", ID: "test-recipe", Engine: "openrewrite", Recipes: []string{"com.acme.Recipe"}},
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
			config: &TransflowConfig{
				ID:         "test-workflow",
				TargetRepo: "https://github.com/org/project",
				BaseRef:    "refs/heads/main",
				Lane:       "D",
				Steps: []TransflowStep{
					{Type: "recipe", ID: "test-recipe", Engine: "openrewrite", Recipes: []string{"com.acme.Recipe"}},
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
			runner := &TransflowRunner{
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

func TestTransflowResult_Summary(t *testing.T) {
	result := &TransflowResult{
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
