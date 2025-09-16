package mods

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test runner integration
func TestModRunnerWithHealing(t *testing.T) {
	t.Run("healing triggered on build failure", func(t *testing.T) {
		// Stub out external interactions via injectable seams
		oldDL := downloadToFileFn
		oldGet := getJSONFn
		oldPut := putJSONFn
		oldVDP := validateDiffPathsFn
		oldVUD := validateUnifiedDiffFn
		oldAD := applyUnifiedDiffFn
		oldHasChanges := hasRepoChangesFn
		// Stub remote artifact calls: write minimal content to dest, and no-op JSON interactions
		downloadToFileFn = func(_ string, dest string) error {
			_ = os.MkdirAll(filepath.Dir(dest), 0755)
			diff := "--- a/pom.xml\n+++ b/pom.xml\n@@ -1 +1 @@\n-<project></project>\n+<project><modelVersion>4.0.0</modelVersion></project>\n"
			return os.WriteFile(dest, []byte(diff), 0644)
		}
		getJSONFn = func(string, string) ([]byte, int, error) { return nil, 404, nil }
		putJSONFn = func(string, string, []byte) error { return nil }
		// Skip path validation and patch application in unit test
		validateDiffPathsFn = func(string, []string) error { return nil }
		validateUnifiedDiffFn = func(context.Context, string, string) error { return nil }
		applyUnifiedDiffFn = func(context.Context, string, string) error { return nil }
		hasRepoChangesFn = func(string) (bool, error) { return true, nil }

		defer func() {
			downloadToFileFn = oldDL
			getJSONFn = oldGet
			putJSONFn = oldPut
			validateDiffPathsFn = oldVDP
			validateUnifiedDiffFn = oldVUD
			applyUnifiedDiffFn = oldAD
			hasRepoChangesFn = oldHasChanges
		}()
		// MOD_ID used in artifact paths and events
		_ = os.Setenv("MOD_ID", "mod-test-exec")
		defer func() { _ = os.Unsetenv("MOD_ID") }()
		// Setup
		config := &ModConfig{
			ID:         "healing-test",
			TargetRepo: "https://github.com/test/repo",
			BaseRef:    "main",
			SelfHeal: &SelfHealConfig{
				MaxRetries: 2,
				Enabled:    true,
			},
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
		}

		// Mocks
		mockGit := &MockGitOperations{}
		mockRecipe := &MockRecipeExecutor{}
		var check seqBuildChecker
		mockBuild := &check

		runner, err := NewModRunner(config, "/tmp/workspace")
		require.NoError(t, err)

		runner.SetGitOperations(mockGit)
		runner.SetRecipeExecutor(mockRecipe)
		runner.SetBuildChecker(mockBuild)
		runner.SetJobHelper(testJobHelper{})
		runner.SetHCLSubmitter(okHCLSubmitter{})
		// Non-nil jobSubmitter enables healing path; injected jobHelper handles planner/reducer
		runner.SetJobSubmitter(NoopJobSubmitter{})
		// Healer that returns a successful winner without submitting real jobs
		runner.SetHealingOrchestrator(okHealer{})

		// Ensure a minimal build file exists to pass ORW guard
		_ = os.MkdirAll("/tmp/workspace/repo", 0755)
		_ = os.WriteFile("/tmp/workspace/repo/pom.xml", []byte("<project></project>"), 0644)

		// Execute
		ctx := context.Background()
		result, err := runner.Run(ctx)

		// Verify healing was attempted
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.HealingSummary)
		assert.True(t, result.HealingSummary.Enabled)
		assert.Greater(t, result.HealingSummary.AttemptsCount, 0)
	})
}
