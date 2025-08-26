package utils

import (
	"archive/tar"
	"compress/gzip"
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
)

// Test utility functions
func createTestTempDir(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "utils_test_*")
	require.NoError(t, err)
	return tmpDir
}

func cleanupTestTempDir(t *testing.T, dir string) {
	err := os.RemoveAll(dir)
	require.NoError(t, err)
}

func createTestFile(t *testing.T, path, content string) {
	err := os.MkdirAll(filepath.Dir(path), 0755)
	require.NoError(t, err)
	err = os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
}

// Tests for Getenv function
func TestGetenv(t *testing.T) {
	tests := []struct {
		name     string
		envKey   string
		envValue string
		setEnv   bool
		fallback string
		expected string
	}{
		{
			name:     "environment variable exists",
			envKey:   "TEST_VAR_EXISTS",
			envValue: "test_value",
			setEnv:   true,
			fallback: "default_value",
			expected: "test_value",
		},
		{
			name:     "environment variable missing uses fallback",
			envKey:   "TEST_VAR_MISSING",
			envValue: "",
			setEnv:   false,
			fallback: "default_value",
			expected: "default_value",
		},
		{
			name:     "empty environment variable uses fallback",
			envKey:   "TEST_VAR_EMPTY",
			envValue: "",
			setEnv:   true,
			fallback: "default_value",
			expected: "default_value",
		},
		{
			name:     "environment variable with spaces",
			envKey:   "TEST_VAR_SPACES",
			envValue: "  value with spaces  ",
			setEnv:   true,
			fallback: "default_value",
			expected: "  value with spaces  ",
		},
		{
			name:     "empty fallback value",
			envKey:   "TEST_VAR_NO_FALLBACK",
			envValue: "",
			setEnv:   false,
			fallback: "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up environment variable before test
			originalValue := os.Getenv(tt.envKey)
			os.Unsetenv(tt.envKey)
			defer func() {
				if originalValue != "" {
					os.Setenv(tt.envKey, originalValue)
				} else {
					os.Unsetenv(tt.envKey)
				}
			}()

			// Set environment variable if needed
			if tt.setEnv {
				os.Setenv(tt.envKey, tt.envValue)
			}

			// Execute test
			result := Getenv(tt.envKey, tt.fallback)

			// Verify result
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Tests for ParseIntEnv function
func TestParseIntEnv(t *testing.T) {
	tests := []struct {
		name     string
		envKey   string
		envValue string
		setEnv   bool
		fallback int
		expected int
	}{
		{
			name:     "valid integer environment variable",
			envKey:   "TEST_INT_VALID",
			envValue: "42",
			setEnv:   true,
			fallback: 100,
			expected: 42,
		},
		{
			name:     "negative integer environment variable",
			envKey:   "TEST_INT_NEGATIVE",
			envValue: "-123",
			setEnv:   true,
			fallback: 100,
			expected: -123,
		},
		{
			name:     "zero integer environment variable",
			envKey:   "TEST_INT_ZERO",
			envValue: "0",
			setEnv:   true,
			fallback: 100,
			expected: 0,
		},
		{
			name:     "invalid integer uses fallback",
			envKey:   "TEST_INT_INVALID",
			envValue: "not_a_number",
			setEnv:   true,
			fallback: 100,
			expected: 100,
		},
		{
			name:     "missing environment variable uses fallback",
			envKey:   "TEST_INT_MISSING",
			envValue: "",
			setEnv:   false,
			fallback: 200,
			expected: 200,
		},
		{
			name:     "empty environment variable uses fallback",
			envKey:   "TEST_INT_EMPTY",
			envValue: "",
			setEnv:   true,
			fallback: 300,
			expected: 300,
		},
		{
			name:     "decimal number uses fallback",
			envKey:   "TEST_INT_DECIMAL",
			envValue: "42.5",
			setEnv:   true,
			fallback: 400,
			expected: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up environment variable before test
			originalValue := os.Getenv(tt.envKey)
			os.Unsetenv(tt.envKey)
			defer func() {
				if originalValue != "" {
					os.Setenv(tt.envKey, originalValue)
				} else {
					os.Unsetenv(tt.envKey)
				}
			}()

			// Set environment variable if needed
			if tt.setEnv {
				os.Setenv(tt.envKey, tt.envValue)
			}

			// Execute test
			result := ParseIntEnv(tt.envKey, tt.fallback)

			// Verify result
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Tests for FileExists function
func TestFileExists(t *testing.T) {
	tmpDir := createTestTempDir(t)
	defer cleanupTestTempDir(t, tmpDir)

	tests := []struct {
		name     string
		filePath string
		setup    func() string // Returns actual file path to test
		expected bool
	}{
		{
			name: "existing file",
			setup: func() string {
				filePath := filepath.Join(tmpDir, "existing_file.txt")
				createTestFile(t, filePath, "test content")
				return filePath
			},
			expected: true,
		},
		{
			name: "existing directory",
			setup: func() string {
				dirPath := filepath.Join(tmpDir, "existing_dir")
				err := os.MkdirAll(dirPath, 0755)
				require.NoError(t, err)
				return dirPath
			},
			expected: true,
		},
		{
			name: "non-existing file",
			setup: func() string {
				return filepath.Join(tmpDir, "non_existing_file.txt")
			},
			expected: false,
		},
		{
			name: "empty path",
			setup: func() string {
				return ""
			},
			expected: false,
		},
		{
			name: "path with special characters",
			setup: func() string {
				filePath := filepath.Join(tmpDir, "file with spaces & symbols!.txt")
				createTestFile(t, filePath, "special content")
				return filePath
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setup()
			result := FileExists(filePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Tests for ErrJSON function
func TestErrJSON(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		err          error
		expectedCode int
		expectedBody map[string]interface{}
	}{
		{
			name:         "standard error response",
			statusCode:   400,
			err:          fmt.Errorf("validation failed"),
			expectedCode: 400,
			expectedBody: map[string]interface{}{
				"error": "validation failed",
			},
		},
		{
			name:         "internal server error",
			statusCode:   500,
			err:          fmt.Errorf("database connection failed"),
			expectedCode: 500,
			expectedBody: map[string]interface{}{
				"error": "database connection failed",
			},
		},
		{
			name:         "not found error",
			statusCode:   404,
			err:          fmt.Errorf("resource not found"),
			expectedCode: 404,
			expectedBody: map[string]interface{}{
				"error": "resource not found",
			},
		},
		{
			name:         "error with special characters",
			statusCode:   422,
			err:          fmt.Errorf("invalid JSON: unexpected character '&' at position 10"),
			expectedCode: 422,
			expectedBody: map[string]interface{}{
				"error": "invalid JSON: unexpected character '&' at position 10",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test Fiber app
			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				return ErrJSON(c, tt.statusCode, tt.err)
			})

			// Create test request
			req := httptest.NewRequest("GET", "/test", nil)

			// Execute request
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Verify status code
			assert.Equal(t, tt.expectedCode, resp.StatusCode)

			// Verify response body
			var responseBody map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&responseBody)
			require.NoError(t, err)

			// Check error message
			assert.Equal(t, tt.expectedBody["error"], responseBody["error"])

			// Verify Content-Type header
			assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
		})
	}
}

// Tests for IsHealthy function
func TestIsHealthy(t *testing.T) {
	tests := []struct {
		name           string
		setupServer    func() *httptest.Server
		expectedResult bool
	}{
		{
			name: "healthy service returns 200",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("OK"))
				}))
			},
			expectedResult: true,
		},
		{
			name: "unhealthy service returns 500",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte("Internal Server Error"))
				}))
			},
			expectedResult: false,
		},
		{
			name: "service returns 404",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
					w.Write([]byte("Not Found"))
				}))
			},
			expectedResult: false,
		},
		{
			name: "service with slow response (should timeout)",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(2 * time.Second) // Longer than 1 second timeout
					w.WriteHeader(http.StatusOK)
				}))
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			result := IsHealthy(server.URL)
			assert.Equal(t, tt.expectedResult, result)
		})
	}

	// Test invalid URL
	t.Run("invalid URL", func(t *testing.T) {
		result := IsHealthy("not-a-valid-url")
		assert.False(t, result)
	})

	// Test connection refused
	t.Run("connection refused", func(t *testing.T) {
		result := IsHealthy("http://localhost:99999") // Unlikely to be used
		assert.False(t, result)
	})
}

