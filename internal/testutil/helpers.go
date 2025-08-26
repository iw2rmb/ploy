package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CreateTempDir creates a temporary directory for testing
func CreateTempDir(t testing.TB) string {
	t.Helper()
	
	tmpDir, err := os.MkdirTemp("", "ploy-test-")
	require.NoError(t, err)
	
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})
	
	return tmpDir
}

// CleanupTempDir removes a temporary directory (deprecated - use CreateTempDir with t.Cleanup)
func CleanupTempDir(t testing.TB, path string) {
	t.Helper()
	err := os.RemoveAll(path)
	if err != nil {
		t.Logf("Failed to cleanup temp dir %s: %v", path, err)
	}
}

// CreateTestFiles creates test files in a directory
func CreateTestFiles(t testing.TB, baseDir string, files map[string]string) {
	t.Helper()
	
	for relPath, content := range files {
		fullPath := filepath.Join(baseDir, relPath)
		
		// Create directory if needed
		dir := filepath.Dir(fullPath)
		if dir != baseDir {
			err := os.MkdirAll(dir, 0755)
			require.NoError(t, err)
		}
		
		// Write file
		err := os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}
}

// CreateTestFilesFromSlice creates test files from a slice of filenames
func CreateTestFilesFromSlice(t testing.TB, baseDir string, filenames []string) {
	t.Helper()
	
	files := make(map[string]string)
	for _, filename := range filenames {
		files[filename] = "# Test file: " + filename
	}
	
	CreateTestFiles(t, baseDir, files)
}

// WriteTestFile writes a single test file
func WriteTestFile(t testing.TB, path, content string) {
	t.Helper()
	
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	require.NoError(t, err)
	
	err = os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
}

// ReadTestFile reads a test file and returns its contents
func ReadTestFile(t testing.TB, path string) string {
	t.Helper()
	
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	
	return string(content)
}

// FileExists checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DirExists checks if a directory exists
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// AssertFileExists asserts that a file exists
func AssertFileExists(t testing.TB, path string) {
	t.Helper()
	assert.True(t, FileExists(path), "File should exist: %s", path)
}

// AssertFileNotExists asserts that a file does not exist
func AssertFileNotExists(t testing.TB, path string) {
	t.Helper()
	assert.False(t, FileExists(path), "File should not exist: %s", path)
}

// AssertDirExists asserts that a directory exists
func AssertDirExists(t testing.TB, path string) {
	t.Helper()
	assert.True(t, DirExists(path), "Directory should exist: %s", path)
}

// AssertFileContains asserts that a file contains the specified content
func AssertFileContains(t testing.TB, path, expectedContent string) {
	t.Helper()
	
	content := ReadTestFile(t, path)
	assert.Contains(t, content, expectedContent, "File should contain expected content")
}

// HTTP Response Helpers

// ReadJSONResponse reads and unmarshals a JSON response
func ReadJSONResponse(t testing.TB, resp *http.Response, target interface{}) {
	t.Helper()
	
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	defer resp.Body.Close()
	
	err = json.Unmarshal(body, target)
	require.NoError(t, err)
}

// ReadStringResponse reads a response body as string
func ReadStringResponse(t testing.TB, resp *http.Response) string {
	t.Helper()
	
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	defer resp.Body.Close()
	
	return string(body)
}

// AssertJSONResponse asserts that a response contains expected JSON
func AssertJSONResponse(t testing.TB, resp *http.Response, expected interface{}) {
	t.Helper()
	
	var actual interface{}
	ReadJSONResponse(t, resp, &actual)
	
	assert.Equal(t, expected, actual)
}

// AssertJSONResponseContains asserts that a JSON response contains specific fields
func AssertJSONResponseContains(t testing.TB, resp *http.Response, expectedFields map[string]interface{}) {
	t.Helper()
	
	var responseMap map[string]interface{}
	ReadJSONResponse(t, resp, &responseMap)
	
	for key, expectedValue := range expectedFields {
		assert.Equal(t, expectedValue, responseMap[key], "Field %s should have expected value", key)
	}
}

// AssertResponseStatus asserts the HTTP response status
func AssertResponseStatus(t testing.TB, resp *http.Response, expectedStatus int) {
	t.Helper()
	assert.Equal(t, expectedStatus, resp.StatusCode, "Response status should match")
}

// AssertResponseHeader asserts a specific response header
func AssertResponseHeader(t testing.TB, resp *http.Response, headerName, expectedValue string) {
	t.Helper()
	assert.Equal(t, expectedValue, resp.Header.Get(headerName), "Header %s should have expected value", headerName)
}

// JSON Helpers

// MarshalJSON marshals an object to JSON bytes
func MarshalJSON(t testing.TB, v interface{}) []byte {
	t.Helper()
	
	data, err := json.Marshal(v)
	require.NoError(t, err)
	
	return data
}

// MarshalJSONString marshals an object to JSON string
func MarshalJSONString(t testing.TB, v interface{}) string {
	t.Helper()
	return string(MarshalJSON(t, v))
}

// UnmarshalJSON unmarshals JSON bytes to an object
func UnmarshalJSON(t testing.TB, data []byte, target interface{}) {
	t.Helper()
	
	err := json.Unmarshal(data, target)
	require.NoError(t, err)
}

