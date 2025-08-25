package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	
	"github.com/iw2rmb/ploy/internal/storage"
)

// Mock storage client for testing
type MockStorageClient struct {
	mock.Mock
}

func (m *MockStorageClient) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*storage.PutObjectResult, error) {
	args := m.Called(bucket, key, body, contentType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.PutObjectResult), args.Error(1)
}

func (m *MockStorageClient) GetObject(bucket, key string) (io.ReadCloser, error) {
	args := m.Called(bucket, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockStorageClient) UploadArtifactBundle(keyPrefix, artifactPath string) error {
	args := m.Called(keyPrefix, artifactPath)
	return args.Error(0)
}

func (m *MockStorageClient) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*storage.BundleIntegrityResult, error) {
	args := m.Called(keyPrefix, artifactPath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.BundleIntegrityResult), args.Error(1)
}

func (m *MockStorageClient) VerifyUpload(key string) error {
	args := m.Called(key)
	return args.Error(0)
}

func (m *MockStorageClient) ListObjects(bucket, prefix string) ([]storage.ObjectInfo, error) {
	args := m.Called(bucket, prefix)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.ObjectInfo), args.Error(1)
}

func (m *MockStorageClient) GetProviderType() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockStorageClient) GetArtifactsBucket() string {
	args := m.Called()
	return args.String(0)
}

// Note: Using storage package types directly instead of duplicating them here

// Mock environment store for testing
type MockEnvStore struct {
	mock.Mock
}

func (m *MockEnvStore) Get(appName, key string) (string, bool, error) {
	args := m.Called(appName, key)
	return args.String(0), args.Bool(1), args.Error(2)
}

func (m *MockEnvStore) GetAll(appName string) (map[string]string, error) {
	args := m.Called(appName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]string), args.Error(1)
}

func (m *MockEnvStore) Set(appName, key, value string) error {
	args := m.Called(appName, key, value)
	return args.Error(0)
}

func (m *MockEnvStore) SetAll(appName string, envVars map[string]string) error {
	args := m.Called(appName, envVars)
	return args.Error(0)
}

func (m *MockEnvStore) Delete(appName, key string) error {
	args := m.Called(appName, key)
	return args.Error(0)
}

func (m *MockEnvStore) ToStringArray(appName string) ([]string, error) {
	args := m.Called(appName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func TestMapNomadStatusToARF(t *testing.T) {
	tests := []struct {
		name         string
		nomadStatus  string
		expectedARF  string
	}{
		{
			name:        "pending to building",
			nomadStatus: "pending",
			expectedARF: "building",
		},
		{
			name:        "running to deploying",
			nomadStatus: "running",
			expectedARF: "deploying",
		},
		{
			name:        "dead to stopped",
			nomadStatus: "dead",
			expectedARF: "stopped",
		},
		{
			name:        "case insensitive pending",
			nomadStatus: "PENDING",
			expectedARF: "building",
		},
		{
			name:        "case insensitive running",
			nomadStatus: "RUNNING",
			expectedARF: "deploying",
		},
		{
			name:        "case insensitive dead",
			nomadStatus: "DEAD",
			expectedARF: "stopped",
		},
		{
			name:        "unknown status defaults to running",
			nomadStatus: "unknown",
			expectedARF: "running",
		},
		{
			name:        "empty status defaults to running",
			nomadStatus: "",
			expectedARF: "running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapNomadStatusToARF(tt.nomadStatus)
			assert.Equal(t, tt.expectedARF, result)
		})
	}
}

func TestExtractLaneFromJobName(t *testing.T) {
	tests := []struct {
		name        string
		jobName     string
		expectedLane string
	}{
		{
			name:        "valid lane A",
			jobName:     "myapp-lane-a",
			expectedLane: "A",
		},
		{
			name:        "valid lane B",
			jobName:     "myapp-lane-b",
			expectedLane: "B",
		},
		{
			name:        "valid lane C",
			jobName:     "hello-world-lane-c",
			expectedLane: "C",
		},
		{
			name:        "valid lane with complex app name",
			jobName:     "my-complex-app-name-lane-d",
			expectedLane: "D",
		},
		{
			name:        "invalid format - no lane",
			jobName:     "myapp",
			expectedLane: "unknown",
		},
		{
			name:        "invalid format - wrong separator",
			jobName:     "myapp_lane_a",
			expectedLane: "unknown",
		},
		{
			name:        "empty job name",
			jobName:     "",
			expectedLane: "unknown",
		},
		{
			name:        "lane with special characters",
			jobName:     "app-with-dashes-lane-e",
			expectedLane: "E",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractLaneFromJobName(tt.jobName)
			assert.Equal(t, tt.expectedLane, result)
		})
	}
}

