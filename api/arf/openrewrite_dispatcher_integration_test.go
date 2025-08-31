package arf

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/testing/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOpenRewriteDispatcher_Integration is a TDD RED test that MUST fail initially
// This test validates that the OpenRewrite dispatcher actually works end-to-end
// Unlike the existing MockEngine tests, this test will expose real issues
func TestOpenRewriteDispatcher_Integration(t *testing.T) {
	// Skip if not in integration test mode or missing environment
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check for required environment variables
	nomadAddr := os.Getenv("NOMAD_ADDR")
	if nomadAddr == "" {
		nomadAddr = "http://localhost:4646" // Default for local testing
	}

	seaweedfsURL := os.Getenv("SEAWEEDFS_URL")
	if seaweedfsURL == "" {
		seaweedfsURL = "http://seaweedfs-filer.service.consul:8888" // Default
	}

	t.Logf("Integration test running with Nomad: %s, SeaweedFS: %s", nomadAddr, seaweedfsURL)

	// Create test repository with Java code
	repoPath := helpers.CreateTempDir(t, "integration-test-repo")
	defer os.RemoveAll(repoPath)

	// Create a simple Maven project that OpenRewrite can process
	pomContent := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.test</groupId>
    <artifactId>integration-test</artifactId>
    <version>1.0.0</version>
    <properties>
        <maven.compiler.source>8</maven.compiler.source>
        <maven.compiler.target>8</maven.compiler.target>
    </properties>
</project>`
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "pom.xml"), []byte(pomContent), 0644))

	// Create Java source file with old-style code
	srcDir := filepath.Join(repoPath, "src", "main", "java", "com", "test")
	require.NoError(t, os.MkdirAll(srcDir, 0755))

	javaContent := `package com.test;

import java.util.ArrayList;
import java.util.List;

public class TestApp {
    // This should be detected and potentially transformed by OpenRewrite
    private List<String> names = new ArrayList<String>();
    
    public void addName(String name) {
        if (name != null && !name.isEmpty()) {
            names.add(name);
        }
    }
    
    public static void main(String[] args) {
        System.out.println("Test application");
    }
}`
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "TestApp.java"), []byte(javaContent), 0644))

	// Create mock storage service
	mockStorage := &MockStorageService{}

	// Create OpenRewrite dispatcher - this should work if infrastructure is available
	dispatcher, err := NewOpenRewriteDispatcher(
		nomadAddr,
		"registry.dev.ployman.app", // Registry URL
		seaweedfsURL,
		"https://api.dev.ployman.app/v1", // API URL
		mockStorage,
	)

	// This assertion will FAIL if dispatcher initialization fails (RED phase of TDD)
	require.NoError(t, err, "Dispatcher initialization should succeed")
	require.NotNil(t, dispatcher, "Dispatcher should not be nil")

	// Test a simple OpenRewrite recipe
	req := &OpenRewriteRecipeRequest{
		RecipeClass: "org.openrewrite.java.cleanup.UnnecessaryThrows",
		RepoPath:    repoPath,
		JobID:       "integration-test-job",
	}

	// Execute with reasonable timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// This is where the current system fails - the dispatcher times out
	result, err := dispatcher.ExecuteOpenRewriteRecipe(ctx, req)

	// TDD RED: These assertions MUST fail initially to expose the real issues
	require.NoError(t, err, "OpenRewrite execution should not error")
	require.NotNil(t, result, "Result should not be nil")
	assert.True(t, result.Success, "Transformation should be successful")

	// Verify that this is NOT a mock result
	if len(result.FilesModified) > 0 {
		assert.NotEqual(t, "MockFile.java", result.FilesModified[0],
			"Should not return mock files - this indicates real execution")
	}

	// Verify real transformation occurred (or at least real execution was attempted)
	assert.NotEmpty(t, result.RecipeID, "Result should have recipe ID")
	assert.Greater(t, result.ExecutionTime, time.Duration(0), "Should have execution time")

	t.Logf("Integration test completed successfully with result: %+v", result)
}

// TestOpenRewriteDispatcher_MissingInfrastructure tests behavior when infrastructure is not available
func TestOpenRewriteDispatcher_MissingInfrastructure(t *testing.T) {
	// Test with invalid Nomad address to simulate infrastructure issues
	mockStorage := &MockStorageService{}

	dispatcher, err := NewOpenRewriteDispatcher(
		"http://invalid-nomad:4646", // Invalid Nomad
		"registry.dev.ployman.app",
		"http://invalid-seaweedfs:8888", // Invalid SeaweedFS
		"https://api.dev.ployman.app/v1",
		mockStorage,
	)

	// Dispatcher creation might succeed (Nomad client is lazy)
	if err != nil {
		t.Logf("Expected: Dispatcher creation failed with invalid infrastructure: %v", err)
		return
	}

	// Create minimal test request
	req := &OpenRewriteRecipeRequest{
		RecipeClass: "org.openrewrite.java.cleanup.UnnecessaryThrows",
		RepoPath:    "/tmp/nonexistent",
		JobID:       "test-failure",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// This should fail gracefully with meaningful error
	result, err := dispatcher.ExecuteOpenRewriteRecipe(ctx, req)

	assert.Error(t, err, "Should fail with infrastructure unavailable")
	assert.Nil(t, result, "Should not return result on failure")
	assert.Contains(t, err.Error(), "failed", "Error should indicate failure reason")

	t.Logf("Expected failure occurred: %v", err)
}

// MockStorageService implements storage.StorageService for testing
type MockStorageService struct{}

func (m *MockStorageService) Put(ctx context.Context, key string, data []byte) error {
	// Simulate successful storage
	return nil
}

func (m *MockStorageService) Get(ctx context.Context, key string) ([]byte, error) {
	// Return mock tar data
	return []byte("mock tar data"), nil
}

func (m *MockStorageService) Delete(ctx context.Context, key string) error {
	return nil
}

func (m *MockStorageService) Exists(ctx context.Context, key string) (bool, error) {
	return true, nil
}

func (m *MockStorageService) List(ctx context.Context, prefix string) ([]string, error) {
	return []string{}, nil
}
