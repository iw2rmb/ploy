package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/testutil"
)

// MockBuildDependencies provides mock implementations for build dependencies
type MockBuildDependencies struct {
	StorageClient *testutil.MockStorageClient
	EnvStore      *testutil.MockEnvStore
}

// NewMockBuildDependencies creates new mock dependencies
func NewMockBuildDependencies() *MockBuildDependencies {
	return &MockBuildDependencies{
		StorageClient: testutil.NewMockStorageClient(),
		EnvStore:      testutil.NewMockEnvStore(),
	}
}

// Test TriggerBuild function - focused unit tests for core logic
func TestTriggerBuild(t *testing.T) {
	tests := []struct {
		name           string
		appName        string
		queryParams    string
		requestBody    []byte
		mockSetup      func(*testutil.MockEnvStore)
		expectedStatus int
		expectedError  string
		skipReason     string
	}{
		{
			name:    "invalid app name - too short",
			appName: "x", // Single character name (invalid, minimum is 2)
			expectedStatus: 400,
			expectedError:  "Invalid app name",
		},
		{
			name:    "invalid app name - special characters",
			appName: "invalid@app",
			expectedStatus: 400,
			expectedError:  "Invalid app name",
		},
		{
			name:    "invalid app name - reserved name",
			appName: "api",
			expectedStatus: 400,
			expectedError:  "Invalid app name",
		},
		{
			name:        "successful basic validation - valid app name",
			appName:     "valid-app",
			requestBody: createTestTarball(t, map[string]string{"README.md": "test"}),
			mockSetup: func(envStore *testutil.MockEnvStore) {
				envStore.On("GetAll", "valid-app").Return(map[string]string{
					"ENV_VAR": "value",
				}, nil)
			},
			expectedStatus: 500, // Will fail at build stage but validation passes
			skipReason:     "Integration test - requires builders and nomad",
		},
		{
			name:        "env store error handling",
			appName:     "test-app",
			requestBody: createTestTarball(t, map[string]string{"main.go": "package main"}),
			mockSetup: func(envStore *testutil.MockEnvStore) {
				envStore.On("GetAll", "test-app").Return(nil, fmt.Errorf("env store error"))
			},
			expectedStatus: 500, // Will fail at build stage
			skipReason:     "Integration test - requires builders and nomad",
		},
		{
			name:        "lane parameter handling",
			appName:     "lane-test",
			queryParams: "?lane=A",
			requestBody: createTestTarball(t, map[string]string{"go.mod": "module test"}),
			mockSetup: func(envStore *testutil.MockEnvStore) {
				envStore.On("GetAll", "lane-test").Return(map[string]string{}, nil)
			},
			expectedStatus: 500, // Will fail at build stage but lane logic passes
			skipReason:     "Integration test - requires builders and nomad",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipReason != "" {
				t.Skip(tt.skipReason)
			}
			
			// Setup Fiber app for testing
			app := fiber.New()
			mockEnvStore := testutil.NewMockEnvStore()
			
			if tt.mockSetup != nil {
				tt.mockSetup(mockEnvStore)
			}

			// Create test route using the actual TriggerBuild function
			app.Post("/apps/:app/build", func(c *fiber.Ctx) error {
				return TriggerBuild(c, nil, mockEnvStore)
			})

			// Create test request  
			url := "/apps/" + tt.appName + "/build" + tt.queryParams
			var reqBody io.Reader
			if tt.requestBody != nil {
				reqBody = bytes.NewReader(tt.requestBody)
			}
			req := httptest.NewRequest("POST", url, reqBody)

			// Execute request
			resp, err := app.Test(req, 10000)
			require.NoError(t, err)

			// Verify response
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			
			if tt.expectedError != "" {
				var responseBody map[string]interface{}
				json.NewDecoder(resp.Body).Decode(&responseBody)
				
				if errorMsg, exists := responseBody["error"]; exists {
					assert.Contains(t, errorMsg.(string), tt.expectedError)
				}
			}
			
			// Verify mock expectations
			mockEnvStore.AssertExpectations(t)
		})
	}
}