// Tests for Untar function
func TestUntar(t *testing.T) {
	tmpDir := createTestTempDir(t)
	defer cleanupTestTempDir(t, tmpDir)

	tests := []struct {
		name          string
		setupTarFile  func() (string, error) // Returns tar file path
		expectedFiles []string               // Files that should be extracted
		wantErr       bool
	}{
		{
			name: "extract simple tar file",
			setupTarFile: func() (string, error) {
				return createTestTar(t, tmpDir, map[string]string{
					"file1.txt": "content1",
					"file2.txt": "content2",
				}, false)
			},
			expectedFiles: []string{"file1.txt", "file2.txt"},
			wantErr:       false,
		},
		{
			name: "extract gzipped tar file",
			setupTarFile: func() (string, error) {
				return createTestTar(t, tmpDir, map[string]string{
					"compressed1.txt": "compressed content 1",
					"compressed2.txt": "compressed content 2",
				}, true)
			},
			expectedFiles: []string{"compressed1.txt", "compressed2.txt"},
			wantErr:       false,
		},
		{
			name: "extract tar with directory structure",
			setupTarFile: func() (string, error) {
				return createTestTar(t, tmpDir, map[string]string{
					"dir1/file1.txt":     "dir content 1",
					"dir1/dir2/file2.txt": "nested content",
					"root.txt":           "root content",
				}, false)
			},
			expectedFiles: []string{"dir1/file1.txt", "dir1/dir2/file2.txt", "root.txt"},
			wantErr:       false,
		},
		{
			name: "non-existent tar file",
			setupTarFile: func() (string, error) {
				return filepath.Join(tmpDir, "nonexistent.tar"), nil
			},
			expectedFiles: nil,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup tar file
			tarPath, err := tt.setupTarFile()
			if err != nil {
				t.Fatalf("Failed to setup tar file: %v", err)
			}

			// Create extraction directory
			extractDir := filepath.Join(tmpDir, "extract_"+strings.ReplaceAll(tt.name, " ", "_"))
			err = os.MkdirAll(extractDir, 0755)
			require.NoError(t, err)

			// Execute extraction
			err = Untar(tarPath, extractDir)

			// Verify results
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Check that expected files exist
				for _, expectedFile := range tt.expectedFiles {
					fullPath := filepath.Join(extractDir, expectedFile)
					assert.True(t, FileExists(fullPath), "Expected file should exist: %s", expectedFile)
				}
			}
		})
	}
}