func TestDetermineSigningMethod(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "signing-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name           string
		imagePath      string
		dockerImage    string
		env            string
		setupFiles     func() string // Returns actual imagePath to use
		expectedMethod string
	}{
		{
			name:        "keyless OIDC with certificate file",
			env:         "prod",
			setupFiles: func() string {
				imgPath := filepath.Join(tmpDir, "image.bin")
				certPath := imgPath + ".cert"
				
				// Create dummy files
				os.WriteFile(imgPath, []byte("dummy image"), 0644)
				os.WriteFile(certPath, []byte("dummy certificate"), 0644)
				
				return imgPath
			},
			expectedMethod: "keyless-oidc",
		},
		{
			name:        "development signing with development signature",
			env:         "dev",
			setupFiles: func() string {
				imgPath := filepath.Join(tmpDir, "dev-image.bin")
				sigPath := imgPath + ".sig"
				
				// Create dummy files
				os.WriteFile(imgPath, []byte("dummy image"), 0644)
				os.WriteFile(sigPath, []byte("development signature"), 0644)
				
				return imgPath
			},
			expectedMethod: "development",
		},
		{
			name:        "key-based signing with regular signature",
			env:         "prod",
			setupFiles: func() string {
				imgPath := filepath.Join(tmpDir, "key-image.bin")
				sigPath := imgPath + ".sig"
				
				// Create dummy files
				os.WriteFile(imgPath, []byte("dummy image"), 0644)
				os.WriteFile(sigPath, []byte("regular signature content"), 0644)
				
				return imgPath
			},
			expectedMethod: "key-based",
		},
		{
			name:           "docker image production environment",
			dockerImage:    "harbor.local/ploy/myapp:v1.0.0",
			env:            "production",
			expectedMethod: "keyless-oidc",
		},
		{
			name:           "docker image staging environment",
			dockerImage:    "harbor.local/ploy/myapp:v1.0.0",
			env:            "staging",
			expectedMethod: "keyless-oidc",
		},
		{
			name:           "docker image development environment",
			dockerImage:    "harbor.local/ploy/myapp:v1.0.0",
			env:            "dev",
			expectedMethod: "development",
		},
		{
			name:           "no artifacts defaults to development",
			env:            "prod",
			expectedMethod: "development",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var imagePath string
			if tt.setupFiles != nil {
				imagePath = tt.setupFiles()
			} else {
				imagePath = tt.imagePath
			}
			
			result := determineSigningMethod(imagePath, tt.dockerImage, tt.env)
			assert.Equal(t, tt.expectedMethod, result)
		})
	}
}