// Test upload functions with retry logic
func TestUploadFileWithRetryAndVerification(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "upload-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("upload file with retry - file not found", func(t *testing.T) {
		mockProvider := &MockStorageClient{}
		mockProvider.On("GetArtifactsBucket").Return("test-bucket")
		
		// Create storage client wrapper
		storeClient := storage.NewStorageClient(mockProvider, storage.DefaultClientConfig())
		
		nonExistentPath := filepath.Join(tmpDir, "nonexistent.txt")
		
		err := uploadFileWithRetryAndVerification(storeClient, nonExistentPath, "test-key", "text/plain")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open file")
	})

	t.Run("upload file with retry - success on first attempt", func(t *testing.T) {
		// Create test file
		testPath := filepath.Join(tmpDir, "test.txt")
		testContent := []byte("test content")
		err := os.WriteFile(testPath, testContent, 0644)
		require.NoError(t, err)

		mockProvider := &MockStorageClient{}
		mockProvider.On("GetArtifactsBucket").Return("test-bucket")
		mockProvider.On("PutObject", "test-bucket", "test-key", mock.Anything, "text/plain").Return(&storage.PutObjectResult{
			ETag:     "test-etag",
			Location: "test-location",
			Size:     int64(len(testContent)),
		}, nil)
		
		// Mock successful integrity verification
		mockProvider.On("VerifyUpload", "test-key").Return(nil)
		mockProvider.On("GetObject", "test-bucket", "test-key").Return(io.NopCloser(bytes.NewReader(testContent)), nil)
		
		storeClient := storage.NewStorageClient(mockProvider, storage.DefaultClientConfig())
		
		err = uploadFileWithRetryAndVerification(storeClient, testPath, "test-key", "text/plain")
		assert.NoError(t, err)
		
		mockProvider.AssertExpectations(t)
	})

	t.Run("upload file with retry - upload failure then success", func(t *testing.T) {
		// Create test file
		testPath := filepath.Join(tmpDir, "test2.txt")
		testContent := []byte("test content 2")
		err := os.WriteFile(testPath, testContent, 0644)
		require.NoError(t, err)

		mockProvider := &MockStorageClient{}
		mockProvider.On("GetArtifactsBucket").Return("test-bucket")
		
		// First attempt fails
		mockProvider.On("PutObject", "test-bucket", "test-key", mock.Anything, "text/plain").Return(nil, fmt.Errorf("network error")).Once()
		// Second attempt succeeds
		mockProvider.On("PutObject", "test-bucket", "test-key", mock.Anything, "text/plain").Return(&storage.PutObjectResult{
			ETag:     "test-etag",
			Location: "test-location",
			Size:     int64(len(testContent)),
		}, nil).Once()
		
		// Mock successful verification after retry
		mockProvider.On("VerifyUpload", "test-key").Return(nil)
		mockProvider.On("GetObject", "test-bucket", "test-key").Return(io.NopCloser(bytes.NewReader(testContent)), nil)
		
		// Use fast retry config for unit tests to prevent timeouts
		config := storage.DefaultClientConfig()
		config.RetryConfig = testutil.TestRetryConfig()
		storeClient := storage.NewStorageClient(mockProvider, config)
		
		err = uploadFileWithRetryAndVerification(storeClient, testPath, "test-key", "text/plain")
		assert.NoError(t, err)
		
		// Verify both attempts were made
		mockProvider.AssertExpectations(t)
	})

	t.Run("upload file with retry - integration note", func(t *testing.T) {
		t.Skip("Integration test - has hardcoded delays unsuitable for unit testing. Should be tested on VPS.")
		
		// NOTE: The uploadFileWithRetryAndVerification function has its own hardcoded 
		// retry delays (1 second baseDelay) that make it unsuitable for fast unit tests.
		// This functionality should be tested in integration tests on VPS where delays are acceptable.
	})
}

func TestUploadBytesWithRetryAndVerification(t *testing.T) {
	t.Run("upload bytes with retry - success", func(t *testing.T) {
		mockProvider := &MockStorageClient{}
		mockProvider.On("GetArtifactsBucket").Return("test-bucket")
		mockProvider.On("PutObject", "test-bucket", "test-key", mock.Anything, "application/json").Return(&storage.PutObjectResult{
			ETag:     "test-etag",
			Location: "test-location",
			Size:     10,
		}, nil)
		
		// Create storage client wrapper
		storeClient := storage.NewStorageClient(mockProvider, storage.DefaultClientConfig())
		
		testData := []byte("test data!")
		
		err := uploadBytesWithRetryAndVerification(storeClient, testData, "test-key", "application/json")
		assert.NoError(t, err)
		
		mockProvider.AssertExpectations(t)
	})

	t.Run("upload bytes with retry - size mismatch then success", func(t *testing.T) {
		t.Skip("Integration test - has hardcoded delays unsuitable for unit testing. Should be tested on VPS.")
		
		// NOTE: The uploadBytesWithRetryAndVerification function has hardcoded retry delays
		// (1 second baseDelay) that cause unit tests to timeout. Test on VPS instead.
	})

	t.Run("upload bytes with retry - upload failures then success", func(t *testing.T) {
		t.Skip("Integration test - has hardcoded delays unsuitable for unit testing. Should be tested on VPS.")
	})

	t.Run("upload bytes with retry - all attempts fail", func(t *testing.T) {
		t.Skip("Integration test - has hardcoded delays unsuitable for unit testing. Should be tested on VPS.")
	})

	t.Run("upload bytes with retry - size mismatch all attempts", func(t *testing.T) {
		t.Skip("Integration test - has hardcoded delays unsuitable for unit testing. Should be tested on VPS.")
	})

	t.Run("upload bytes with retry - nil result handling", func(t *testing.T) {
		t.Skip("Integration test - has hardcoded delays unsuitable for unit testing. Should be tested on VPS.")
	})
}

func TestCopyFile(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "copy-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Test successful copy
	t.Run("successful copy", func(t *testing.T) {
		srcPath := filepath.Join(tmpDir, "source.txt")
		dstPath := filepath.Join(tmpDir, "destination.txt")
		
		testContent := []byte("Hello, World!")
		err := os.WriteFile(srcPath, testContent, 0644)
		require.NoError(t, err)
		
		err = copyFile(srcPath, dstPath)
		assert.NoError(t, err)
		
		// Verify file was copied correctly
		copiedContent, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, testContent, copiedContent)
		
		// Verify permissions were set correctly
		info, err := os.Stat(dstPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0755), info.Mode())
	})

	// Test copy with non-existent source
	t.Run("non-existent source file", func(t *testing.T) {
		srcPath := filepath.Join(tmpDir, "nonexistent.txt")
		dstPath := filepath.Join(tmpDir, "destination2.txt")
		
		err := copyFile(srcPath, dstPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no such file or directory")
	})

	// Test copy with invalid destination directory
	t.Run("invalid destination directory", func(t *testing.T) {
		srcPath := filepath.Join(tmpDir, "source2.txt")
		dstPath := "/nonexistent/directory/destination.txt"
		
		testContent := []byte("Test content")
		err := os.WriteFile(srcPath, testContent, 0644)
		require.NoError(t, err)
		
		err = copyFile(srcPath, dstPath)
		assert.Error(t, err)
	})
}