// Helper function to create test tar files
func createTestTar(t *testing.T, baseDir string, files map[string]string, useGzip bool) (string, error) {
	var tarPath string
	if useGzip {
		tarPath = filepath.Join(baseDir, "test.tar.gz")
	} else {
		tarPath = filepath.Join(baseDir, "test.tar")
	}

	tarFile, err := os.Create(tarPath)
	if err != nil {
		return "", err
	}
	defer tarFile.Close()

	var writer io.Writer = tarFile

	if useGzip {
		gzipWriter := gzip.NewWriter(tarFile)
		writer = gzipWriter
		defer gzipWriter.Close()
	}

	tarWriter := tar.NewWriter(writer)
	defer tarWriter.Close()

	for filename, content := range files {
		// Create directory entries if needed
		dir := filepath.Dir(filename)
		if dir != "." {
			header := &tar.Header{
				Name:     dir + "/",
				Mode:     0755,
				Typeflag: tar.TypeDir,
			}
			if err := tarWriter.WriteHeader(header); err != nil {
				return "", err
			}
		}

		// Create file entry
		header := &tar.Header{
			Name: filename,
			Mode: 0644,
			Size: int64(len(content)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return "", err
		}

		if _, err := tarWriter.Write([]byte(content)); err != nil {
			return "", err
		}
	}

	return tarPath, nil
}

// Tests for RunLanePick function
func TestRunLanePick(t *testing.T) {
	// Note: This test requires the lane-pick tool to be available
	// In a real scenario, we might want to mock the exec.Command
	
	tmpDir := createTestTempDir(t)
	defer cleanupTestTempDir(t, tmpDir)

	tests := []struct {
		name        string
		setupPath   func() string
		expectError bool
		skipTest    bool // Skip if lane-pick tool not available
	}{
		{
			name: "valid go project",
			setupPath: func() string {
				projectDir := filepath.Join(tmpDir, "go_project")
				err := os.MkdirAll(projectDir, 0755)
				require.NoError(t, err)
				
				// Create go.mod file
				createTestFile(t, filepath.Join(projectDir, "go.mod"), "module test\n\ngo 1.21")
				createTestFile(t, filepath.Join(projectDir, "main.go"), "package main\n\nfunc main() {}")
				
				return projectDir
			},
			expectError: false,
			skipTest:    true, // Skip unless lane-pick tool is available
		},
		{
			name: "invalid path",
			setupPath: func() string {
				return filepath.Join(tmpDir, "nonexistent")
			},
			expectError: true,
			skipTest:    true, // Skip unless lane-pick tool is available
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipTest {
				// Check if lane-pick tool is available
				if _, err := exec.LookPath("go"); err != nil {
					t.Skip("Skipping test - Go not available")
					return
				}
				
				// Check if tools/lane-pick exists
				if !FileExists("tools/lane-pick") {
					t.Skip("Skipping test - lane-pick tool not available")
					return
				}
			}

			path := tt.setupPath()
			
			result, err := RunLanePick(path)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result.Lane)
				assert.NotEmpty(t, result.Language)
				assert.NotNil(t, result.Reasons)
			}
		})
	}
}

// Benchmark tests
func BenchmarkGetenv(b *testing.B) {
	os.Setenv("BENCHMARK_VAR", "benchmark_value")
	defer os.Unsetenv("BENCHMARK_VAR")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Getenv("BENCHMARK_VAR", "default")
	}
}

func BenchmarkParseIntEnv(b *testing.B) {
	os.Setenv("BENCHMARK_INT", "42")
	defer os.Unsetenv("BENCHMARK_INT")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseIntEnv("BENCHMARK_INT", 100)
	}
}

func BenchmarkFileExists(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "benchmark_*")
	defer os.RemoveAll(tmpDir)
	
	testFile := filepath.Join(tmpDir, "benchmark_file.txt")
	os.WriteFile(testFile, []byte("benchmark content"), 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FileExists(testFile)
	}
}