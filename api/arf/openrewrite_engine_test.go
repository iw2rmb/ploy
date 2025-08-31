package arf

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
	"github.com/iw2rmb/ploy/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOpenRewriteEngine_DetectBuildSystem tests build system detection
func TestOpenRewriteEngine_DetectBuildSystem(t *testing.T) {
	engine := NewOpenRewriteEngine()

	tests := []struct {
		name     string
		files    map[string]string
		expected string
	}{
		{
			name: "Maven project",
			files: map[string]string{
				"pom.xml": `<?xml version="1.0"?><project></project>`,
			},
			expected: "maven",
		},
		{
			name: "Gradle project",
			files: map[string]string{
				"build.gradle": `plugins { id 'java' }`,
			},
			expected: "gradle",
		},
		{
			name: "Gradle Kotlin DSL project",
			files: map[string]string{
				"build.gradle.kts": `plugins { java }`,
			},
			expected: "gradle",
		},
		{
			name:     "No build system",
			files:    map[string]string{},
			expected: "unknown",
		},
		{
			name: "Both Maven and Gradle (Maven takes precedence)",
			files: map[string]string{
				"pom.xml":      `<?xml version="1.0"?><project></project>`,
				"build.gradle": `plugins { id 'java' }`,
			},
			expected: "maven",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with test files
			tmpDir := testutils.CreateTempDir(t, "build-detect")
			defer os.RemoveAll(tmpDir)

			for filename, content := range tt.files {
				err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0644)
				require.NoError(t, err)
			}

			// Test detection
			result := engine.detectBuildSystem(tmpDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestOpenRewriteEngine_GetRecipeArtifacts tests recipe artifact mapping
func TestOpenRewriteEngine_GetRecipeArtifacts(t *testing.T) {
	engine := NewOpenRewriteEngine()

	tests := []struct {
		recipe   string
		expected string
	}{
		{
			recipe:   "org.openrewrite.java.migrate.Java11toJava17",
			expected: "org.openrewrite.recipe:rewrite-migrate-java:2.5.0",
		},
		{
			recipe:   "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0",
			expected: "org.openrewrite.recipe:rewrite-spring:5.7.0",
		},
		{
			recipe:   "org.openrewrite.java.cleanup.UnnecessaryThrows",
			expected: "org.openrewrite:rewrite-java:8.21.0",
		},
		{
			recipe:   "unknown.recipe",
			expected: "org.openrewrite:rewrite-java:8.21.0", // Default fallback
		},
	}

	for _, tt := range tests {
		t.Run(tt.recipe, func(t *testing.T) {
			result := engine.getRecipeArtifacts(tt.recipe)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestOpenRewriteEngine_BuildEnvironment tests environment variable setup
func TestOpenRewriteEngine_BuildEnvironment(t *testing.T) {
	engine := NewOpenRewriteEngine()
	engine.javaHome = "/test/java/home"

	env := engine.buildEnvironment()

	// Check that Java options are set
	hasJavaHome := false
	hasMavenOpts := false
	hasGradleOpts := false

	for _, e := range env {
		if e == "JAVA_HOME=/test/java/home" {
			hasJavaHome = true
		}
		if e == "MAVEN_OPTS=-Xmx2G" {
			hasMavenOpts = true
		}
		if e == "GRADLE_OPTS=-Xmx2G" {
			hasGradleOpts = true
		}
	}

	assert.True(t, hasJavaHome, "JAVA_HOME should be set")
	assert.True(t, hasMavenOpts, "MAVEN_OPTS should be set")
	assert.True(t, hasGradleOpts, "GRADLE_OPTS should be set")
}

// TestOpenRewriteEngine_Execute_NoMaven tests error handling when Maven is not available
func TestOpenRewriteEngine_Execute_NoMaven(t *testing.T) {
	// Skip if Maven is actually available
	if _, err := exec.LookPath("mvn"); err == nil {
		t.Skip("Maven is available, skipping no-Maven test")
	}

	repoPath := testutils.CreateTempDir(t, "no-maven-test")
	defer os.RemoveAll(repoPath)

	// Create pom.xml to trigger Maven detection
	pomContent := `<?xml version="1.0"?><project></project>`
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "pom.xml"), []byte(pomContent), 0644))

	engine := NewOpenRewriteEngine()
	engine.mavenPath = "" // Ensure Maven is not available

	step := &models.RecipeStep{
		Name: "test",
		Type: models.StepTypeOpenRewrite,
		Config: map[string]interface{}{
			"recipe": "org.openrewrite.java.cleanup.UnnecessaryThrows",
		},
	}

	ctx := context.Background()
	result, err := engine.Execute(ctx, step, repoPath)

	// Should return a result with error details, not a nil result
	require.NoError(t, err, "Execute should not return error, but include error in result")
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Message, "Maven not found")
}

// TestOpenRewriteEngine_Execute_NoBuildSystem tests error handling for unsupported projects
func TestOpenRewriteEngine_Execute_NoBuildSystem(t *testing.T) {
	repoPath := testutils.CreateTempDir(t, "no-build-test")
	defer os.RemoveAll(repoPath)

	// Create a project with no build system
	require.NoError(t, os.WriteFile(
		filepath.Join(repoPath, "Main.java"),
		[]byte("public class Main {}"),
		0644,
	))

	engine := NewOpenRewriteEngine()

	step := &models.RecipeStep{
		Name: "test",
		Type: models.StepTypeOpenRewrite,
		Config: map[string]interface{}{
			"recipe": "org.openrewrite.java.cleanup.UnnecessaryThrows",
		},
	}

	ctx := context.Background()
	result, err := engine.Execute(ctx, step, repoPath)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no supported build system")
	assert.Nil(t, result)
}

// TestOpenRewriteEngine_Execute_MissingRecipe tests error handling for missing recipe config
func TestOpenRewriteEngine_Execute_MissingRecipe(t *testing.T) {
	repoPath := testutils.CreateTempDir(t, "missing-recipe-test")
	defer os.RemoveAll(repoPath)

	// Create pom.xml
	pomContent := `<?xml version="1.0"?><project></project>`
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "pom.xml"), []byte(pomContent), 0644))

	engine := NewOpenRewriteEngine()

	step := &models.RecipeStep{
		Name:   "test",
		Type:   models.StepTypeOpenRewrite,
		Config: map[string]interface{}{}, // No recipe specified
	}

	ctx := context.Background()
	result, err := engine.Execute(ctx, step, repoPath)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing recipe configuration")
	assert.Nil(t, result)
}

