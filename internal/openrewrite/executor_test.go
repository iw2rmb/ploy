package openrewrite

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/iw2rmb/ploy/internal/testutils/fixtures"
)

func TestExecutorImpl_DetectBuildSystem(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected BuildSystem
	}{
		{
			name: "detects Maven project",
			files: map[string]string{
				"pom.xml": `<?xml version="1.0"?><project></project>`,
			},
			expected: BuildSystemMaven,
		},
		{
			name: "detects Gradle project with build.gradle",
			files: map[string]string{
				"build.gradle": `plugins { id 'java' }`,
			},
			expected: BuildSystemGradle,
		},
		{
			name: "detects Gradle project with build.gradle.kts",
			files: map[string]string{
				"build.gradle.kts": `plugins { java }`,
			},
			expected: BuildSystemGradle,
		},
		{
			name: "prefers Maven when both exist",
			files: map[string]string{
				"pom.xml":      `<?xml version="1.0"?><project></project>`,
				"build.gradle": `plugins { id 'java' }`,
			},
			expected: BuildSystemMaven,
		},
		{
			name: "detects no build system",
			files: map[string]string{
				"App.java": `public class App {}`,
			},
			expected: BuildSystemNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			tempDir := t.TempDir()
			for filename, content := range tt.files {
				filePath := filepath.Join(tempDir, filename)
				require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
			}

			executor := NewExecutor(DefaultConfig())
			
			// Execute
			result := executor.DetectBuildSystem(tempDir)
			
			// Verify
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecutorImpl_DetectJavaVersion(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		expected    JavaVersion
		expectError bool
	}{
		{
			name: "detects Java 17 from Maven properties",
			files: map[string]string{
				"pom.xml": `<?xml version="1.0"?>
<project>
	<properties>
		<maven.compiler.source>17</maven.compiler.source>
		<maven.compiler.target>17</maven.compiler.target>
	</properties>
</project>`,
			},
			expected:    Java17,
			expectError: false,
		},
		{
			name: "detects Java 11 from Maven properties",
			files: map[string]string{
				"pom.xml": `<?xml version="1.0"?>
<project>
	<properties>
		<maven.compiler.source>11</maven.compiler.source>
		<maven.compiler.target>11</maven.compiler.target>
	</properties>
</project>`,
			},
			expected:    Java11,
			expectError: false,
		},
		{
			name: "detects Java 21 from Maven properties",
			files: map[string]string{
				"pom.xml": `<?xml version="1.0"?>
<project>
	<properties>
		<maven.compiler.source>21</maven.compiler.source>
		<maven.compiler.target>21</maven.compiler.target>
	</properties>
</project>`,
			},
			expected:    Java21,
			expectError: false,
		},
		{
			name: "detects Java version from Gradle",
			files: map[string]string{
				"build.gradle": `java {
	sourceCompatibility = JavaVersion.VERSION_17
	targetCompatibility = JavaVersion.VERSION_17
}`,
			},
			expected:    Java17,
			expectError: false,
		},
		{
			name: "defaults to Java 17 when not specified",
			files: map[string]string{
				"pom.xml": `<?xml version="1.0"?><project></project>`,
			},
			expected:    Java17,
			expectError: false,
		},
		{
			name: "detects from .java-version file",
			files: map[string]string{
				".java-version": "21",
			},
			expected:    Java21,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			tempDir := t.TempDir()
			for filename, content := range tt.files {
				filePath := filepath.Join(tempDir, filename)
				require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
			}

			executor := NewExecutor(DefaultConfig())
			
			// Execute
			result, err := executor.DetectJavaVersion(tempDir)
			
			// Verify
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestExecutorImpl_Execute(t *testing.T) {
	// Skip tests that require Maven/Gradle to be installed
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("Maven not installed, skipping integration test")
	}

	tests := []struct {
		name        string
		jobID       string
		setupTar    func() []byte
		recipe      RecipeConfig
		expectError bool
		validate    func(t *testing.T, result *TransformResult)
	}{
		{
			name:  "executes simple Maven transformation",
			jobID: "maven-test-001",
			setupTar: func() []byte {
				fixture := &fixtures.ApplicationTar{
					Name:     "maven-project",
					Language: "java",
					Files: map[string]string{
						"pom.xml": `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
		 xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
		 xsi:schemaLocation="http://maven.apache.org/POM/4.0.0
		 http://maven.apache.org/xsd/maven-4.0.0.xsd">
	<modelVersion>4.0.0</modelVersion>
	<groupId>com.example</groupId>
	<artifactId>demo</artifactId>
	<version>1.0.0</version>
	<properties>
		<maven.compiler.source>11</maven.compiler.source>
		<maven.compiler.target>11</maven.compiler.target>
	</properties>
</project>`,
						"src/main/java/App.java": `package com.example;

public class App {
	public static void main(String[] args) {
		System.out.println("Hello from Java 11");
	}
}`,
					},
				}
				tarData, err := fixtures.CreateTarballFromFixture(fixture)
				require.NoError(t, err)
				return tarData
			},
			recipe: RecipeConfig{
				Recipe:    "org.openrewrite.java.migrate.UpgradeToJava17",
				Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
			},
			expectError: false,
			validate: func(t *testing.T, result *TransformResult) {
				assert.True(t, result.Success)
				assert.NotEmpty(t, result.Diff)
				assert.Equal(t, "maven", result.BuildSystem)
				assert.NotEmpty(t, result.JavaVersion)
				assert.Greater(t, result.Duration, time.Duration(0))
				
				// Check diff contains Java version upgrade
				diffStr := string(result.Diff)
				if len(diffStr) > 0 {
					// Might contain changes to pom.xml
					assert.True(t, 
						strings.Contains(diffStr, "17") || 
						strings.Contains(diffStr, "pom.xml"),
						"Diff should contain version upgrade markers")
				}
			},
		},
		{
			name:  "handles missing build system",
			jobID: "no-build-test",
			setupTar: func() []byte {
				fixture := &fixtures.ApplicationTar{
					Name:     "plain-java",
					Language: "java",
					Files: map[string]string{
						"App.java": `public class App {}`,
					},
				}
				tarData, err := fixtures.CreateTarballFromFixture(fixture)
				require.NoError(t, err)
				return tarData
			},
			recipe: RecipeConfig{
				Recipe:    "org.openrewrite.java.migrate.UpgradeToJava17",
				Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
			},
			expectError: true,
			validate: func(t *testing.T, result *TransformResult) {
				assert.False(t, result.Success)
				assert.NotEmpty(t, result.Error)
				assert.Contains(t, result.Error, "no supported build system")
			},
		},
		{
			name:  "handles context cancellation",
			jobID: "cancel-test",
			setupTar: func() []byte {
				// Create a large project that takes time to process
				files := make(map[string]string)
				files["pom.xml"] = `<?xml version="1.0"?>
<project>
	<groupId>com.example</groupId>
	<artifactId>large</artifactId>
	<version>1.0.0</version>
</project>`
				// Add many Java files
				for i := 0; i < 50; i++ {
					files[fmt.Sprintf("src/main/java/Class%d.java", i)] = fmt.Sprintf(`package com.example;
public class Class%d {
	// Large class with lots of code
	%s
}`, i, strings.Repeat("// Comment\n", 100))
				}
				
				fixture := &fixtures.ApplicationTar{
					Name:     "large-project",
					Language: "java",
					Files:    files,
				}
				tarData, err := fixtures.CreateTarballFromFixture(fixture)
				require.NoError(t, err)
				return tarData
			},
			recipe: RecipeConfig{
				Recipe:    "org.openrewrite.java.migrate.UpgradeToJava17",
				Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
			},
			expectError: true,
			validate: func(t *testing.T, result *TransformResult) {
				assert.False(t, result.Success)
				assert.Contains(t, result.Error, "context")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			config := DefaultConfig()
			config.WorkDir = t.TempDir()
			executor := NewExecutor(config)
			
			// Create context (with cancellation for specific test)
			ctx := context.Background()
			if tt.name == "handles context cancellation" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(context.Background(), 1*time.Millisecond)
				defer cancel()
			}
			
			tarData := tt.setupTar()
			
			// Execute
			result, err := executor.Execute(ctx, tt.jobID, tarData, tt.recipe)
			
			// Verify
			if tt.expectError {
				assert.Error(t, err)
				if result != nil {
					tt.validate(t, result)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestExecutorImpl_GenerateRewriteYaml(t *testing.T) {
	executor := &ExecutorImpl{
		config: DefaultConfig(),
	}

	tests := []struct {
		name     string
		recipe   RecipeConfig
		expected string
	}{
		{
			name: "generates basic recipe YAML",
			recipe: RecipeConfig{
				Recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
			},
			expected: `---
type: specs.openrewrite.org/v1beta/recipe
name: PloyTransformation
recipeList:
  - org.openrewrite.java.migrate.UpgradeToJava17
`,
		},
		{
			name: "generates recipe with options",
			recipe: RecipeConfig{
				Recipe: "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0",
				Options: map[string]string{
					"addLoggingShutdownHook": "true",
				},
			},
			expected: `---
type: specs.openrewrite.org/v1beta/recipe
name: PloyTransformation
recipeList:
  - org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := executor.generateRewriteYaml(tt.recipe)
			assert.Equal(t, tt.expected, yaml)
		})
	}
}

func TestExecutorImpl_BuildMavenCommand(t *testing.T) {
	executor := &ExecutorImpl{
		config: DefaultConfig(),
	}

	recipe := RecipeConfig{
		Recipe:    "org.openrewrite.java.migrate.UpgradeToJava17",
		Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
	}

	args := executor.buildMavenCommand(recipe)
	
	assert.Contains(t, args, "org.openrewrite.maven:rewrite-maven-plugin:5.34.0:run")
	assert.Contains(t, args, "-Drewrite.recipeArtifactCoordinates=org.openrewrite.recipe:rewrite-migrate-java:3.15.0")
	assert.Contains(t, args, "-Drewrite.activeRecipes=PloyTransformation")
}

func TestExecutorImpl_Cleanup(t *testing.T) {
	config := DefaultConfig()
	config.WorkDir = t.TempDir()
	executor := NewExecutor(config)

	// Create a test directory
	testDir := filepath.Join(config.WorkDir, "test-cleanup")
	require.NoError(t, os.MkdirAll(testDir, 0755))
	
	// Add some files
	testFile := filepath.Join(testDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))
	
	// Verify directory exists
	assert.DirExists(t, testDir)
	
	// Cleanup
	executor.(*ExecutorImpl).cleanup(testDir)
	
	// Verify directory is removed
	assert.NoDirExists(t, testDir)
}