func TestPerformVulnerabilityScanning(t *testing.T) {
	tests := []struct {
		name        string
		imagePath   string
		dockerImage string
		env         string
		expected    bool
	}{
		{
			name:        "skip scanning in dev environment",
			imagePath:   "/path/to/image.bin",
			env:         "dev",
			expected:    false,
		},
		{
			name:        "skip scanning in development environment",
			imagePath:   "/path/to/image.bin",
			env:         "development",
			expected:    false,
		},
		{
			name:        "skip scanning with empty environment",
			imagePath:   "/path/to/image.bin",
			env:         "",
			expected:    false,
		},
		{
			name:        "attempt scanning in production (will fail without grype)",
			imagePath:   "/path/to/image.bin",
			env:         "production",
			expected:    false, // Will fail because grype is not installed
		},
		{
			name:        "attempt scanning in staging (will fail without grype)",
			dockerImage: "myapp:latest",
			env:         "staging",
			expected:    false, // Will fail because grype is not installed
		},
		{
			name:     "no target artifacts",
			env:      "production",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := performVulnerabilityScanning(tt.imagePath, tt.dockerImage, tt.env)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractSourceRepository(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "repo-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name        string
		setupFiles  func() string // Returns directory path to use
		expectedURL string
	}{
		{
			name: "extract from package.json with object repository",
			setupFiles: func() string {
				dir := filepath.Join(tmpDir, "obj-repo")
				os.MkdirAll(dir, 0755)
				
				packageJSON := map[string]interface{}{
					"name": "test-app",
					"repository": map[string]interface{}{
						"type": "git",
						"url":  "https://github.com/user/repo.git",
					},
				}
				
				data, _ := json.Marshal(packageJSON)
				os.WriteFile(filepath.Join(dir, "package.json"), data, 0644)
				
				return dir
			},
			expectedURL: "https://github.com/user/repo.git",
		},
		{
			name: "extract from package.json with string repository",
			setupFiles: func() string {
				dir := filepath.Join(tmpDir, "str-repo")
				os.MkdirAll(dir, 0755)
				
				packageJSON := map[string]interface{}{
					"name":       "test-app",
					"repository": "https://github.com/user/another-repo.git",
				}
				
				data, _ := json.Marshal(packageJSON)
				os.WriteFile(filepath.Join(dir, "package.json"), data, 0644)
				
				return dir
			},
			expectedURL: "https://github.com/user/another-repo.git",
		},
		{
			name: "no package.json returns empty",
			setupFiles: func() string {
				dir := filepath.Join(tmpDir, "no-package")
				os.MkdirAll(dir, 0755)
				return dir
			},
			expectedURL: "",
		},
		{
			name: "package.json without repository returns empty",
			setupFiles: func() string {
				dir := filepath.Join(tmpDir, "no-repo-field")
				os.MkdirAll(dir, 0755)
				
				packageJSON := map[string]interface{}{
					"name": "test-app",
					"version": "1.0.0",
				}
				
				data, _ := json.Marshal(packageJSON)
				os.WriteFile(filepath.Join(dir, "package.json"), data, 0644)
				
				return dir
			},
			expectedURL: "",
		},
		{
			name: "invalid package.json returns empty",
			setupFiles: func() string {
				dir := filepath.Join(tmpDir, "invalid-json")
				os.MkdirAll(dir, 0755)
				
				os.WriteFile(filepath.Join(dir, "package.json"), []byte("invalid json"), 0644)
				
				return dir
			},
			expectedURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setupFiles()
			result := extractSourceRepository(dir)
			assert.Equal(t, tt.expectedURL, result)
		})
	}
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

func TestLaneResourceAllocation(t *testing.T) {
	tests := []struct {
		name                  string
		lane                  string
		expectedInstanceCount int
		expectedCpuLimit      int
		expectedMemoryLimit   int
		expectedJvmMemory     int
	}{
		{
			name:                  "Lane A - Unikraft",
			lane:                  "A",
			expectedInstanceCount: 3,
			expectedCpuLimit:      200,
			expectedMemoryLimit:   128,
			expectedJvmMemory:     0,
		},
		{
			name:                  "Lane B - Unikraft",
			lane:                  "B",
			expectedInstanceCount: 3,
			expectedCpuLimit:      200,
			expectedMemoryLimit:   128,
			expectedJvmMemory:     0,
		},
		{
			name:                  "Lane C - OSv/JVM",
			lane:                  "C",
			expectedInstanceCount: 2,
			expectedCpuLimit:      1000,
			expectedMemoryLimit:   1024,
			expectedJvmMemory:     768,
		},
		{
			name:                  "Lane D - FreeBSD jail",
			lane:                  "D",
			expectedInstanceCount: 2,
			expectedCpuLimit:      500,
			expectedMemoryLimit:   256,
			expectedJvmMemory:     0,
		},
		{
			name:                  "Lane E - OCI with Kontain",
			lane:                  "E",
			expectedInstanceCount: 2,
			expectedCpuLimit:      600,
			expectedMemoryLimit:   512,
			expectedJvmMemory:     0,
		},
		{
			name:                  "Lane F - Full VM",
			lane:                  "F",
			expectedInstanceCount: 1,
			expectedCpuLimit:      800,
			expectedMemoryLimit:   2048,
			expectedJvmMemory:     0,
		},
		{
			name:                  "Default lane (lowercase)",
			lane:                  "unknown",
			expectedInstanceCount: 2,
			expectedCpuLimit:      500,
			expectedMemoryLimit:   256,
			expectedJvmMemory:     0,
		},
		{
			name:                  "Case insensitive - lowercase c",
			lane:                  "c",
			expectedInstanceCount: 2,
			expectedCpuLimit:      1000,
			expectedMemoryLimit:   1024,
			expectedJvmMemory:     768,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instanceCount := getInstanceCountForLane(tt.lane)
			assert.Equal(t, tt.expectedInstanceCount, instanceCount)
			
			cpuLimit := getCpuLimitForLane(tt.lane)
			assert.Equal(t, tt.expectedCpuLimit, cpuLimit)
			
			memoryLimit := getMemoryLimitForLane(tt.lane)
			assert.Equal(t, tt.expectedMemoryLimit, memoryLimit)
			
			jvmMemory := getJvmMemoryForLane(tt.lane)
			assert.Equal(t, tt.expectedJvmMemory, jvmMemory)
		})
	}
}

func TestListApps(t *testing.T) {
	app := fiber.New()
	app.Get("/apps", ListApps)

	req := httptest.NewRequest("GET", "/apps", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	apps, ok := response["apps"]
	assert.True(t, ok)
	
	// Should return empty array
	appsArray, ok := apps.([]interface{})
	assert.True(t, ok)
	assert.Empty(t, appsArray)
}

// Test helper functions for lane resources consistency
func TestLaneResourceConsistency(t *testing.T) {
	lanes := []string{"A", "B", "C", "D", "E", "F", "G"}
	
	for _, lane := range lanes {
		t.Run(fmt.Sprintf("Lane_%s_consistency", lane), func(t *testing.T) {
			instanceCount := getInstanceCountForLane(lane)
			cpuLimit := getCpuLimitForLane(lane)
			memoryLimit := getMemoryLimitForLane(lane)
			jvmMemory := getJvmMemoryForLane(lane)
			
			// Validate ranges
			assert.True(t, instanceCount >= 1 && instanceCount <= 5, 
				"Instance count should be between 1 and 5, got %d", instanceCount)
			
			assert.True(t, cpuLimit >= 100 && cpuLimit <= 2000, 
				"CPU limit should be between 100 and 2000, got %d", cpuLimit)
			
			assert.True(t, memoryLimit >= 64 && memoryLimit <= 4096, 
				"Memory limit should be between 64 and 4096, got %d", memoryLimit)
			
			assert.True(t, jvmMemory >= 0 && jvmMemory <= memoryLimit, 
				"JVM memory should be between 0 and memory limit (%d), got %d", memoryLimit, jvmMemory)
			
			// Specific validations for JVM lane
			if strings.ToUpper(lane) == "C" {
				assert.Greater(t, jvmMemory, 0, "Lane C should have JVM memory allocation")
				assert.Less(t, jvmMemory, memoryLimit, "JVM memory should be less than total memory limit")
			} else {
				assert.Equal(t, 0, jvmMemory, "Non-JVM lanes should have zero JVM memory")
			}
		})
	}
}

// Benchmarks for resource allocation functions
func BenchmarkGetInstanceCountForLane(b *testing.B) {
	lanes := []string{"A", "B", "C", "D", "E", "F"}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, lane := range lanes {
			getInstanceCountForLane(lane)
		}
	}
}

func BenchmarkGetCpuLimitForLane(b *testing.B) {
	lanes := []string{"A", "B", "C", "D", "E", "F"}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, lane := range lanes {
			getCpuLimitForLane(lane)
		}
	}
}

