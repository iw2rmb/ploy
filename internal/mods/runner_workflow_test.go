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
	_ = os.Setenv("MOD_ID", "mod-t-runner")
	defer func() { _ = os.Unsetenv("MOD_ID") }()

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
				assert.Contains(t, build.BuildConfig.App, "mod-test-workflow-")
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
			expectedErrMsg: "ORW HCL validation failed",
			verifyMocks: func(t *testing.T, git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				assert.True(t, git.CloneCalled)
				assert.True(t, git.CreateBranchCalled)
				assert.False(t, git.CommitCalled)
				assert.True(t, build.BuildCalled) // baseline build runs before validation error surfaces
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
			expectedErrMsg: "Baseline build failed",
			verifyMocks: func(t *testing.T, git *MockGitOperations, recipe *MockRecipeExecutor, build *MockBuildChecker) {
				assert.True(t, git.CloneCalled)
				assert.True(t, git.CreateBranchCalled)
				assert.False(t, git.CommitCalled) // abort before commit when baseline build fails
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
