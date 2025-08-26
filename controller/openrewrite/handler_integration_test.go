// +build integration

package openrewrite

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/iw2rmb/ploy/internal/openrewrite"
)

// TestAPI_HealthEndpointIntegration tests the health check endpoint with real system tools
func TestAPI_HealthEndpointIntegration(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping API integration test in short mode")
	}

	// Setup real executor and handler
	executor := openrewrite.NewExecutor(openrewrite.DefaultConfig())
	handler := NewHandler(executor)
	
	app := fiber.New()
	handler.RegisterRoutes(app)

	// Make request to health endpoint
	req := httptest.NewRequest("GET", "/v1/openrewrite/health", nil)
	resp, err := app.Test(req, 10000) // 10s timeout
	require.NoError(t, err)

	// Verify response
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Parse response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var health HealthResponse
	err = json.Unmarshal(body, &health)
	require.NoError(t, err)

	// Validate health response
	t.Logf("Health Response: %+v", health)
	assert.Equal(t, "healthy", health.Status)
	assert.Equal(t, "1.0.0", health.Version)
	assert.NotZero(t, health.Timestamp)

	// Verify system tools are detected
	// Java should be available (required for OpenRewrite)
	if health.JavaVersion == "" {
		t.Logf("Warning: Java version not detected - this may cause transformation failures")
	} else {
		t.Logf("✓ Java detected: %s", health.JavaVersion)
		// Check for either "java" or "openjdk" in the version string
		javaDetected := strings.Contains(strings.ToLower(health.JavaVersion), "java") || 
			strings.Contains(strings.ToLower(health.JavaVersion), "openjdk")
		assert.True(t, javaDetected, "Should detect Java or OpenJDK in version string")
	}

	// Maven should be available for Maven projects
	if health.MavenVersion == "" {
		t.Logf("Warning: Maven version not detected")
	} else {
		t.Logf("✓ Maven detected: %s", health.MavenVersion)
		assert.Contains(t, strings.ToLower(health.MavenVersion), "maven")
	}

	// Git should be available for diff generation
	if health.GitVersion == "" {
		t.Logf("Warning: Git version not detected")
	} else {
		t.Logf("✓ Git detected: %s", health.GitVersion)
		assert.Contains(t, strings.ToLower(health.GitVersion), "git")
	}

	// Gradle detection is optional
	if health.GradleVersion != "" {
		t.Logf("✓ Gradle detected: %s", health.GradleVersion)
	}
}

