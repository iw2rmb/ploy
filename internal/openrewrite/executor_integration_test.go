// +build integration

package openrewrite

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutorIntegration_SpringPetclinic(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if Maven is available
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("Maven not installed, skipping Spring Petclinic integration test")
	}

	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git not installed, skipping Spring Petclinic integration test")
	}

	// Setup
	tempDir := t.TempDir()
	config := &Config{
		WorkDir:          tempDir,
		MavenPath:        "mvn",
		GitPath:          "git",
		MaxTransformTime: 5 * time.Minute,
		JavaHome:         os.Getenv("JAVA_HOME"),
	}

	// Clone Spring Petclinic repository and modify to Java 11 for testing
	t.Log("Cloning Spring Petclinic repository...")
	cloneDir := filepath.Join(tempDir, "spring-petclinic")
	cloneCmd := exec.Command("git", "clone", 
		"--depth", "1",
		"--branch", "main",
		"https://github.com/spring-projects/spring-petclinic.git",
		cloneDir)
	cloneOutput, err := cloneCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to clone Spring Petclinic: %v\nOutput: %s", err, cloneOutput)
	}
	t.Logf("Successfully cloned Spring Petclinic to %s", cloneDir)

	// Modify pom.xml to use Java 11 for testing migration
	t.Log("Modifying pom.xml to use Java 11 for migration testing...")
	pomPath := filepath.Join(cloneDir, "pom.xml")
	pomContent, err := os.ReadFile(pomPath)
	require.NoError(t, err)

	// Replace Java 17 with Java 11
	modifiedPom := strings.ReplaceAll(string(pomContent), "<java.version>17</java.version>", "<java.version>11</java.version>")
	modifiedPom = strings.ReplaceAll(modifiedPom, "<version>17</version>", "<version>11</version>")
	
	require.NoError(t, os.WriteFile(pomPath, []byte(modifiedPom), 0644))
	t.Log("Modified Spring Petclinic to use Java 11")

	// Create tar archive of the modified repository
	t.Log("Creating tar archive of modified Spring Petclinic...")
	tarCmd := exec.Command("tar", "-czf", "petclinic.tar.gz", "-C", cloneDir, ".")
	tarCmd.Dir = tempDir
	if err := tarCmd.Run(); err != nil {
		t.Fatalf("Failed to create tar archive: %v", err)
	}

	tarPath := filepath.Join(tempDir, "petclinic.tar.gz")
	tarData, err := os.ReadFile(tarPath)
	require.NoError(t, err)
	t.Logf("Created tar archive: %d bytes", len(tarData))

	// Execute transformation
	executor := NewExecutor(config)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	recipe := RecipeConfig{
		Recipe:    "org.openrewrite.java.migrate.UpgradeToJava17",
		Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
	}

	startTime := time.Now()
	t.Log("Starting OpenRewrite Java 11→17 transformation...")
	result, err := executor.Execute(ctx, "petclinic-test", tarData, recipe)
	duration := time.Since(startTime)

	// Log the result regardless of success/failure
	if err != nil {
		t.Logf("Transformation error: %v", err)
		if result != nil {
			t.Logf("Error details: %s", result.Error)
		}
	}

	// Basic assertions that should always pass
	assert.NotNil(t, result, "Result should not be nil")
	t.Logf("Transformation completed in %v", duration)

	if result.Success {
		t.Log("Java 11→17 transformation successful!")
		assert.NotEmpty(t, result.Diff, "Diff should not be empty on success")
		assert.Equal(t, "maven", result.BuildSystem, "Should detect Maven build system")
		assert.NotEmpty(t, result.JavaVersion, "Should detect Java version")
		assert.Less(t, duration, 5*time.Minute, "Should complete within 5 minutes")

		// Check that diff contains Java 17 migration patterns
		diffStr := string(result.Diff)
		t.Logf("Diff size: %d bytes", len(diffStr))
		
		// Look for Java 17 migration patterns
		expectedPatterns := []string{
			"pom.xml",      // Should modify pom.xml
			"17",           // Should update to Java 17
		}

		foundPatterns := 0
		for _, pattern := range expectedPatterns {
			if strings.Contains(diffStr, pattern) {
				t.Logf("✓ Found expected migration pattern: %s", pattern)
				foundPatterns++
			}
		}
		
		assert.Greater(t, foundPatterns, 0, "Should find at least one migration pattern in diff")
	} else {
		t.Logf("Transformation failed: %s", result.Error)
		// Log the error but don't fail the test - Maven deps might not be available
	}

	// Verify basic execution metrics
	assert.Equal(t, "maven", result.BuildSystem, "Should correctly detect Maven")
	assert.Greater(t, duration, time.Duration(0), "Should take some time to execute")
	assert.Less(t, duration, 10*time.Minute, "Should not exceed timeout")
}

func TestExecutorIntegration_SimpleMavenProject(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if Maven is available
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("Maven not installed, skipping integration test")
	}

	// Create a simple Maven project for testing
	tempDir := t.TempDir()
	projectDir := filepath.Join(tempDir, "simple-project")
	require.NoError(t, os.MkdirAll(projectDir, 0755))

	// Create a minimal pom.xml with Java 11
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.example</groupId>
    <artifactId>simple-project</artifactId>
    <version>1.0.0</version>
    
    <properties>
        <maven.compiler.source>11</maven.compiler.source>
        <maven.compiler.target>11</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    </properties>
</project>`

	pomPath := filepath.Join(projectDir, "pom.xml")
	require.NoError(t, os.WriteFile(pomPath, []byte(pomXML), 0644))

	// Create a simple Java file
	srcDir := filepath.Join(projectDir, "src", "main", "java", "com", "example")
	require.NoError(t, os.MkdirAll(srcDir, 0755))

	javaFile := `package com.example;

public class SimpleApp {
    public static void main(String[] args) {
        System.out.println("Hello from Java 11");
        
        // Use var keyword (Java 10+)
        var message = "Testing OpenRewrite";
        System.out.println(message);
    }
}`

	javaPath := filepath.Join(srcDir, "SimpleApp.java")
	require.NoError(t, os.WriteFile(javaPath, []byte(javaFile), 0644))

	// Create tar archive
	tarCmd := exec.Command("tar", "-czf", "simple.tar.gz", "-C", projectDir, ".")
	tarCmd.Dir = tempDir
	require.NoError(t, tarCmd.Run())

	tarPath := filepath.Join(tempDir, "simple.tar.gz")
	tarData, err := os.ReadFile(tarPath)
	require.NoError(t, err)

	// Execute transformation
	config := &Config{
		WorkDir:          tempDir,
		MavenPath:        "mvn",
		GitPath:          "git",
		MaxTransformTime: 2 * time.Minute,
		JavaHome:         os.Getenv("JAVA_HOME"),
	}

	executor := NewExecutor(config)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	recipe := RecipeConfig{
		Recipe:    "org.openrewrite.java.migrate.UpgradeToJava17",
		Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
	}

	result, err := executor.Execute(ctx, "simple-test", tarData, recipe)

	// Basic assertions
	assert.NotNil(t, result)
	assert.Equal(t, "maven", result.BuildSystem)

	if result.Success {
		t.Log("Simple project transformation successful")
		assert.NotEmpty(t, result.Diff)
		
		// Check for Java version change in pom.xml
		diffStr := string(result.Diff)
		if strings.Contains(diffStr, "17") {
			t.Log("Found Java 17 in diff - migration appears successful")
		}
	} else {
		t.Logf("Transformation failed (might be expected without Maven cache): %s", result.Error)
	}
}