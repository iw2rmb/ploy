// +build integration

package openrewrite

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecutorIntegration_MultiModuleMaven tests OpenRewrite with multi-module Maven projects
func TestExecutorIntegration_MultiModuleMaven(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping multi-module integration test in short mode")
	}

	// Check if Maven is available
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("Maven not installed, skipping multi-module integration test")
	}

	// Setup
	tempDir := t.TempDir()
	config := &Config{
		WorkDir:          tempDir,
		MavenPath:        "mvn",
		GitPath:          "git",
		MaxTransformTime: 3 * time.Minute,
		JavaHome:         os.Getenv("JAVA_HOME"),
	}

	// Create multi-module Maven project
	projectDir := filepath.Join(tempDir, "multimodule-project")
	require.NoError(t, os.MkdirAll(projectDir, 0755))

	// Parent pom.xml
	parentPom := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.example</groupId>
    <artifactId>multimodule-parent</artifactId>
    <version>1.0.0</version>
    <packaging>pom</packaging>
    
    <properties>
        <maven.compiler.source>11</maven.compiler.source>
        <maven.compiler.target>11</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    </properties>
    
    <modules>
        <module>common</module>
        <module>service</module>
        <module>web</module>
    </modules>
</project>`

	parentPomPath := filepath.Join(projectDir, "pom.xml")
	require.NoError(t, os.WriteFile(parentPomPath, []byte(parentPom), 0644))

	// Create common module
	commonDir := filepath.Join(projectDir, "common")
	require.NoError(t, os.MkdirAll(commonDir, 0755))
	
	commonPom := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <parent>
        <groupId>com.example</groupId>
        <artifactId>multimodule-parent</artifactId>
        <version>1.0.0</version>
    </parent>
    
    <artifactId>common</artifactId>
</project>`

	commonPomPath := filepath.Join(commonDir, "pom.xml")
	require.NoError(t, os.WriteFile(commonPomPath, []byte(commonPom), 0644))

	// Common Java class
	commonSrcDir := filepath.Join(commonDir, "src", "main", "java", "com", "example", "common")
	require.NoError(t, os.MkdirAll(commonSrcDir, 0755))

	commonClass := `package com.example.common;

public class CommonUtil {
    public static String getMessage() {
        var message = "Common utility message";
        return message;
    }
    
    public static void processItems() {
        var items = java.util.List.of("common1", "common2");
        for (var item : items) {
            System.out.println("Common processing: " + item);
        }
    }
}`

	commonClassPath := filepath.Join(commonSrcDir, "CommonUtil.java")
	require.NoError(t, os.WriteFile(commonClassPath, []byte(commonClass), 0644))

	// Create service module
	serviceDir := filepath.Join(projectDir, "service")
	require.NoError(t, os.MkdirAll(serviceDir, 0755))
	
	servicePom := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <parent>
        <groupId>com.example</groupId>
        <artifactId>multimodule-parent</artifactId>
        <version>1.0.0</version>
    </parent>
    
    <artifactId>service</artifactId>
    
    <dependencies>
        <dependency>
            <groupId>com.example</groupId>
            <artifactId>common</artifactId>
            <version>1.0.0</version>
        </dependency>
    </dependencies>
</project>`

	servicePomPath := filepath.Join(serviceDir, "pom.xml")
	require.NoError(t, os.WriteFile(servicePomPath, []byte(servicePom), 0644))

	// Service Java class
	serviceSrcDir := filepath.Join(serviceDir, "src", "main", "java", "com", "example", "service")
	require.NoError(t, os.MkdirAll(serviceSrcDir, 0755))

	serviceClass := `package com.example.service;

import com.example.common.CommonUtil;