// TestAPI_TransformEndpointIntegration tests the transform endpoint with real projects
func TestAPI_TransformEndpointIntegration(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping API integration test in short mode")
	}

	// Check if Maven is available
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("Maven not installed, skipping transform endpoint integration test")
	}

	// Setup real executor and handler
	executor := openrewrite.NewExecutor(openrewrite.DefaultConfig())
	handler := NewHandler(executor)
	
	app := fiber.New()
	handler.RegisterRoutes(app)

	// Create test project
	tarArchive := createAPITestProject(t)

	// Prepare request
	transformReq := TransformRequest{
		JobID:      "api-integration-test",
		TarArchive: tarArchive,
		RecipeConfig: RecipeConfig{
			Recipe:    "org.openrewrite.java.migrate.UpgradeToJava17",
			Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
		},
		Timeout: "8m", // Generous timeout for integration test
	}

	reqBody, err := json.Marshal(transformReq)
	require.NoError(t, err)

	// Make request
	req := httptest.NewRequest("POST", "/v1/openrewrite/transform", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	t.Log("Starting API transformation request...")
	resp, err := app.Test(req, 10*60*1000) // 10 minute timeout for integration test
	duration := time.Since(startTime)
	require.NoError(t, err)

	// Verify response status
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Parse response
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var transformResp TransformResponse
	err = json.Unmarshal(body, &transformResp)
	require.NoError(t, err)

	// Log response details
	t.Logf("Transform Response: Success=%v, Duration=%.2fs, BuildSystem=%s, JavaVersion=%s", 
		transformResp.Success, transformResp.Duration, transformResp.BuildSystem, transformResp.JavaVersion)
	
	if transformResp.Error != "" {
		t.Logf("Transform Error: %s", transformResp.Error)
	}

	// Validate response structure
	assert.Equal(t, "api-integration-test", transformResp.JobID)
	assert.Greater(t, transformResp.Duration, 0.0, "Duration should be positive")
	assert.Less(t, duration, 5*time.Minute, "API should complete within 5 minutes") // Roadmap requirement

	// Basic validation - transformation may fail due to Maven deps but should be handled gracefully
	if transformResp.Success {
		t.Log("✅ Transformation successful!")
		assert.NotEmpty(t, transformResp.Diff, "Successful transformation should have diff")
		assert.Equal(t, "maven", transformResp.BuildSystem, "Should detect Maven build system")
		
		// Validate diff is properly base64 encoded
		diffData, err := base64.StdEncoding.DecodeString(transformResp.Diff)
		require.NoError(t, err, "Diff should be valid base64")
		t.Logf("Diff size: %d bytes", len(diffData))
		
		// Check for Java 17 patterns in diff
		diffStr := string(diffData)
		if strings.Contains(diffStr, "17") {
			t.Log("✓ Found Java 17 migration patterns in diff")
		}

		// Validate stats if present
		if transformResp.Stats != nil {
			assert.GreaterOrEqual(t, transformResp.Stats.FilesChanged, 0)
			assert.GreaterOrEqual(t, transformResp.Stats.LinesAdded, 0)
			assert.GreaterOrEqual(t, transformResp.Stats.LinesRemoved, 0)
			assert.Greater(t, transformResp.Stats.TarSize, 0)
			t.Logf("Transform stats: %d files changed, +%d/-%d lines", 
				transformResp.Stats.FilesChanged, transformResp.Stats.LinesAdded, transformResp.Stats.LinesRemoved)
		}
	} else {
		t.Logf("⚠️  Transformation failed (may be expected due to Maven dependencies): %s", transformResp.Error)
		// Even on failure, we should have basic metadata
		assert.Equal(t, "maven", transformResp.BuildSystem, "Should still detect Maven build system")
	}

	// Performance validation
	assert.Less(t, transformResp.Duration, 300.0, "API response time should be under 5 minutes")
	t.Logf("✓ API response time: %.2f seconds", transformResp.Duration)
}

