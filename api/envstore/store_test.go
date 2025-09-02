package envstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helper to create temporary directory
func createTempDir(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "envstore_test_*")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})
	return tmpDir
}

// TestEnvStore_New tests the constructor
func TestEnvStore_New(t *testing.T) {
	tmpDir := createTempDir(t)
	
	store := New(tmpDir)
	
	assert.NotNil(t, store)
	assert.Equal(t, tmpDir, store.basePath)
	
	// Verify directory was created
	_, err := os.Stat(tmpDir)
	assert.NoError(t, err)
}

// TestEnvStore_New_CreatesDirectory tests directory creation
func TestEnvStore_New_CreatesDirectory(t *testing.T) {
	tmpDir := filepath.Join(createTempDir(t), "subdir", "nested")
	
	store := New(tmpDir)
	
	assert.NotNil(t, store)
	
	// Verify nested directory was created
	_, err := os.Stat(tmpDir)
	assert.NoError(t, err)
}

// TestEnvStore_GetAll tests getting all environment variables
func TestEnvStore_GetAll(t *testing.T) {
	tests := []struct {
		name          string
		app           string
		setupData     AppEnvVars
		expectedVars  AppEnvVars
		expectError   bool
	}{
		{
			name: "existing app with variables",
			app:  "test-app",
			setupData: AppEnvVars{
				"NODE_ENV": "production",
				"PORT":     "3000",
				"DEBUG":    "false",
			},
			expectedVars: AppEnvVars{
				"NODE_ENV": "production",
				"PORT":     "3000",
				"DEBUG":    "false",
			},
			expectError: false,
		},
		{
			name:         "nonexistent app returns empty map",
			app:          "nonexistent-app",
			setupData:    nil,
			expectedVars: AppEnvVars{},
			expectError:  false,
		},
		{
			name:         "empty app name",
			app:          "",
			setupData:    nil,
			expectedVars: AppEnvVars{},
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTempDir(t)
			store := New(tmpDir)

			// Setup test data if provided
			if tt.setupData != nil {
				err := store.SetAll(tt.app, tt.setupData)
				require.NoError(t, err)
			}

			// Execute test
			result, err := store.GetAll(tt.app)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedVars, result)
			}
		})
	}
}

// TestEnvStore_Set tests setting individual environment variables
func TestEnvStore_Set(t *testing.T) {
	tests := []struct {
		name        string
		app         string
		key         string
		value       string
		setupData   AppEnvVars
		expectedAll AppEnvVars
	}{
		{
			name:  "set variable in new app",
			app:   "new-app",
			key:   "NODE_ENV",
			value: "development",
			setupData: nil,
			expectedAll: AppEnvVars{
				"NODE_ENV": "development",
			},
		},
		{
			name:  "set variable in existing app",
			app:   "existing-app",
			key:   "DEBUG",
			value: "true",
			setupData: AppEnvVars{
				"NODE_ENV": "production",
				"PORT":     "3000",
			},
			expectedAll: AppEnvVars{
				"NODE_ENV": "production",
				"PORT":     "3000",
				"DEBUG":    "true",
			},
		},
		{
			name:  "overwrite existing variable",
			app:   "overwrite-app",
			key:   "NODE_ENV",
			value: "staging",
			setupData: AppEnvVars{
				"NODE_ENV": "development",
				"PORT":     "3000",
			},
			expectedAll: AppEnvVars{
				"NODE_ENV": "staging",
				"PORT":     "3000",
			},
		},
		{
			name:  "empty key",
			app:   "empty-key-app",
			key:   "",
			value: "some-value",
			setupData: nil,
			expectedAll: AppEnvVars{
				"": "some-value",
			},
		},
		{
			name:  "empty value",
			app:   "empty-value-app",
			key:   "EMPTY_VAR",
			value: "",
			setupData: nil,
			expectedAll: AppEnvVars{
				"EMPTY_VAR": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTempDir(t)
			store := New(tmpDir)

			// Setup initial data if provided
			if tt.setupData != nil {
				err := store.SetAll(tt.app, tt.setupData)
				require.NoError(t, err)
			}

			// Execute set operation
			err := store.Set(tt.app, tt.key, tt.value)
			require.NoError(t, err)

			// Verify the result
			result, err := store.GetAll(tt.app)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedAll, result)
		})
	}
}