// UnmarshalJSONString unmarshals JSON string to an object
func UnmarshalJSONString(t testing.TB, jsonStr string, target interface{}) {
	t.Helper()
	UnmarshalJSON(t, []byte(jsonStr), target)
}

// CreateJSONReader creates an io.Reader from a JSON-serializable object
func CreateJSONReader(t testing.TB, v interface{}) io.Reader {
	t.Helper()
	return bytes.NewReader(MarshalJSON(t, v))
}

// String Helpers

// ContainsAll checks if a string contains all specified substrings
func ContainsAll(s string, substrings ...string) bool {
	for _, substring := range substrings {
		if !strings.Contains(s, substring) {
			return false
		}
	}
	return true
}

// ContainsAny checks if a string contains any of the specified substrings
func ContainsAny(s string, substrings ...string) bool {
	for _, substring := range substrings {
		if strings.Contains(s, substring) {
			return true
		}
	}
	return false
}

// AssertContainsAll asserts that a string contains all specified substrings
func AssertContainsAll(t testing.TB, s string, substrings ...string) {
	t.Helper()
	for _, substring := range substrings {
		assert.Contains(t, s, substring, "String should contain: %s", substring)
	}
}

// AssertContainsAny asserts that a string contains at least one of the specified substrings
func AssertContainsAny(t testing.TB, s string, substrings ...string) {
	t.Helper()
	assert.True(t, ContainsAny(s, substrings...), "String should contain at least one of: %v", substrings)
}

// Map Helpers

// MergeStringMaps merges multiple string maps into one
func MergeStringMaps(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// CopyStringMap creates a copy of a string map
func CopyStringMap(original map[string]string) map[string]string {
	copy := make(map[string]string)
	for k, v := range original {
		copy[k] = v
	}
	return copy
}

// StringMapEquals checks if two string maps are equal
func StringMapEquals(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// AssertStringMapEquals asserts that two string maps are equal
func AssertStringMapEquals(t testing.TB, expected, actual map[string]string) {
	t.Helper()
	assert.Equal(t, expected, actual, "String maps should be equal")
}

// AssertStringMapContains asserts that a map contains specific key-value pairs
func AssertStringMapContains(t testing.TB, actual map[string]string, expected map[string]string) {
	t.Helper()
	for key, expectedValue := range expected {
		actualValue, exists := actual[key]
		assert.True(t, exists, "Map should contain key: %s", key)
		assert.Equal(t, expectedValue, actualValue, "Value for key %s should match", key)
	}
}

// Slice Helpers

// StringSliceContains checks if a string slice contains a value
func StringSliceContains(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

// StringSliceContainsAll checks if a slice contains all specified values
func StringSliceContainsAll(slice []string, values ...string) bool {
	for _, value := range values {
		if !StringSliceContains(slice, value) {
			return false
		}
	}
	return true
}

// AssertSliceContains asserts that a slice contains a specific value
func AssertSliceContains(t testing.TB, slice []string, value string) {
	t.Helper()
	assert.Contains(t, slice, value, "Slice should contain value: %s", value)
}

// AssertSliceContainsAll asserts that a slice contains all specified values
func AssertSliceContainsAll(t testing.TB, slice []string, values ...string) {
	t.Helper()
	for _, value := range values {
		assert.Contains(t, slice, value, "Slice should contain value: %s", value)
	}
}

// Test Execution Helpers

// RequireNoError is a convenience wrapper around require.NoError
func RequireNoError(t testing.TB, err error, msgAndArgs ...interface{}) {
	t.Helper()
	require.NoError(t, err, msgAndArgs...)
}

// RequireError is a convenience wrapper around require.Error
func RequireError(t testing.TB, err error, msgAndArgs ...interface{}) {
	t.Helper()
	require.Error(t, err, msgAndArgs...)
}

// AssertNoError is a convenience wrapper around assert.NoError
func AssertNoError(t testing.TB, err error, msgAndArgs ...interface{}) {
	t.Helper()
	assert.NoError(t, err, msgAndArgs...)
}

// AssertError is a convenience wrapper around assert.Error
func AssertError(t testing.TB, err error, msgAndArgs ...interface{}) {
	t.Helper()
	assert.Error(t, err, msgAndArgs...)
}

// AssertErrorContains asserts that an error contains a specific message
func AssertErrorContains(t testing.TB, err error, expectedMessage string) {
	t.Helper()
	require.Error(t, err)
	assert.Contains(t, err.Error(), expectedMessage, "Error should contain expected message")
}

// Environment Variable Helpers

// WithEnvVar temporarily sets an environment variable for the duration of a test
func WithEnvVar(t testing.TB, key, value string) {
	t.Helper()
	
	oldValue, existed := os.LookupEnv(key)
	
	err := os.Setenv(key, value)
	require.NoError(t, err)
	
	t.Cleanup(func() {
		if existed {
			os.Setenv(key, oldValue)
		} else {
			os.Unsetenv(key)
		}
	})
}

// WithEnvVars temporarily sets multiple environment variables
func WithEnvVars(t testing.TB, envVars map[string]string) {
	t.Helper()
	
	for key, value := range envVars {
		WithEnvVar(t, key, value)
	}
}