func BenchmarkGetMemoryLimitForLane(b *testing.B) {
	lanes := []string{"A", "B", "C", "D", "E", "F"}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, lane := range lanes {
			getMemoryLimitForLane(lane)
		}
	}
}

func BenchmarkMapNomadStatusToARF(b *testing.B) {
	statuses := []string{"pending", "running", "dead", "PENDING", "RUNNING", "DEAD", "unknown", ""}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, status := range statuses {
			mapNomadStatusToARF(status)
		}
	}
}

func BenchmarkExtractLaneFromJobName(b *testing.B) {
	jobNames := []string{
		"myapp-lane-a",
		"hello-world-lane-b",
		"complex-app-name-lane-c",
		"invalid-job-name",
		"",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, jobName := range jobNames {
			extractLaneFromJobName(jobName)
		}
	}
}

// Property-based testing
func TestLaneResourceProperties(t *testing.T) {
	t.Run("memory efficiency order", func(t *testing.T) {
		// Unikraft (A,B) should be most memory efficient
		unikraftMemory := getMemoryLimitForLane("A")
		jailMemory := getMemoryLimitForLane("D")
		containerMemory := getMemoryLimitForLane("E")
		jvmMemory := getMemoryLimitForLane("C")
		vmMemory := getMemoryLimitForLane("F")
		
		// Verify expected memory efficiency order
		assert.Less(t, unikraftMemory, jailMemory, "Unikraft should be more memory efficient than jails")
		assert.Less(t, jailMemory, containerMemory, "Jails should be more memory efficient than containers")
		assert.Less(t, containerMemory, jvmMemory, "Containers should be more memory efficient than JVM")
		assert.Less(t, jvmMemory, vmMemory, "JVM should be more memory efficient than full VMs")
	})

	t.Run("CPU efficiency considerations", func(t *testing.T) {
		// Unikraft should need least CPU due to efficiency
		unikraftCPU := getCpuLimitForLane("A")
		jailCPU := getCpuLimitForLane("D")
		containerCPU := getCpuLimitForLane("E")
		jvmCPU := getCpuLimitForLane("C")
		
		// Unikraft should be most CPU efficient
		assert.Less(t, unikraftCPU, jailCPU, "Unikraft should need less CPU than jails")
		assert.Less(t, jailCPU, containerCPU, "Native jails should need less CPU than containers")
		
		// JVM needs most CPU for JIT compilation and GC
		assert.Greater(t, jvmCPU, containerCPU, "JVM should need more CPU than containers")
	})

	t.Run("instance scaling relationship", func(t *testing.T) {
		// More efficient lanes should support more instances
		unikraftInstances := getInstanceCountForLane("A")
		vmInstances := getInstanceCountForLane("F")
		
		assert.Greater(t, unikraftInstances, vmInstances, "More efficient lanes should support more instances")
	})
}