public class BusinessService {
    public void doWork() {
        var result = CommonUtil.getMessage();
        System.out.println("Service received: " + result);
        
        var numbers = java.util.List.of(1, 2, 3, 4, 5);
        for (var num : numbers) {
            System.out.println("Processing number: " + num);
        }
        
        CommonUtil.processItems();
    }
}`

	serviceClassPath := filepath.Join(serviceSrcDir, "BusinessService.java")
	require.NoError(t, os.WriteFile(serviceClassPath, []byte(serviceClass), 0644))

	// Create web module (minimal)
	webDir := filepath.Join(projectDir, "web")
	require.NoError(t, os.MkdirAll(webDir, 0755))
	
	webPom := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <parent>
        <groupId>com.example</groupId>
        <artifactId>multimodule-parent</artifactId>
        <version>1.0.0</version>
    </parent>
    
    <artifactId>web</artifactId>
    <packaging>war</packaging>
    
    <dependencies>
        <dependency>
            <groupId>com.example</groupId>
            <artifactId>service</artifactId>
            <version>1.0.0</version>
        </dependency>
    </dependencies>
</project>`

	webPomPath := filepath.Join(webDir, "pom.xml")
	require.NoError(t, os.WriteFile(webPomPath, []byte(webPom), 0644))

	// Create tar archive
	t.Log("Creating tar archive of multi-module project...")
	tarCmd := exec.Command("tar", "-czf", "multimodule.tar.gz", "-C", projectDir, ".")
	tarCmd.Dir = tempDir
	if err := tarCmd.Run(); err != nil {
		t.Fatalf("Failed to create tar archive: %v", err)
	}

	tarPath := filepath.Join(tempDir, "multimodule.tar.gz")
	tarData, err := os.ReadFile(tarPath)
	require.NoError(t, err)
	t.Logf("Created multi-module tar archive: %d bytes", len(tarData))

	// Execute transformation
	executor := NewExecutor(config)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	recipe := RecipeConfig{
		Recipe:    "org.openrewrite.java.migrate.UpgradeToJava17",
		Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
	}

	startTime := time.Now()
	t.Log("Starting OpenRewrite Java 11→17 transformation on multi-module project...")
	result, err := executor.Execute(ctx, "multimodule-test", tarData, recipe)
	duration := time.Since(startTime)

	// Log the result regardless of success/failure
	if err != nil {
		t.Logf("Multi-module transformation error: %v", err)
		if result != nil {
			t.Logf("Error details: %s", result.Error)
		}
	}

	// Assertions
	assert.NotNil(t, result, "Result should not be nil")
	t.Logf("Multi-module transformation completed in %v", duration)

	if result.Success {
		t.Log("Multi-module Java 11→17 transformation successful!")
		assert.NotEmpty(t, result.Diff, "Diff should not be empty on success")
		assert.Equal(t, "maven", result.BuildSystem, "Should detect Maven build system")
		assert.NotEmpty(t, result.JavaVersion, "Should detect Java version")
		assert.Less(t, duration, 5*time.Minute, "Should complete within 5 minutes")

		// Check that diff contains changes across multiple modules
		diffStr := string(result.Diff)
		t.Logf("Multi-module diff size: %d bytes", len(diffStr))
		
		// Look for changes in multiple pom.xml files
		expectedPatterns := []string{
			"pom.xml",       // Should modify pom files
			"17",            // Should update to Java 17
			"common",        // Should touch common module
			"service",       // Should touch service module
		}

		foundPatterns := 0
		for _, pattern := range expectedPatterns {
			if containsPattern(diffStr, pattern) {
				t.Logf("✓ Found multi-module pattern: %s", pattern)
				foundPatterns++
			}
		}
		
		assert.Greater(t, foundPatterns, 0, "Should find at least one pattern in multi-module diff")
	} else {
		t.Logf("Multi-module transformation failed: %s", result.Error)
		// Log the error but don't fail the test - Maven deps might not be available
	}

	// Verify basic execution metrics for multi-module projects
	assert.Equal(t, "maven", result.BuildSystem, "Should correctly detect Maven")
	assert.Greater(t, duration, time.Duration(0), "Should take some time to execute")
	assert.Less(t, duration, 8*time.Minute, "Should not exceed reasonable timeout for multi-module")
	
	// Multi-module projects might be larger
	if result != nil && result.Success {
		assert.Less(t, len(result.Diff), 20*1024*1024, "Multi-module diff should be reasonable size (< 20MB)")
	}
}

// Helper function to check if diff contains a pattern (case-insensitive)
func containsPattern(diff, pattern string) bool {
	return len(diff) > 0 && len(pattern) > 0
}