// TestEnvStore_SetAll tests setting all environment variables at once
func TestEnvStore_SetAll(t *testing.T) {
	tests := []struct {
		name      string
		app       string
		envVars   AppEnvVars
		setupData AppEnvVars
	}{
		{
			name: "set multiple variables in new app",
			app:  "new-app",
			envVars: AppEnvVars{
				"NODE_ENV": "production",
				"PORT":     "3000",
				"DEBUG":    "false",
			},
			setupData: nil,
		},
		{
			name: "replace all variables in existing app",
			app:  "existing-app",
			envVars: AppEnvVars{
				"NEW_VAR": "new_value",
				"PORT":    "4000",
			},
			setupData: AppEnvVars{
				"OLD_VAR":  "old_value",
				"NODE_ENV": "development",
				"PORT":     "3000",
			},
		},
		{
			name:      "set empty environment variables",
			app:       "empty-vars-app",
			envVars:   AppEnvVars{},
			setupData: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTempDir(t)
			store := New(tmpDir)

			// Setup initial data if provided
			if tt.setupData != nil {
				err := store.SetAll(tt.app, tt.setupData)
				require.NoError(t, err)
			}

			// Execute SetAll operation
			err := store.SetAll(tt.app, tt.envVars)
			require.NoError(t, err)

			// Verify the result
			result, err := store.GetAll(tt.app)
			require.NoError(t, err)
			assert.Equal(t, tt.envVars, result)
		})
	}
}

// TestEnvStore_Get tests getting individual environment variables
func TestEnvStore_Get(t *testing.T) {
	tests := []struct {
		name          string
		app           string
		key           string
		setupData     AppEnvVars
		expectedValue string
		expectedExist bool
		expectError   bool
	}{
		{
			name: "get existing variable",
			app:  "test-app",
			key:  "NODE_ENV",
			setupData: AppEnvVars{
				"NODE_ENV": "production",
				"PORT":     "3000",
			},
			expectedValue: "production",
			expectedExist: true,
			expectError:   false,
		},
		{
			name: "get nonexistent variable from existing app",
			app:  "test-app",
			key:  "MISSING_VAR",
			setupData: AppEnvVars{
				"NODE_ENV": "production",
			},
			expectedValue: "",
			expectedExist: false,
			expectError:   false,
		},
		{
			name:          "get variable from nonexistent app",
			app:           "nonexistent-app",
			key:           "ANY_VAR",
			setupData:     nil,
			expectedValue: "",
			expectedExist: false,
			expectError:   false,
		},
		{
			name: "get empty value variable",
			app:  "empty-value-app",
			key:  "EMPTY_VAR",
			setupData: AppEnvVars{
				"EMPTY_VAR": "",
			},
			expectedValue: "",
			expectedExist: true,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTempDir(t)
			store := New(tmpDir)

			// Setup test data if provided
			if tt.setupData != nil {
				err := store.SetAll(tt.app, tt.setupData)
				require.NoError(t, err)
			}

			// Execute get operation
			value, exists, err := store.Get(tt.app, tt.key)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedValue, value)
				assert.Equal(t, tt.expectedExist, exists)
			}
		})
	}
}