// Edge case testing
func TestLaneResourceEdgeCases(t *testing.T) {
	t.Run("case insensitivity", func(t *testing.T) {
		upperA := getInstanceCountForLane("A")
		lowerA := getInstanceCountForLane("a")
		assert.Equal(t, upperA, lowerA, "Lane resource allocation should be case insensitive")
		
		upperC := getCpuLimitForLane("C")
		lowerC := getCpuLimitForLane("c")
		assert.Equal(t, upperC, lowerC, "Lane resource allocation should be case insensitive")
	})

	t.Run("unknown lanes default consistently", func(t *testing.T) {
		unknownInstances := getInstanceCountForLane("unknown")
		emptyInstances := getInstanceCountForLane("")
		invalidInstances := getInstanceCountForLane("XYZ")
		
		// All unknown lanes should get the same default values
		assert.Equal(t, unknownInstances, emptyInstances, "All unknown lanes should get same defaults")
		assert.Equal(t, emptyInstances, invalidInstances, "All unknown lanes should get same defaults")
	})
}

// Test error scenarios for upload functions
func TestUploadErrorScenarios(t *testing.T) {
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
}

// Test mock implementations
func TestMockStorageClient(t *testing.T) {
	t.Run("mock put object", func(t *testing.T) {
		mockClient := &MockStorageClient{}
		expectedResult := &storage.PutObjectResult{
			ETag:     "test-etag",
			Location: "test-location",
			Size:     100,
		}
		
		mockClient.On("PutObject", "bucket", "key", mock.Anything, "text/plain").Return(expectedResult, nil)
		
		result, err := mockClient.PutObject("bucket", "key", bytes.NewReader([]byte("test")), "text/plain")
		assert.NoError(t, err)
		assert.Equal(t, expectedResult, result)
		
		mockClient.AssertExpectations(t)
	})

	t.Run("mock get artifacts bucket", func(t *testing.T) {
		mockClient := &MockStorageClient{}
		mockClient.On("GetArtifactsBucket").Return("test-artifacts-bucket")
		
		bucket := mockClient.GetArtifactsBucket()
		assert.Equal(t, "test-artifacts-bucket", bucket)
		
		mockClient.AssertExpectations(t)
	})
}

func TestMockEnvStore(t *testing.T) {
	t.Run("mock get all env vars", func(t *testing.T) {
		mockEnvStore := &MockEnvStore{}
		expectedVars := map[string]string{
			"DATABASE_URL": "postgresql://localhost/myapp",
			"API_KEY":      "secret-key",
		}
		
		mockEnvStore.On("GetAll", "myapp").Return(expectedVars, nil)
		
		vars, err := mockEnvStore.GetAll("myapp")
		assert.NoError(t, err)
		assert.Equal(t, expectedVars, vars)
		
		mockEnvStore.AssertExpectations(t)
	})

	t.Run("mock get single env var", func(t *testing.T) {
		mockEnvStore := &MockEnvStore{}
		mockEnvStore.On("Get", "myapp", "DATABASE_URL").Return("postgresql://localhost/myapp", nil)
		
		value, err := mockEnvStore.Get("myapp", "DATABASE_URL")
		assert.NoError(t, err)
		assert.Equal(t, "postgresql://localhost/myapp", value)
		
		mockEnvStore.AssertExpectations(t)
	})
}

// Integration test structure for future VPS testing
func TestBuildHandlerIntegration(t *testing.T) {
	t.Skip("Integration test - requires VPS environment")
	
	// This test would be run on VPS with full infrastructure
	// app := fiber.New()
	// storeClient := storage.NewStorageClient(...)
	// envStore := envstore.NewEnvStore(...)
	// 
	// app.Post("/build/:app", func(c *fiber.Ctx) error {
	//     return TriggerBuild(c, storeClient, envStore)
	// })
	//
	// // Test actual build scenarios with real tar files
	// // Test lane detection integration
	// // Test Nomad deployment integration
	// // Test storage upload integration
}