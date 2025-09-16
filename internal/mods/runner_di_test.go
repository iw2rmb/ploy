package mods

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test individual function coverage for runner DI methods
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