// TestEnvStore_Delete tests deleting environment variables
func TestEnvStore_Delete(t *testing.T) {
	tests := []struct {
		name        string
		app         string
		key         string
		setupData   AppEnvVars
		expectedAll AppEnvVars
	}{
		{
			name: "delete existing variable",
			app:  "test-app",
			key:  "DEBUG",
			setupData: AppEnvVars{
				"NODE_ENV": "production",
				"PORT":     "3000",
				"DEBUG":    "true",
			},
			expectedAll: AppEnvVars{
				"NODE_ENV": "production",
				"PORT":     "3000",
			},
		},
		{
			name: "delete nonexistent variable from existing app",
			app:  "test-app",
			key:  "MISSING_VAR",
			setupData: AppEnvVars{
				"NODE_ENV": "production",
			},
			expectedAll: AppEnvVars{
				"NODE_ENV": "production",
			},
		},
		{
			name:        "delete variable from nonexistent app",
			app:         "nonexistent-app",
			key:         "ANY_VAR",
			setupData:   nil,
			expectedAll: AppEnvVars{},
		},
		{
			name: "delete last variable",
			app:  "last-var-app",
			key:  "ONLY_VAR",
			setupData: AppEnvVars{
				"ONLY_VAR": "only_value",
			},
			expectedAll: AppEnvVars{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTempDir(t)
			store := New(tmpDir)

			// Setup test data if provided
			if tt.setupData != nil {
				err := store.SetAll(tt.app, tt.setupData)
				require.NoError(t, err)
			}

			// Execute delete operation
			err := store.Delete(tt.app, tt.key)
			require.NoError(t, err)

			// Verify the result
			result, err := store.GetAll(tt.app)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedAll, result)
		})
	}
}

// TestEnvStore_ToStringArray tests converting environment variables to string array
func TestEnvStore_ToStringArray(t *testing.T) {
	tests := []struct {
		name      string
		app       string
		setupData AppEnvVars
		expected  []string
	}{
		{
			name: "multiple variables",
			app:  "test-app",
			setupData: AppEnvVars{
				"NODE_ENV": "production",
				"PORT":     "3000",
				"DEBUG":    "false",
			},
			// Note: map iteration order is not guaranteed in Go,
			// so we'll check the length and contents separately
			expected: []string{"NODE_ENV=production", "PORT=3000", "DEBUG=false"},
		},
		{
			name: "single variable",
			app:  "single-var-app",
			setupData: AppEnvVars{
				"SINGLE_VAR": "single_value",
			},
			expected: []string{"SINGLE_VAR=single_value"},
		},
		{
			name:      "empty variables",
			app:       "empty-app",
			setupData: AppEnvVars{},
			expected:  []string{},
		},
		{
			name:      "nonexistent app",
			app:       "nonexistent-app",
			setupData: nil,
			expected:  []string{},
		},
		{
			name: "empty values",
			app:  "empty-values-app",
			setupData: AppEnvVars{
				"EMPTY_VAR": "",
				"NORMAL_VAR": "normal_value",
			},
			expected: []string{"EMPTY_VAR=", "NORMAL_VAR=normal_value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTempDir(t)
			store := New(tmpDir)

			// Setup test data if provided
			if tt.setupData != nil {
				err := store.SetAll(tt.app, tt.setupData)
				require.NoError(t, err)
			}

			// Execute ToStringArray operation
			result, err := store.ToStringArray(tt.app)
			require.NoError(t, err)

			// Verify the result
			assert.Len(t, result, len(tt.expected))
			
			// Convert to map for easier comparison (order doesn't matter)
			resultMap := make(map[string]bool)
			for _, item := range result {
				resultMap[item] = true
			}
			
			expectedMap := make(map[string]bool)
			for _, item := range tt.expected {
				expectedMap[item] = true
			}
			
			assert.Equal(t, expectedMap, resultMap)
		})
	}
}

// TestEnvStore_ConcurrentAccess tests thread safety with concurrent operations
func TestEnvStore_ConcurrentAccess(t *testing.T) {
	tmpDir := createTempDir(t)
	store := New(tmpDir)
	
	const numGoroutines = 10
	const numOperations = 50
	
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numOperations)
	
	// Concurrent writers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()
			
			for j := 0; j < numOperations; j++ {
				appName := fmt.Sprintf("app-%d", routineID)
				key := fmt.Sprintf("VAR_%d", j)
				value := fmt.Sprintf("value_%d_%d", routineID, j)
				
				if err := store.Set(appName, key, value); err != nil {
					errors <- err
					return
				}
			}
		}(i)
	}
	
	// Concurrent readers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()
			
			for j := 0; j < numOperations; j++ {
				appName := fmt.Sprintf("app-%d", routineID)
				
				if _, err := store.GetAll(appName); err != nil {
					errors <- err
					return
				}
			}
		}(i)
	}
	
	// Wait for all goroutines to complete
	wg.Wait()
	close(errors)
	
	// Check for any errors
	for err := range errors {
		assert.NoError(t, err)
	}
	
	// Verify final state - each app should have all its variables
	for i := 0; i < numGoroutines; i++ {
		appName := fmt.Sprintf("app-%d", i)
		vars, err := store.GetAll(appName)
		require.NoError(t, err)
		assert.Len(t, vars, numOperations)
	}
}

