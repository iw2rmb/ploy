package arf

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
	"github.com/iw2rmb/ploy/api/arf/storage"
	"github.com/iw2rmb/ploy/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOpenRewriteEngine_ExecuteMavenProject tests Maven-based OpenRewrite execution
// TDD RED PHASE: This test MUST FAIL initially as OpenRewriteEngine doesn't exist yet
func TestOpenRewriteEngine_ExecuteMavenProject(t *testing.T) {
	// Create test repository path
	repoPath := testutils.CreateTempDir(t, "maven-test")
	defer os.RemoveAll(repoPath)

	// Create a simple pom.xml
	pomContent := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.test</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>
    <properties>
        <java.version>11</java.version>
    </properties>
</project>`
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "pom.xml"), []byte(pomContent), 0644))

	// Create test Java file
	srcDir := filepath.Join(repoPath, "src", "main", "java", "com", "test")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	javaContent := `package com.test;
public class TestApp {
    public static void main(String[] args) {
        System.out.println("Java 11 code");
    }
}`
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "TestApp.java"), []byte(javaContent), 0644))

	// Create engine (this will fail in RED phase)
	engine := NewOpenRewriteEngine()
	
	// Create recipe step
	step := &models.RecipeStep{
		Name: "java-migration",
		Type: models.StepTypeOpenRewrite,
		Config: map[string]interface{}{
			"recipe": "org.openrewrite.java.migrate.Java11toJava17",
		},
	}

	// Execute
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := engine.Execute(ctx, step, repoPath)
	
	// Assertions for GREEN phase
	require.NoError(t, err, "OpenRewrite execution should not error")
	assert.NotNil(t, result, "Result should not be nil")
	assert.True(t, result.Success, "Execution should be successful")
	assert.Greater(t, result.ChangesApplied, 0, "Should have applied changes")
	assert.NotEqual(t, "MockFile.java", result.FilesModified[0], "Should not return mock file")
	
	// Verify pom.xml was updated to Java 17
	pomBytes, err := os.ReadFile(filepath.Join(repoPath, "pom.xml"))
	require.NoError(t, err)
	assert.Contains(t, string(pomBytes), "<java.version>17</java.version>", "Java version should be updated to 17")
}

// TestOpenRewriteEngine_ExecuteGradleProject tests Gradle-based OpenRewrite execution
// TDD RED PHASE: This test MUST FAIL initially
func TestOpenRewriteEngine_ExecuteGradleProject(t *testing.T) {
	// Create test repository path
	repoPath := testutils.CreateTempDir(t, "gradle-test")
	defer os.RemoveAll(repoPath)

	// Create build.gradle
	buildContent := `plugins {
    id 'java'
}

java {
    sourceCompatibility = JavaVersion.VERSION_11
    targetCompatibility = JavaVersion.VERSION_11
}

repositories {
    mavenCentral()
}`
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "build.gradle"), []byte(buildContent), 0644))

	// Create test Java file
	srcDir := filepath.Join(repoPath, "src", "main", "java", "com", "test")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	javaContent := `package com.test;
public class TestApp {
    public static void main(String[] args) {
        System.out.println("Java 11 code");
    }
}`
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "TestApp.java"), []byte(javaContent), 0644))

	// Create engine
	engine := NewOpenRewriteEngine()
	
	// Create recipe step
	step := &models.RecipeStep{
		Name: "java-migration",
		Type: models.StepTypeOpenRewrite,
		Config: map[string]interface{}{
			"recipe": "org.openrewrite.java.migrate.Java11toJava17",
		},
	}

	// Execute
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := engine.Execute(ctx, step, repoPath)
	
	// Assertions
	require.NoError(t, err, "OpenRewrite execution should not error")
	assert.NotNil(t, result, "Result should not be nil")
	assert.True(t, result.Success, "Execution should be successful")
	assert.Greater(t, result.ChangesApplied, 0, "Should have applied changes")
	
	// Verify build.gradle was updated
	buildBytes, err := os.ReadFile(filepath.Join(repoPath, "build.gradle"))
	require.NoError(t, err)
	assert.Contains(t, string(buildBytes), "VERSION_17", "Java version should be updated to 17")
}

// TestOpenRewriteEngine_DetectBuildSystem tests build system detection
// TDD RED PHASE: This test MUST FAIL initially
func TestOpenRewriteEngine_DetectBuildSystem(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected string
	}{
		{
			name: "Maven project",
			files: map[string]string{
				"pom.xml": "<project></project>",
			},
			expected: "maven",
		},
		{
			name: "Gradle project with build.gradle",
			files: map[string]string{
				"build.gradle": "apply plugin: 'java'",
			},
			expected: "gradle",
		},
		{
			name: "Gradle Kotlin DSL project",
			files: map[string]string{
				"build.gradle.kts": "plugins { java }",
			},
			expected: "gradle",
		},
		{
			name: "Both Maven and Gradle (Maven takes precedence)",
			files: map[string]string{
				"pom.xml":      "<project></project>",
				"build.gradle": "apply plugin: 'java'",
			},
			expected: "maven",
		},
		{
			name:     "No build system",
			files:    map[string]string{},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tempDir := testutils.CreateTempDir(t, "detect-test")
			defer os.RemoveAll(tempDir)

			// Create test files
			for filename, content := range tt.files {
				filepath := filepath.Join(tempDir, filename)
				require.NoError(t, os.WriteFile(filepath, []byte(content), 0644))
			}

			// Create engine and detect
			engine := NewOpenRewriteEngine()
			detected := engine.detectBuildSystem(tempDir)

			assert.Equal(t, tt.expected, detected, "Build system detection mismatch")
		})
	}
}

// TestOpenRewriteEngine_ApplyJava11to17Recipe tests specific Java migration recipe
// TDD RED PHASE: This test MUST FAIL initially
func TestOpenRewriteEngine_ApplyJava11to17Recipe(t *testing.T) {
	// Skip if Maven is not available
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("Maven not available, skipping test")
	}

	repoPath := testutils.CreateTempDir(t, "java-migration")
	defer os.RemoveAll(repoPath)

	// Create a more complex pom.xml that needs migration
	pomContent := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.test</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>
    
    <properties>
        <java.version>11</java.version>
        <maven.compiler.source>11</maven.compiler.source>
        <maven.compiler.target>11</maven.compiler.target>
    </properties>
    
    <dependencies>
        <dependency>
            <groupId>javax.annotation</groupId>
            <artifactId>javax.annotation-api</artifactId>
            <version>1.3.2</version>
        </dependency>
    </dependencies>
</project>`
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "pom.xml"), []byte(pomContent), 0644))

	// Create Java file with Java 11 specific code
	srcDir := filepath.Join(repoPath, "src", "main", "java", "com", "test")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	javaContent := `package com.test;

import javax.annotation.PostConstruct;

public class TestApp {
    @PostConstruct
    public void init() {
        // This annotation needs migration
    }
    
    public static void main(String[] args) {
        var message = "Java 11 var keyword";
        System.out.println(message);
    }
}`
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "TestApp.java"), []byte(javaContent), 0644))

	// Create engine with proper configuration
	engine := NewOpenRewriteEngine()
	engine.ConfigureForJavaMigration()

	// Create recipe step for Java 11 to 17 migration
	step := &models.RecipeStep{
		Name: "java-11-to-17-migration",
		Type: models.StepTypeOpenRewrite,
		Config: map[string]interface{}{
			"recipe": "org.openrewrite.java.migrate.Java11toJava17",
			"recipeArtifacts": "org.openrewrite.recipe:rewrite-migrate-java:2.5.0",
		},
	}

	// Execute migration
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := engine.Execute(ctx, step, repoPath)
	
	// Detailed assertions
	require.NoError(t, err, "Java migration should not error")
	assert.NotNil(t, result, "Result should not be nil")
	assert.True(t, result.Success, "Migration should be successful")
	assert.GreaterOrEqual(t, result.ChangesApplied, 2, "Should have at least 2 changes (pom.xml and Java file)")
	
	// Verify pom.xml changes
	pomBytes, err := os.ReadFile(filepath.Join(repoPath, "pom.xml"))
	require.NoError(t, err)
	pomStr := string(pomBytes)
	assert.Contains(t, pomStr, "<java.version>17</java.version>", "Java version property updated")
	assert.Contains(t, pomStr, "<maven.compiler.source>17</maven.compiler.source>", "Compiler source updated")
	assert.Contains(t, pomStr, "<maven.compiler.target>17</maven.compiler.target>", "Compiler target updated")
	
	// Check for javax to jakarta migration
	assert.Contains(t, pomStr, "jakarta.annotation", "javax dependencies should be migrated to jakarta")
	
	// Verify Java file changes
	javaBytes, err := os.ReadFile(filepath.Join(srcDir, "TestApp.java"))
	require.NoError(t, err)
	javaStr := string(javaBytes)
	assert.Contains(t, javaStr, "jakarta.annotation.PostConstruct", "javax imports should be migrated to jakarta")
}