// TestOpenRewriteEngine_ParseMavenOutput tests Maven output parsing
func TestOpenRewriteEngine_ParseMavenOutput(t *testing.T) {
	engine := NewOpenRewriteEngine()

	tests := []struct {
		name     string
		output   string
		expected []string
	}{
		{
			name: "Files modified",
			output: `[INFO] Running recipe...
[INFO] Changes have been made to Main.java
[INFO] Modified Test.java
[INFO] BUILD SUCCESS`,
			expected: []string{"Main.java", "Test.java"},
		},
		{
			name: "Files fixed",
			output: `[INFO] Running recipe...
[INFO] App.java fixed
[INFO] Config.xml fixed
[INFO] BUILD SUCCESS`,
			expected: []string{"App.java", "Config.xml"},
		},
		{
			name: "No specific files but success",
			output: `[INFO] Running recipe...
[INFO] Executing OpenRewrite
[INFO] BUILD SUCCESS`,
			expected: []string{"pom.xml"}, // Default assumption
		},
		{
			name:     "Build failure",
			output:   `[ERROR] BUILD FAILURE`,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.parseMavenOutput(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestOpenRewriteEngine_ParseGradleOutput tests Gradle output parsing
func TestOpenRewriteEngine_ParseGradleOutput(t *testing.T) {
	engine := NewOpenRewriteEngine()

	tests := []struct {
		name     string
		output   string
		expected []string
	}{
		{
			name: "Files modified",
			output: `> Task :rewriteRun
Fixed Main.java
Modified Test.java
BUILD SUCCESSFUL`,
			expected: []string{"Main.java", "Test.java"},
		},
		{
			name: "No specific files but success",
			output: `> Task :rewriteRun
Running OpenRewrite recipes
BUILD SUCCESSFUL`,
			expected: []string{"build.gradle"}, // Default assumption
		},
		{
			name:     "Build failure",
			output:   `BUILD FAILED`,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.parseGradleOutput(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestOpenRewriteEngine_ExecutionTimeout tests timeout handling
func TestOpenRewriteEngine_ExecutionTimeout(t *testing.T) {
	// This test verifies that the engine respects context cancellation
	repoPath := testutils.CreateTempDir(t, "timeout-test")
	defer os.RemoveAll(repoPath)

	// Create pom.xml
	pomContent := `<?xml version="1.0"?><project></project>`
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "pom.xml"), []byte(pomContent), 0644))

	engine := NewOpenRewriteEngine()

	step := &models.RecipeStep{
		Name: "test",
		Type: models.StepTypeOpenRewrite,
		Config: map[string]interface{}{
			"recipe": "org.openrewrite.java.cleanup.UnnecessaryThrows",
		},
	}

	// Create context with immediate cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(10 * time.Millisecond) // Ensure context is cancelled

	result, err := engine.Execute(ctx, step, repoPath)

	// The execution should handle the cancelled context gracefully
	// Either by returning an error or a result with failure status
	if err != nil {
		assert.Contains(t, err.Error(), "context")
	} else if result != nil {
		assert.False(t, result.Success)
	}
}

// TestOpenRewriteEngine_IntegrationSkip demonstrates skipping integration tests
func TestOpenRewriteEngine_IntegrationSkip(t *testing.T) {
	// Skip actual OpenRewrite execution tests that require full setup
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if Maven is available
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("Maven not available, skipping integration test")
	}

	// This test would require actual OpenRewrite setup and Maven Central access
	// For unit tests, we skip this to avoid external dependencies
	t.Skip("Skipping actual OpenRewrite execution - use batch jobs for production")
}

// Note: The OpenRewriteEngine is primarily for local testing and development.
// Production transformations should use OpenRewriteDispatcher with Nomad batch jobs.
// These tests verify the engine's basic functionality without requiring full
// OpenRewrite infrastructure or external dependencies.