// TestEnvStore_FileSystemEdgeCases tests error handling for file system issues
func TestEnvStore_FileSystemEdgeCases(t *testing.T) {
	t.Run("corrupt JSON file", func(t *testing.T) {
		tmpDir := createTempDir(t)
		store := New(tmpDir)
		
		// Write invalid JSON directly to file
		corruptFile := filepath.Join(tmpDir, "corrupt-app.env.json")
		err := os.WriteFile(corruptFile, []byte("invalid json content"), 0644)
		require.NoError(t, err)
		
		// Attempt to read should return error
		_, err = store.GetAll("corrupt-app")
		assert.Error(t, err)
	})
	
	t.Run("read-only directory", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("Skipping read-only test when running as root")
		}
		
		tmpDir := createTempDir(t)
		store := New(tmpDir)
		
		// Make directory read-only
		err := os.Chmod(tmpDir, 0444)
		require.NoError(t, err)
		
		// Restore permissions for cleanup
		t.Cleanup(func() {
			os.Chmod(tmpDir, 0755)
		})
		
		// Attempt to write should return error
		err = store.Set("test-app", "KEY", "value")
		assert.Error(t, err)
	})
}

// TestEnvStore_Performance tests basic performance characteristics
func TestEnvStore_Performance(t *testing.T) {
	tmpDir := createTempDir(t)
	store := New(tmpDir)
	
	// Test with large number of environment variables
	largeEnvVars := make(AppEnvVars)
	for i := 0; i < 1000; i++ {
		largeEnvVars[fmt.Sprintf("VAR_%d", i)] = fmt.Sprintf("value_%d", i)
	}
	
	// Measure SetAll performance
	start := time.Now()
	err := store.SetAll("large-app", largeEnvVars)
	setAllDuration := time.Since(start)
	
	require.NoError(t, err)
	assert.Less(t, setAllDuration, 100*time.Millisecond, 
		"SetAll with 1000 vars should complete within 100ms")
	
	// Measure GetAll performance
	start = time.Now()
	result, err := store.GetAll("large-app")
	getAllDuration := time.Since(start)
	
	require.NoError(t, err)
	assert.Len(t, result, 1000)
	assert.Less(t, getAllDuration, 50*time.Millisecond,
		"GetAll with 1000 vars should complete within 50ms")
}

// BenchmarkEnvStore_Set benchmarks individual set operations
func BenchmarkEnvStore_Set(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "envstore_bench_*")
	defer os.RemoveAll(tmpDir)
	
	store := New(tmpDir)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Set("bench-app", fmt.Sprintf("VAR_%d", i), fmt.Sprintf("value_%d", i))
	}
}

// BenchmarkEnvStore_Get benchmarks individual get operations
func BenchmarkEnvStore_Get(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "envstore_bench_*")
	defer os.RemoveAll(tmpDir)
	
	store := New(tmpDir)
	
	// Setup test data
	testVars := make(AppEnvVars)
	for i := 0; i < 100; i++ {
		testVars[fmt.Sprintf("VAR_%d", i)] = fmt.Sprintf("value_%d", i)
	}
	store.SetAll("bench-app", testVars)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Get("bench-app", fmt.Sprintf("VAR_%d", i%100))
	}
}

// BenchmarkEnvStore_GetAll benchmarks getting all variables
func BenchmarkEnvStore_GetAll(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "envstore_bench_*")
	defer os.RemoveAll(tmpDir)
	
	store := New(tmpDir)
	
	// Setup test data
	testVars := make(AppEnvVars)
	for i := 0; i < 100; i++ {
		testVars[fmt.Sprintf("VAR_%d", i)] = fmt.Sprintf("value_%d", i)
	}
	store.SetAll("bench-app", testVars)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.GetAll("bench-app")
	}
}