// TestRecipeExecutor_RealOpenRewriteExecution verifies mock is replaced with real engine
// TDD RED PHASE: This test MUST FAIL initially while mock is still in use
func TestRecipeExecutor_RealOpenRewriteExecution(t *testing.T) {
	// Create a mock storage
	storage := &mockRecipeStorage{}
	
	// Create sandbox manager
	sandboxMgr := NewMockSandboxManager()
	
	// Create recipe executor
	executor := NewRecipeExecutor(storage, sandboxMgr)
	
	// Create test repository path
	repoPath := testutils.CreateTempDir(t, "executor-test")
	defer os.RemoveAll(repoPath)
	
	// Create pom.xml to ensure it's a Java project
	pomContent := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.test</groupId>
    <artifactId>test</artifactId>
    <version>1.0.0</version>
</project>`
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "pom.xml"), []byte(pomContent), 0644))
	
	// Execute with OpenRewrite step
	ctx := context.Background()
	result, err := executor.ExecuteRecipeByID(ctx, "test-recipe", repoPath)
	
	// This should NOT return mock results
	require.NoError(t, err)
	assert.NotNil(t, result)
	
	// Key assertion: We should NOT get MockFile.java
	if len(result.FilesModified) > 0 {
		assert.NotEqual(t, "MockFile.java", result.FilesModified[0], 
			"Should use real OpenRewrite engine, not mock")
	}
	
	// The result should reflect real execution
	assert.NotContains(t, result.Diff, "mock transformation", 
		"Should not contain mock transformation text")
}

// mockRecipeStorage implements storage.RecipeStorage for testing
type mockRecipeStorage struct{}

func (m *mockRecipeStorage) CreateRecipe(ctx context.Context, recipe *models.Recipe) error {
	return nil
}

func (m *mockRecipeStorage) GetRecipe(ctx context.Context, recipeID string) (*models.Recipe, error) {
	recipe := &models.Recipe{
		ID: recipeID,
		Steps: []models.RecipeStep{
			{
				Name: "test-step",
				Type: models.StepTypeOpenRewrite,
				Config: map[string]interface{}{
					"recipe": "org.openrewrite.java.cleanup.UnnecessaryThrows",
				},
			},
		},
	}
	recipe.Metadata.Name = "Test Recipe"
	return recipe, nil
}

func (m *mockRecipeStorage) GetRecipeByNameAndVersion(ctx context.Context, name, version string) (*models.Recipe, error) {
	return m.GetRecipe(ctx, name)
}

func (m *mockRecipeStorage) UpdateRecipe(ctx context.Context, id string, recipe *models.Recipe) error {
	return nil
}

func (m *mockRecipeStorage) DeleteRecipe(ctx context.Context, recipeID string) error {
	return nil
}

func (m *mockRecipeStorage) ListRecipes(ctx context.Context, filter storage.RecipeFilter) ([]*models.Recipe, error) {
	return nil, nil
}

func (m *mockRecipeStorage) SearchRecipes(ctx context.Context, query string) ([]*storage.RecipeSearchResult, error) {
	return nil, nil
}

func (m *mockRecipeStorage) GetRecipeVersions(ctx context.Context, name string) ([]*models.Recipe, error) {
	return nil, nil
}

func (m *mockRecipeStorage) GetLatestRecipe(ctx context.Context, name string) (*models.Recipe, error) {
	return m.GetRecipe(ctx, name)
}

func (m *mockRecipeStorage) ImportRecipes(ctx context.Context, recipes []*models.Recipe) error {
	return nil
}

func (m *mockRecipeStorage) ExportRecipes(ctx context.Context, filter storage.RecipeFilter) ([]*models.Recipe, error) {
	return nil, nil
}

func (m *mockRecipeStorage) ValidateRecipe(ctx context.Context, recipe *models.Recipe) error {
	return nil
}

func (m *mockRecipeStorage) CheckRecipeIntegrity(ctx context.Context, recipeID string) error {
	return nil
}

func (m *mockRecipeStorage) RebuildIndex(ctx context.Context) error {
	return nil
}

func (m *mockRecipeStorage) UpdateIndex(ctx context.Context, recipe *models.Recipe, action storage.IndexAction) error {
	return nil
}

func (m *mockRecipeStorage) VerifyRecipeHash(ctx context.Context, id, expectedHash string) (bool, error) {
	return true, nil
}