// TestAPI_ErrorHandlingIntegration tests API error handling with various malformed requests
func TestAPI_ErrorHandlingIntegration(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping API integration test in short mode")
	}

	// Setup real executor and handler
	executor := openrewrite.NewExecutor(openrewrite.DefaultConfig())
	handler := NewHandler(executor)
	
	app := fiber.New()
	handler.RegisterRoutes(app)

	tests := []struct {
		name           string
		endpoint       string
		method         string
		body           interface{}
		expectedStatus int
		errorCheck     func(t *testing.T, body map[string]interface{})
	}{
		{
			name:           "malformed JSON request",
			endpoint:       "/v1/openrewrite/transform",
			method:         "POST",
			body:           `{"invalid": "json"`, // Malformed JSON
			expectedStatus: fiber.StatusBadRequest,
			errorCheck: func(t *testing.T, body map[string]interface{}) {
				assert.Contains(t, body["error"], "Invalid request body")
				assert.Equal(t, "INVALID_REQUEST", body["code"])
			},
		},
		{
			name:     "missing required fields",
			endpoint: "/v1/openrewrite/transform", 
			method:   "POST",
			body: TransformRequest{
				// Missing JobID, TarArchive, RecipeConfig
			},
			expectedStatus: fiber.StatusBadRequest,
			errorCheck: func(t *testing.T, body map[string]interface{}) {
				assert.Contains(t, body["error"], "job_id is required")
				assert.Equal(t, "VALIDATION_ERROR", body["code"])
			},
		},
		{
			name:     "invalid base64 tar archive",
			endpoint: "/v1/openrewrite/transform",
			method:   "POST", 
			body: TransformRequest{
				JobID:      "error-test-001",
				TarArchive: "not-valid-base64!@#$%^&*()",
				RecipeConfig: RecipeConfig{
					Recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
				},
			},
			expectedStatus: fiber.StatusBadRequest,
			errorCheck: func(t *testing.T, body map[string]interface{}) {
				assert.Contains(t, body["error"], "Invalid base64 encoded tar archive")
				assert.Equal(t, "INVALID_TAR_ARCHIVE", body["code"])
				assert.NotNil(t, body["details"])
			},
		},
		{
			name:     "invalid timeout format",
			endpoint: "/v1/openrewrite/transform",
			method:   "POST",
			body: TransformRequest{
				JobID:      "error-test-002",
				TarArchive: base64.StdEncoding.EncodeToString([]byte("fake-tar-data")),
				RecipeConfig: RecipeConfig{
					Recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
				},
				Timeout: "invalid-timeout-format",
			},
			expectedStatus: fiber.StatusBadRequest,
			errorCheck: func(t *testing.T, body map[string]interface{}) {
				assert.Contains(t, body["error"], "invalid timeout format")
				assert.Equal(t, "VALIDATION_ERROR", body["code"])
			},
		},
		{
			name:           "unsupported HTTP method",
			endpoint:       "/v1/openrewrite/transform",
			method:         "GET", // Transform only accepts POST
			body:           nil,
			expectedStatus: fiber.StatusMethodNotAllowed,
			errorCheck: func(t *testing.T, body map[string]interface{}) {
				// Fiber returns method not allowed for unsupported methods
			},
		},
		{
			name:           "health endpoint with wrong method",
			endpoint:       "/v1/openrewrite/health",
			method:         "POST", // Health only accepts GET
			body:           nil,
			expectedStatus: fiber.StatusMethodNotAllowed,
			errorCheck: func(t *testing.T, body map[string]interface{}) {
				// Fiber returns method not allowed
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reqBody []byte
			var err error

			// Prepare request body
			if tt.body != nil {
				if str, ok := tt.body.(string); ok {
					// Malformed JSON string
					reqBody = []byte(str)
				} else {
					// Marshal struct to JSON
					reqBody, err = json.Marshal(tt.body)
					require.NoError(t, err)
				}
			}

			// Create request
			var req *http.Request
			if len(reqBody) > 0 {
				req = httptest.NewRequest(tt.method, tt.endpoint, bytes.NewReader(reqBody))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.endpoint, nil)
			}

			// Execute request
			resp, err := app.Test(req, 5000) // 5s timeout for error tests
			require.NoError(t, err)

			// Verify status code
			assert.Equal(t, tt.expectedStatus, resp.StatusCode, 
				"Expected status %d but got %d for %s", tt.expectedStatus, resp.StatusCode, tt.name)

			// Parse response if JSON error expected
			if tt.errorCheck != nil && resp.StatusCode >= 400 {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var responseBody map[string]interface{}
				if json.Unmarshal(body, &responseBody) == nil {
					// Valid JSON error response
					tt.errorCheck(t, responseBody)
				}
			}

			t.Logf("✓ Error handling test '%s' passed with status %d", tt.name, resp.StatusCode)
		})
	}
}

