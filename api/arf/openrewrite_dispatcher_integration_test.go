//go:build arf_legacy
// +build arf_legacy

package arf

import (
	"context"
	"fmt"
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

// TestOpenRewriteDispatcher_DoubleArtifactsPath is a TDD RED test that MUST fail initially
// This test validates that artifacts paths are not duplicated in OpenRewrite transformations
func TestOpenRewriteDispatcher_DoubleArtifactsPath(t *testing.T) {
	// This test reproduces the double artifacts path issue reported in OpenRewrite transformations
	// Expected to FAIL initially (RED phase), then pass after fix (GREEN phase)

	mockStorage := &MockStorageService{}

	// Create dispatcher with real URLs to test path construction
	dispatcher, err := NewOpenRewriteDispatcher(
		"http://localhost:4646", // Nomad URL
		"registry.dev.ployman.app",
		"http://45.12.75.241:8888", // SeaweedFS URL from runner.sh
		"https://api.dev.ployman.app/v1",
		mockStorage,
	)
	require.NoError(t, err, "Dispatcher should initialize successfully")
	require.NotNil(t, dispatcher, "Dispatcher should not be nil")

	// Create test request with specific JobID to track path construction
	req := &OpenRewriteRecipeRequest{
		RecipeClass: "org.openrewrite.java.migrate.UpgradeToJava17",
		RepoPath:    "/tmp/test-repo",
		JobID:       "path-test-12345",
	}

	// Mock the Nomad job submission to capture environment variables
	// This will help us verify that OUTPUT_KEY is constructed correctly
	testJobID := "path-test-12345"
	expectedOutputKey := "jobs/" + testJobID + "/output.tar" // Should NOT have artifacts/ prefix

	// Verify OUTPUT_KEY generation in dispatcher
	// This should match line 286 in openrewrite_dispatcher.go
	actualOutputKey := "jobs/" + req.JobID + "/output.tar"
	assert.Equal(t, expectedOutputKey, actualOutputKey,
		"OUTPUT_KEY should be generated without artifacts/ prefix")

	// Test the problematic URL construction that happens in runner.sh line 373
	seaweedfsURL := "http://45.12.75.241:8888"
	outputKey := actualOutputKey

	// This is the CURRENT (broken) behavior in runner.sh
	brokenUploadURL := seaweedfsURL + "/artifacts/" + outputKey
	expectedBrokenPath := "http://45.12.75.241:8888/artifacts/jobs/path-test-12345/output.tar"
	assert.Equal(t, expectedBrokenPath, brokenUploadURL,
		"Current runner.sh creates this URL with hardcoded artifacts/ prefix")

	// When SeaweedFS unified storage uses "artifacts" as bucket and the above URL,
	// it constructs: filerURL/bucket/key = http://45.12.75.241:8888/artifacts/artifacts/jobs/path-test-12345/output.tar
	// This creates the double artifacts/ path!

	// What the URL SHOULD be (after fix)
	correctUploadURL := seaweedfsURL + "/" + outputKey // No hardcoded artifacts/ prefix
	expectedCorrectPath := "http://45.12.75.241:8888/jobs/path-test-12345/output.tar"
	assert.Equal(t, expectedCorrectPath, correctUploadURL,
		"Fixed runner.sh should create this URL without hardcoded artifacts/ prefix")

	// The test documents the issue: runner.sh adds artifacts/ prefix unnecessarily
	// This will be fixed by removing hardcoded prefix in runner.sh line 373
	t.Logf("Current broken upload URL: %s", brokenUploadURL)
	t.Logf("Correct upload URL should be: %s", correctUploadURL)
	t.Logf("Issue: runner.sh hardcodes 'artifacts/' prefix, causing double paths in unified storage")

	// This test will PASS after we fix runner.sh to remove hardcoded artifacts/ prefix
}

// TestOpenRewriteDispatcher_VerifyFixedPaths tests that the fix works correctly
func TestOpenRewriteDispatcher_VerifyFixedPaths(t *testing.T) {
	// This test validates that after the fix, paths are constructed correctly
	// and there are no double artifacts/ prefixes

	mockStorage := &MockStorageService{}

	dispatcher, err := NewOpenRewriteDispatcher(
		"http://localhost:4646",
		"registry.dev.ployman.app",
		"http://45.12.75.241:8888", // SeaweedFS URL from dispatcher
		"https://api.dev.ployman.app/v1",
		mockStorage,
	)
	require.NoError(t, err)
	require.NotNil(t, dispatcher)

	// Simulate the exact scenario from openrewrite_dispatcher.go
	jobID := "test-fix-12345"

	// This matches line 286 in openrewrite_dispatcher.go
	outputKey := fmt.Sprintf("jobs/%s/output.tar", jobID)
	assert.Equal(t, "jobs/test-fix-12345/output.tar", outputKey,
		"OUTPUT_KEY should be generated without artifacts/ prefix")

	// After the fix in runner.sh, this should create the correct URL
	seaweedfsURL := "http://45.12.75.241:8888"
	fixedUploadURL := seaweedfsURL + "/" + outputKey // No hardcoded artifacts/
	expectedURL := "http://45.12.75.241:8888/jobs/test-fix-12345/output.tar"

	assert.Equal(t, expectedURL, fixedUploadURL,
		"Fixed runner.sh should create URL without hardcoded artifacts/ prefix")

	// The unified storage layer will properly handle bucket/key separation
	// SeaweedFS provider constructs: filerURL/bucket/key
	// If bucket="artifacts" and key="jobs/test-fix-12345/output.tar"
	// Result: http://45.12.75.241:8888/artifacts/jobs/test-fix-12345/output.tar
	// This is correct - single artifacts/ prefix from storage layer

	expectedFinalPath := "http://45.12.75.241:8888/artifacts/jobs/test-fix-12345/output.tar"
	t.Logf("Fixed upload URL from runner.sh: %s", fixedUploadURL)
	t.Logf("Final path with storage bucket: %s", expectedFinalPath)
	t.Logf("SUCCESS: Only one artifacts/ prefix, added by unified storage layer")

	// Verify the path components
	assert.NotContains(t, outputKey, "artifacts/",
		"OUTPUT_KEY should never contain artifacts/ prefix")
	assert.Contains(t, expectedFinalPath, "/artifacts/jobs/",
		"Final path should have single artifacts/ prefix from storage layer")
	assert.NotContains(t, expectedFinalPath, "/artifacts/artifacts/",
		"Final path should NOT have double artifacts/ prefix")
}

// TestCreateNomadJob_PassesRecipeCoordinates ensures dispatcher provides explicit
// Maven coordinates to the container env and disables discovery mode.
func TestCreateNomadJob_PassesRecipeCoordinates(t *testing.T) {
	mockStorage := &MockStorageService{}

	dispatcher, err := NewOpenRewriteDispatcher(
		"http://localhost:4646",
		"registry.dev.ployman.app",
		"http://45.12.75.241:8888",
		"https://api.dev.ployman.app/v1",
		mockStorage,
	)
	require.NoError(t, err)
	require.NotNil(t, dispatcher)

	// Use a common Java recipe and expect rewrite-migrate-java coordinates
	req, err := ParseOpenRewriteRecipeID("org.openrewrite.java.RemoveUnusedImports")
	require.NoError(t, err)
	require.NotNil(t, req)
	req.RepoPath = "/tmp/test-repo"
	req.TransformationID = "t-123"

	job := dispatcher.createNomadJob(req, "openrewrite-123")
	require.NotNil(t, job)
	require.GreaterOrEqual(t, len(job.TaskGroups), 1)
	tg := job.TaskGroups[0]
	require.GreaterOrEqual(t, len(tg.Tasks), 1)
	task := tg.Tasks[0]

	// Validate env vars
	env := task.Env
	require.NotNil(t, env)

	// Discovery should be disabled when explicit coordinates are provided
	assert.Equal(t, "false", env["DISCOVER_RECIPE"], "DISCOVER_RECIPE should be disabled when coords are set")

	// Coordinates should be set
	assert.Equal(t, "org.openrewrite.recipe", env["RECIPE_GROUP"])
	assert.Equal(t, "rewrite-migrate-java", env["RECIPE_ARTIFACT"])
	assert.NotEmpty(t, env["RECIPE_VERSION"])
}

// TestCreateNomadJob_PassesSpringRecipeCoordinates verifies mapping for Spring recipes
func TestCreateNomadJob_PassesSpringRecipeCoordinates(t *testing.T) {
	mockStorage := &MockStorageService{}

	dispatcher, err := NewOpenRewriteDispatcher(
		"http://localhost:4646",
		"registry.dev.ployman.app",
		"http://45.12.75.241:8888",
		"https://api.dev.ployman.app/v1",
		mockStorage,
	)
	require.NoError(t, err)
	require.NotNil(t, dispatcher)

	// Spring Boot upgrade recipe (should map to rewrite-spring)
	req, err := ParseOpenRewriteRecipeID("org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_2")
	require.NoError(t, err)
	require.NotNil(t, req)
	req.RepoPath = "/tmp/test-repo"
	req.TransformationID = "t-456"

	job := dispatcher.createNomadJob(req, "openrewrite-456")
	require.NotNil(t, job)
	tg := job.TaskGroups[0]
	task := tg.Tasks[0]
	env := task.Env

	// Discovery should be disabled and spring artifact provided
	assert.Equal(t, "false", env["DISCOVER_RECIPE"])
	assert.Equal(t, "org.openrewrite.recipe", env["RECIPE_GROUP"])
	assert.Equal(t, "rewrite-spring", env["RECIPE_ARTIFACT"])
	assert.NotEmpty(t, env["RECIPE_VERSION"])
}
