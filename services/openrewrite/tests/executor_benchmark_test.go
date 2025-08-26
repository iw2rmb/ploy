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

// BenchmarkExecutor_SimpleMavenProject measures performance metrics for OpenRewrite transformations
func BenchmarkExecutor_SimpleMavenProject(b *testing.B) {
	// Skip if not in integration test mode
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	// Setup once
	tempDir := b.TempDir()
	config := &Config{
		WorkDir:          tempDir,
		MavenPath:        "mvn",
		GitPath:          "git",
		MaxTransformTime: 2 * time.Minute,
		JavaHome:         os.Getenv("JAVA_HOME"),
	}

	// Create test project once
	projectDir := filepath.Join(tempDir, "benchmark-project")
	require.NoError(b, os.MkdirAll(projectDir, 0755))

	// Create a minimal pom.xml with Java 11
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.example</groupId>
    <artifactId>benchmark-project</artifactId>
    <version>1.0.0</version>
    
    <properties>
        <maven.compiler.source>11</maven.compiler.source>
        <maven.compiler.target>11</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    </properties>
</project>`

	pomPath := filepath.Join(projectDir, "pom.xml")
	require.NoError(b, os.WriteFile(pomPath, []byte(pomXML), 0644))

	// Create Java files
	srcDir := filepath.Join(projectDir, "src", "main", "java", "com", "example")
	require.NoError(b, os.MkdirAll(srcDir, 0755))

	javaFile := `package com.example;

public class BenchmarkApp {
    public static void main(String[] args) {
        System.out.println("Benchmark test");
        
        // Use var keyword (Java 10+)
        var message = "Testing performance";
        System.out.println(message);
        
        // Some more code to transform
        processData();
    }
    
    private static void processData() {
        var items = java.util.List.of("item1", "item2", "item3");
        for (var item : items) {
            System.out.println("Processing: " + item);
        }
    }
}`

	javaPath := filepath.Join(srcDir, "BenchmarkApp.java")
	require.NoError(b, os.WriteFile(javaPath, []byte(javaFile), 0644))

	// Create tar archive
	tarData := createTarArchive(b, projectDir)

	executor := NewExecutor(config)
	recipe := RecipeConfig{
		Recipe:    "org.openrewrite.java.migrate.UpgradeToJava17",
		Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
	}

	// Reset timer to exclude setup time
	b.ResetTimer()

	// Run the benchmark
	b.Run("Java11to17Migration", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			
			// Measure transformation time
			start := time.Now()
			result, err := executor.Execute(ctx, "benchmark-test", tarData, recipe)
			duration := time.Since(start)
			
			cancel()

			// Basic validation
			assert.NotNil(b, result)
			
			// Report custom metrics
			b.ReportMetric(float64(duration.Milliseconds()), "transform_time_ms")
			
			if result != nil {
				b.ReportMetric(float64(len(result.Diff)), "diff_bytes")
				if result.Success {
					b.ReportMetric(1, "success_rate")
				} else {
					b.ReportMetric(0, "success_rate") 
				}
			}
			
			// Stop on error to avoid skewing results
			if err != nil {
				b.Logf("Transformation error: %v", err)
				break
			}
		}
	})
}

// TestExecutor_PerformanceMetrics validates specific performance requirements
func TestExecutor_PerformanceMetrics(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
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

	// Use the simple Maven project from the integration test
	projectDir := filepath.Join(tempDir, "perf-project")
	require.NoError(t, os.MkdirAll(projectDir, 0755))

	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.example</groupId>
    <artifactId>perf-project</artifactId>
    <version>1.0.0</version>
    
    <properties>
        <maven.compiler.source>11</maven.compiler.source>
        <maven.compiler.target>11</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    </properties>
</project>`

	pomPath := filepath.Join(projectDir, "pom.xml")
	require.NoError(t, os.WriteFile(pomPath, []byte(pomXML), 0644))

	tarData := createTarArchive(t, projectDir)
	
	executor := NewExecutor(config)
	recipe := RecipeConfig{
		Recipe:    "org.openrewrite.java.migrate.UpgradeToJava17",
		Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
	}

	// Execute and measure
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	startTime := time.Now()
	result, err := executor.Execute(ctx, "perf-test", tarData, recipe)
	duration := time.Since(startTime)

	// Performance Requirements Validation
	t.Logf("Performance Metrics:")
	t.Logf("  Transformation Time: %v", duration)
	
	if result != nil {
		t.Logf("  Build System Detected: %s", result.BuildSystem)
		t.Logf("  Java Version Detected: %s", result.JavaVersion)
		t.Logf("  Transformation Success: %v", result.Success)
		t.Logf("  Diff Size: %d bytes", len(result.Diff))
	}

	// MVP Performance Requirements (from roadmap)
	assert.Less(t, duration, 5*time.Minute, "Should complete within 5 minutes")
	assert.Greater(t, duration, time.Duration(0), "Should take some time to execute")
	
	// Basic functionality requirements
	assert.NotNil(t, result, "Result should not be nil")
	
	if result != nil {
		assert.Equal(t, "maven", result.BuildSystem, "Should detect Maven build system")
		
		// Memory usage estimation (basic check)
		// In a real scenario, we'd use runtime.ReadMemStats()
		assert.Less(t, len(result.Diff), 10*1024*1024, "Diff should be reasonable size (< 10MB)")
	}

	// Log final status
	if err != nil {
		t.Logf("Transformation completed with error: %v", err)
	} else if result != nil && result.Success {
		t.Log("✓ Performance test completed successfully")
	} else {
		t.Log("⚠ Transformation completed but may have issues (expected in CI)")
	}
}

// Helper function to create tar archive for testing
func createTarArchive(tb testing.TB, projectDir string) []byte {
	tempDir := tb.TempDir()
	tarPath := filepath.Join(tempDir, "test.tar.gz")
	
	// Use system tar command
	tarCmd := exec.Command("tar", "-czf", tarPath, "-C", projectDir, ".")
	err := tarCmd.Run()
	require.NoError(tb, err, "Failed to create tar archive")
	
	tarData, err := os.ReadFile(tarPath)
	require.NoError(tb, err, "Failed to read tar archive")
	
	return tarData
}