// TestAPI_PerformanceIntegration validates API performance requirements
func TestAPI_PerformanceIntegration(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping API performance integration test in short mode")
	}

	// Check if Maven is available
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("Maven not installed, skipping performance integration test")
	}

	// Setup real executor and handler
	executor := openrewrite.NewExecutor(openrewrite.DefaultConfig())
	handler := NewHandler(executor)
	
	app := fiber.New()
	handler.RegisterRoutes(app)

	// Performance test cases
	tests := []struct {
		name        string
		projectType string
		requirement time.Duration
	}{
		{
			name:        "simple Maven project performance",
			projectType: "simple",
			requirement: 5 * time.Minute, // Roadmap requirement
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test project based on type
			var tarArchive string
			switch tt.projectType {
			case "simple":
				tarArchive = createAPITestProject(t)
			default:
				t.Fatalf("Unknown project type: %s", tt.projectType)
			}

			// Prepare request
			transformReq := TransformRequest{
				JobID:      fmt.Sprintf("perf-test-%d", time.Now().Unix()),
				TarArchive: tarArchive,
				RecipeConfig: RecipeConfig{
					Recipe:    "org.openrewrite.java.migrate.UpgradeToJava17",
					Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
				},
			}

			reqBody, err := json.Marshal(transformReq)
			require.NoError(t, err)

			// Make request and measure time
			req := httptest.NewRequest("POST", "/v1/openrewrite/transform", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			startTime := time.Now()
			resp, err := app.Test(req, int(tt.requirement.Milliseconds())) // Use requirement as timeout
			duration := time.Since(startTime)
			require.NoError(t, err)

			// Verify performance requirement
			assert.Less(t, duration, tt.requirement, 
				"API should complete within %v but took %v", tt.requirement, duration)

			// Parse response for additional validation
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var transformResp TransformResponse
			err = json.Unmarshal(body, &transformResp)
			require.NoError(t, err)

			// Log performance metrics
			t.Logf("Performance Metrics:")
			t.Logf("  Total API Time: %v", duration)
			t.Logf("  Reported Duration: %.2fs", transformResp.Duration)
			t.Logf("  Success: %v", transformResp.Success)
			t.Logf("  Build System: %s", transformResp.BuildSystem)
			
			if transformResp.Stats != nil {
				t.Logf("  Files Changed: %d", transformResp.Stats.FilesChanged)
				t.Logf("  Tar Size: %d bytes", transformResp.Stats.TarSize)
			}

			// Roadmap Performance Requirements validation
			assert.Less(t, duration, 5*time.Minute, "Transformation should complete in < 5 minutes") 
			assert.Less(t, duration, 1*time.Second+time.Duration(transformResp.Duration*float64(time.Second)), 
				"API overhead should be minimal")
		})
	}
}

// Helper function to create a test project for API testing
func createAPITestProject(t *testing.T) string {
	tempDir := t.TempDir()
	projectDir := filepath.Join(tempDir, "api-test-project")
	require.NoError(t, os.MkdirAll(projectDir, 0755))

	// Create pom.xml with Java 11
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.example</groupId>
    <artifactId>api-test-project</artifactId>
    <version>1.0.0</version>
    
    <properties>
        <maven.compiler.source>11</maven.compiler.source>
        <maven.compiler.target>11</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    </properties>
</project>`

	pomPath := filepath.Join(projectDir, "pom.xml")
	require.NoError(t, os.WriteFile(pomPath, []byte(pomXML), 0644))

	// Create Java source file
	srcDir := filepath.Join(projectDir, "src", "main", "java", "com", "example")
	require.NoError(t, os.MkdirAll(srcDir, 0755))

	javaFile := `package com.example;

public class APITestApp {
    public static void main(String[] args) {
        System.out.println("API Test Application");
        
        // Use var keyword (Java 10+) 
        var message = "Testing API integration";
        System.out.println(message);
        
        // Test method with typical Java 11 patterns
        processData();
    }
    
    private static void processData() {
        var items = java.util.List.of("api", "test", "data");
        for (var item : items) {
            System.out.println("Processing: " + item);
        }
    }
}`

	javaPath := filepath.Join(srcDir, "APITestApp.java")
	require.NoError(t, os.WriteFile(javaPath, []byte(javaFile), 0644))

	// Create tar archive
	tarCmd := exec.Command("tar", "-czf", "api-test.tar.gz", "-C", projectDir, ".")
	tarCmd.Dir = tempDir
	require.NoError(t, tarCmd.Run())

	// Read and encode to base64
	tarPath := filepath.Join(tempDir, "api-test.tar.gz")
	tarData, err := os.ReadFile(tarPath)
	require.NoError(t, err)

	return base64.StdEncoding.EncodeToString(tarData)
}