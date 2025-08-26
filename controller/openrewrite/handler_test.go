package openrewrite

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/iw2rmb/ploy/internal/openrewrite"
	"github.com/iw2rmb/ploy/internal/testutils/fixtures"
)

// MockExecutor is a mock implementation of the OpenRewrite executor
type MockExecutor struct {
	mock.Mock
}

func (m *MockExecutor) Execute(ctx context.Context, jobID string, tarData []byte, recipe openrewrite.RecipeConfig) (*openrewrite.TransformResult, error) {
	args := m.Called(ctx, jobID, tarData, recipe)
	result := args.Get(0)
	if result == nil {
		return nil, args.Error(1)
	}
	return result.(*openrewrite.TransformResult), args.Error(1)
}

func (m *MockExecutor) DetectBuildSystem(srcPath string) openrewrite.BuildSystem {
	args := m.Called(srcPath)
	return args.Get(0).(openrewrite.BuildSystem)
}

func (m *MockExecutor) DetectJavaVersion(srcPath string) (openrewrite.JavaVersion, error) {
	args := m.Called(srcPath)
	return args.Get(0).(openrewrite.JavaVersion), args.Error(1)
}

func TestHandler_TransformEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		request        interface{}
		setupMock      func(*MockExecutor)
		expectedStatus int
		validateBody   func(*testing.T, map[string]interface{})
	}{
		{
			name: "successful transformation",
			request: TransformRequest{
				JobID: "test-job-001",
				TarArchive: createTestTarArchive(t),
				RecipeConfig: RecipeConfig{
					Recipe:    "org.openrewrite.java.migrate.UpgradeToJava17",
					Artifacts: "org.openrewrite.recipe:rewrite-migrate-java:3.15.0",
				},
			},
			setupMock: func(m *MockExecutor) {
				m.On("Execute", mock.Anything, "test-job-001", mock.Anything, mock.Anything).
					Return(&openrewrite.TransformResult{
						Success:     true,
						Diff:        []byte("--- a/pom.xml\n+++ b/pom.xml\n@@ -1,1 +1,1 @@\n-<version>11</version>\n+<version>17</version>"),
						BuildSystem: "maven",
						JavaVersion: "17",
						Duration:    30 * time.Second,
					}, nil)
			},
			expectedStatus: fiber.StatusOK,
			validateBody: func(t *testing.T, body map[string]interface{}) {
				assert.True(t, body["success"].(bool))
				assert.Equal(t, "test-job-001", body["job_id"])
				assert.NotEmpty(t, body["diff"])
				assert.Equal(t, "maven", body["build_system"])
				assert.Equal(t, "17", body["java_version"])
				assert.Greater(t, body["duration_seconds"].(float64), 0.0)
			},
		},
		{
			name: "missing job ID",
			request: TransformRequest{
				TarArchive: createTestTarArchive(t),
				RecipeConfig: RecipeConfig{
					Recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
				},
			},
			setupMock:      func(m *MockExecutor) {},
			expectedStatus: fiber.StatusBadRequest,
			validateBody: func(t *testing.T, body map[string]interface{}) {
				assert.Contains(t, body["error"], "job_id")
			},
		},
		{
			name: "missing tar archive",
			request: TransformRequest{
				JobID: "test-job-002",
				RecipeConfig: RecipeConfig{
					Recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
				},
			},
			setupMock:      func(m *MockExecutor) {},
			expectedStatus: fiber.StatusBadRequest,
			validateBody: func(t *testing.T, body map[string]interface{}) {
				assert.Contains(t, body["error"], "tar_archive")
			},
		},
		{
			name: "invalid base64 tar archive",
			request: TransformRequest{
				JobID:      "test-job-003",
				TarArchive: "not-valid-base64!@#$",
				RecipeConfig: RecipeConfig{
					Recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
				},
			},
			setupMock:      func(m *MockExecutor) {},
			expectedStatus: fiber.StatusBadRequest,
			validateBody: func(t *testing.T, body map[string]interface{}) {
				assert.Contains(t, body["error"], "base64")
			},
		},
		{
			name: "executor returns error",
			request: TransformRequest{
				JobID:      "test-job-004",
				TarArchive: createTestTarArchive(t),
				RecipeConfig: RecipeConfig{
					Recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
				},
			},
			setupMock: func(m *MockExecutor) {
				m.On("Execute", mock.Anything, "test-job-004", mock.Anything, mock.Anything).
					Return(&openrewrite.TransformResult{
						Success:     false,
						Error:       "Maven execution failed",
						BuildSystem: "maven",
						Duration:    5 * time.Second,
					}, fmt.Errorf("maven execution failed"))
			},
			expectedStatus: fiber.StatusOK, // Still returns 200 with error in response
			validateBody: func(t *testing.T, body map[string]interface{}) {
				assert.False(t, body["success"].(bool))
				assert.Equal(t, "test-job-004", body["job_id"])
				assert.Contains(t, body["error"], "Maven execution failed")
			},
		},
		{
			name: "transformation with timeout",
			request: TransformRequest{
				JobID:      "test-job-005",
				TarArchive: createTestTarArchive(t),
				RecipeConfig: RecipeConfig{
					Recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
				},
				Timeout: "10s",
			},
			setupMock: func(m *MockExecutor) {
				m.On("Execute", mock.Anything, "test-job-005", mock.Anything, mock.Anything).
					Return(&openrewrite.TransformResult{
						Success:     true,
						Diff:        []byte("diff content"),
						BuildSystem: "gradle",
						Duration:    8 * time.Second,
					}, nil)
			},
			expectedStatus: fiber.StatusOK,
			validateBody: func(t *testing.T, body map[string]interface{}) {
				assert.True(t, body["success"].(bool))
				assert.Equal(t, "gradle", body["build_system"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			app := fiber.New()
			mockExecutor := new(MockExecutor)
			handler := NewHandler(mockExecutor)
			
			handler.RegisterRoutes(app)
			
			if tt.setupMock != nil {
				tt.setupMock(mockExecutor)
			}
			
			// Create request
			reqBody, err := json.Marshal(tt.request)
			require.NoError(t, err)
			
			req := httptest.NewRequest("POST", "/v1/openrewrite/transform", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			
			// Execute
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			
			// Verify status
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			
			// Parse response
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			
			var responseBody map[string]interface{}
			err = json.Unmarshal(body, &responseBody)
			require.NoError(t, err)
			
			// Validate response
			if tt.validateBody != nil {
				tt.validateBody(t, responseBody)
			}
			
			mockExecutor.AssertExpectations(t)
		})
	}
}

func TestHandler_HealthEndpoint(t *testing.T) {
	// Setup
	app := fiber.New()
	mockExecutor := new(MockExecutor)
	handler := NewHandler(mockExecutor)
	
	handler.RegisterRoutes(app)
	
	// Create request
	req := httptest.NewRequest("GET", "/v1/openrewrite/health", nil)
	
	// Execute
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	
	// Verify status
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	
	// Parse response
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	
	var health HealthResponse
	err = json.Unmarshal(body, &health)
	require.NoError(t, err)
	
	// Validate response
	assert.Equal(t, "healthy", health.Status)
	assert.NotEmpty(t, health.Version)
	assert.NotZero(t, health.Timestamp)
}

func TestHandler_ValidateRequest(t *testing.T) {
	handler := &Handler{}

	tests := []struct {
		name     string
		request  TransformRequest
		wantErr  bool
		errorMsg string
	}{
		{
			name: "valid request",
			request: TransformRequest{
				JobID:      "valid-job",
				TarArchive: "dGVzdA==", // base64 "test"
				RecipeConfig: RecipeConfig{
					Recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
				},
			},
			wantErr: false,
		},
		{
			name: "empty job ID",
			request: TransformRequest{
				TarArchive: "dGVzdA==",
				RecipeConfig: RecipeConfig{
					Recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
				},
			},
			wantErr:  true,
			errorMsg: "job_id is required",
		},
		{
			name: "job ID too long",
			request: TransformRequest{
				JobID:      string(make([]byte, 101)), // 101 characters
				TarArchive: "dGVzdA==",
				RecipeConfig: RecipeConfig{
					Recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
				},
			},
			wantErr:  true,
			errorMsg: "job_id must not exceed 100 characters",
		},
		{
			name: "empty tar archive",
			request: TransformRequest{
				JobID: "valid-job",
				RecipeConfig: RecipeConfig{
					Recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
				},
			},
			wantErr:  true,
			errorMsg: "tar_archive is required",
		},
		{
			name: "empty recipe",
			request: TransformRequest{
				JobID:      "valid-job",
				TarArchive: "dGVzdA==",
				RecipeConfig: RecipeConfig{},
			},
			wantErr:  true,
			errorMsg: "recipe is required",
		},
		{
			name: "invalid timeout format",
			request: TransformRequest{
				JobID:      "valid-job",
				TarArchive: "dGVzdA==",
				RecipeConfig: RecipeConfig{
					Recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
				},
				Timeout: "not-a-duration",
			},
			wantErr:  true,
			errorMsg: "invalid timeout format",
		},
		{
			name: "valid timeout",
			request: TransformRequest{
				JobID:      "valid-job",
				TarArchive: "dGVzdA==",
				RecipeConfig: RecipeConfig{
					Recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
				},
				Timeout: "30s",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateRequest(tt.request)
			
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHandler_ParseDiffStats(t *testing.T) {
	handler := &Handler{}

	tests := []struct {
		name     string
		diff     []byte
		expected *TransformStats
	}{
		{
			name: "simple diff",
			diff: []byte(`--- a/pom.xml
+++ b/pom.xml
@@ -1,3 +1,3 @@
 <project>
-  <version>11</version>
+  <version>17</version>
 </project>`),
			expected: &TransformStats{
				FilesChanged: 1,
				LinesAdded:   1,
				LinesRemoved: 1,
			},
		},
		{
			name: "multiple files",
			diff: []byte(`--- a/pom.xml
+++ b/pom.xml
@@ -1,1 +1,1 @@
-<version>11</version>
+<version>17</version>
--- a/src/main/java/App.java
+++ b/src/main/java/App.java
@@ -1,1 +1,2 @@
 public class App {
+  // Updated for Java 17
 }`),
			expected: &TransformStats{
				FilesChanged: 2,
				LinesAdded:   2,
				LinesRemoved: 1,
			},
		},
		{
			name:     "empty diff",
			diff:     []byte{},
			expected: &TransformStats{
				FilesChanged: 0,
				LinesAdded:   0,
				LinesRemoved: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := handler.parseDiffStats(tt.diff)
			assert.Equal(t, tt.expected.FilesChanged, stats.FilesChanged)
			assert.Equal(t, tt.expected.LinesAdded, stats.LinesAdded)
			assert.Equal(t, tt.expected.LinesRemoved, stats.LinesRemoved)
		})
	}
}

// Helper function to create a test tar archive
func createTestTarArchive(t *testing.T) string {
	fixture := &fixtures.ApplicationTar{
		Name:     "test-project",
		Language: "java",
		Files: map[string]string{
			"pom.xml": `<?xml version="1.0"?>
<project>
	<groupId>com.test</groupId>
	<artifactId>test</artifactId>
	<version>1.0.0</version>
	<properties>
		<maven.compiler.source>11</maven.compiler.source>
		<maven.compiler.target>11</maven.compiler.target>
	</properties>
</project>`,
			"src/main/java/App.java": `public class App {
	public static void main(String[] args) {
		System.out.println("Test");
	}
}`,
		},
	}
	
	tarData, err := fixtures.CreateTarballFromFixture(fixture)
	require.NoError(t, err)
	
	return base64.StdEncoding.EncodeToString(tarData)
}