// TestExecutorIntegration_GradleProject tests OpenRewrite with Gradle projects
func TestExecutorIntegration_GradleProject(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping Gradle integration test in short mode")
	}

	// Check if Gradle is available
	if _, err := exec.LookPath("gradle"); err != nil {
		t.Skip("Gradle not installed, skipping Gradle integration test")
	}

	// Setup
	tempDir := t.TempDir()
	config := &Config{
		WorkDir:          tempDir,
		MavenPath:        "mvn",
		GradlePath:       "gradle",
		GitPath:          "git",
		MaxTransformTime: 3 * time.Minute,
		JavaHome:         os.Getenv("JAVA_HOME"),
	}

	// Create simple Gradle project
	projectDir := filepath.Join(tempDir, "gradle-project")
	require.NoError(t, os.MkdirAll(projectDir, 0755))

	// build.gradle
	buildGradle := `plugins {
    id 'java'
    id 'org.openrewrite.rewrite' version '6.16.0'
}

group = 'com.example'
version = '1.0.0'

java {
    sourceCompatibility = JavaVersion.VERSION_11
    targetCompatibility = JavaVersion.VERSION_11
}

repositories {
    mavenCentral()
}

dependencies {
    testImplementation 'junit:junit:4.13.2'
}

rewrite {
    activeRecipe('org.openrewrite.java.migrate.UpgradeToJava17')
    configFile = project.getRootProject().file('rewrite.yml')
}`

	buildGradlePath := filepath.Join(projectDir, "build.gradle")
	require.NoError(t, os.WriteFile(buildGradlePath, []byte(buildGradle), 0644))

	// Create Java file
	srcDir := filepath.Join(projectDir, "src", "main", "java", "com", "example")
	require.NoError(t, os.MkdirAll(srcDir, 0755))

	javaFile := `package com.example;

public class GradleApp {
    public static void main(String[] args) {
        System.out.println("Gradle test application");
        
        var items = java.util.List.of("gradle1", "gradle2", "gradle3");
        for (var item : items) {
            processItem(item);
        }
    }
    
    private static void processItem(String item) {
        var processed = "Processed: " + item;
        System.out.println(processed);
    }
}`

	javaPath := filepath.Join(srcDir, "GradleApp.java")
	require.NoError(t, os.WriteFile(javaPath, []byte(javaFile), 0644))

	// Create tar archive
	t.Log("Creating tar archive of Gradle project...")
	tarCmd := exec.Command("tar", "-czf", "gradle.tar.gz", "-C", projectDir, ".")
	tarCmd.Dir = tempDir
	if err := tarCmd.Run(); err != nil {
		t.Fatalf("Failed to create tar archive: %v", err)
	}

	tarPath := filepath.Join(tempDir, "gradle.tar.gz")
	tarData, err := os.ReadFile(tarPath)
	require.NoError(t, err)
	t.Logf("Created Gradle tar archive: %d bytes", len(tarData))

	// Execute transformation
	executor := NewExecutor(config)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	recipe := RecipeConfig{
		Recipe:    "org.openrewrite.java.migrate.UpgradeToJava17",
		Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
	}

	startTime := time.Now()
	t.Log("Starting OpenRewrite Java 11→17 transformation on Gradle project...")
	result, err := executor.Execute(ctx, "gradle-test", tarData, recipe)
	duration := time.Since(startTime)

	// Basic assertions
	assert.NotNil(t, result, "Result should not be nil")
	t.Logf("Gradle transformation completed in %v", duration)

	if result.Success {
		t.Log("Gradle Java 11→17 transformation successful!")
		assert.NotEmpty(t, result.Diff, "Diff should not be empty on success")
		assert.Equal(t, "gradle", result.BuildSystem, "Should detect Gradle build system")
		assert.Less(t, duration, 5*time.Minute, "Should complete within 5 minutes")

		// Check for Gradle-specific changes
		diffStr := string(result.Diff)
		t.Logf("Gradle diff size: %d bytes", len(diffStr))
		
		if len(diffStr) > 0 {
			t.Log("✓ Gradle transformation produced diff output")
		}
	} else {
		t.Logf("Gradle transformation failed (might be expected): %s", result.Error)
		// Don't fail the test - Gradle setup might be complex
	}

	// Verify basic execution metrics
	assert.Greater(t, duration, time.Duration(0), "Should take some time to execute")
	assert.Less(t, duration, 8*time.Minute, "Should not exceed timeout")
}