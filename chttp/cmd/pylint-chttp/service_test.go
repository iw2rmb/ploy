package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPylintServiceBinaryInfo(t *testing.T) {
	tests := []struct {
		name             string
		expectedService  string
		expectedPort     int
		expectedVersion  string
	}{
		{
			name:            "service info consistency",
			expectedService: "pylint-chttp",
			expectedPort:    8080,
			expectedVersion: "1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceName, port := getPylintServiceInfo()
			
			assert.Equal(t, tt.expectedService, serviceName)
			assert.Equal(t, tt.expectedPort, port)
			assert.Equal(t, tt.expectedVersion, version)
		})
	}
}

func TestPylintExecutableValidation(t *testing.T) {
	tests := []struct {
		name        string
		executable  string
		expectError bool
	}{
		{
			name:        "default pylint executable",
			executable:  pylintExecutable,
			expectError: false, // May or may not be available
		},
		{
			name:        "nonexistent executable",
			executable:  "nonexistent-pylint-tool",
			expectError: true,
		},
		{
			name:        "empty executable",
			executable:  "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't easily mock exec.LookPath in this context
			// So we'll test the behavior indirectly through validatePylintEnvironment
			if tt.executable == pylintExecutable {
				err := validatePylintEnvironment()
				// Error or success depends on system, we just check it doesn't panic
				t.Logf("Pylint validation result: %v", err)
			}
		})
	}
}

func TestPylintConfigTemplate(t *testing.T) {
	config := createPylintConfig()
	
	// Test YAML structure
	lines := strings.Split(config, "\n")
	assert.True(t, len(lines) > 10, "Config should have multiple lines")
	
	// Test required sections
	requiredSections := []string{
		"service:",
		"executable:",
		"security:",
		"input:",
		"output:",
	}
	
	for _, section := range requiredSections {
		assert.Contains(t, config, section, "Config should contain %s section", section)
	}
	
	// Test Pylint-specific values
	pylintSpecific := []string{
		"pylint-chttp",
		"pylint",
		"--output-format=json",
		"--reports=no",
		"pylint_json",
		".py",
		".pyw",
	}
	
	for _, value := range pylintSpecific {
		assert.Contains(t, config, value, "Config should contain Pylint-specific value: %s", value)
	}
	
	// Test security settings
	securitySettings := []string{
		"public_key_path:",
		"run_as_user:",
		"max_memory:",
		"max_cpu:",
	}
	
	for _, setting := range securitySettings {
		assert.Contains(t, config, setting, "Config should contain security setting: %s", setting)
	}
}

func TestTempConfigLifecycle(t *testing.T) {
	// Test config creation
	configPath, err := createTempPylintConfig()
	require.NoError(t, err)
	
	// Verify file exists
	_, err = os.Stat(configPath)
	assert.NoError(t, err, "Config file should exist after creation")
	
	// Verify content
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.NotEmpty(t, content, "Config file should not be empty")
	
	// Verify it contains expected YAML
	configStr := string(content)
	assert.Contains(t, configStr, "service:", "Should be valid YAML with service section")
	
	// Test cleanup
	tempDir := filepath.Dir(configPath)
	err = os.RemoveAll(tempDir)
	assert.NoError(t, err, "Should be able to clean up temp directory")
	
	// Verify cleanup worked
	_, err = os.Stat(configPath)
	assert.True(t, os.IsNotExist(err), "Config file should not exist after cleanup")
}

func TestConfigErrorHandling(t *testing.T) {
	t.Run("temp dir creation failure", func(t *testing.T) {
		// We can't easily simulate temp dir creation failure without
		// changing system permissions, so we'll test other error conditions
		
		// Test with invalid temp directory (this may not fail on all systems)
		originalTempDir := os.Getenv("TMPDIR")
		defer func() {
			if originalTempDir == "" {
				os.Unsetenv("TMPDIR")
			} else {
				os.Setenv("TMPDIR", originalTempDir)
			}
		}()
		
		// Set to a hopefully non-existent or non-writable directory
		os.Setenv("TMPDIR", "/nonexistent/directory")
		
		// This may or may not fail depending on the system's behavior
		configPath, err := createTempPylintConfig()
		if err == nil {
			// If it succeeded despite our attempt, clean up
			os.RemoveAll(filepath.Dir(configPath))
			t.Log("Config creation succeeded despite invalid TMPDIR")
		} else {
			t.Logf("Config creation failed as expected: %v", err)
		}
	})
}

func TestServiceConstants(t *testing.T) {
	// Test that constants are defined correctly
	assert.Equal(t, 8080, defaultPort)
	assert.Equal(t, "pylint-chttp", serviceName)
	assert.Equal(t, "pylint", pylintExecutable)
	assert.Equal(t, "1.0.0", version)
	
	// Test consistency between constants and functions
	name, port := getPylintServiceInfo()
	assert.Equal(t, serviceName, name)
	assert.Equal(t, defaultPort, port)
}

func TestConfigPathHandling(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() (string, error)
		cleanup  func(string) error
		validate func(t *testing.T, path string)
	}{
		{
			name: "standard config creation",
			setup: func() (string, error) {
				return createTempPylintConfig()
			},
			cleanup: func(path string) error {
				return os.RemoveAll(filepath.Dir(path))
			},
			validate: func(t *testing.T, path string) {
				assert.True(t, strings.Contains(path, "pylint-chttp"))
				assert.True(t, strings.HasSuffix(path, ".yaml"))
				
				// Verify file is readable
				content, err := os.ReadFile(path)
				assert.NoError(t, err)
				assert.NotEmpty(t, content)
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := tt.setup()
			require.NoError(t, err)
			
			if tt.cleanup != nil {
				defer func() {
					err := tt.cleanup(path)
					assert.NoError(t, err)
				}()
			}
			
			if tt.validate != nil {
				tt.validate(t, path)
			}
		})
	}
}

func TestServiceConfigurationValidation(t *testing.T) {
	config := createPylintConfig()
	
	// Test that config contains proper defaults
	expectedDefaults := map[string]string{
		"port: 8080":                    "default port",
		"timeout: \"5m\"":               "default timeout", 
		"max_memory: \"512MB\"":         "default memory limit",
		"max_cpu: \"1.0\"":              "default CPU limit",
		"run_as_user: \"pylint\"":       "default user",
		"max_archive_size: \"100MB\"":   "default archive size",
		"auth_method: \"public_key\"":   "default auth method",
	}
	
	for expected, description := range expectedDefaults {
		assert.Contains(t, config, expected, "Config should contain %s: %s", description, expected)
	}
	
	// Test Pylint-specific arguments
	pylintArgs := []string{
		"--output-format=json",
		"--reports=no",
	}
	
	for _, arg := range pylintArgs {
		assert.Contains(t, config, arg, "Config should contain Pylint arg: %s", arg)
	}
}

func BenchmarkServiceConstants(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = getPylintServiceInfo()
	}
}

func BenchmarkConfigValidation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		config := createPylintConfig()
		_ = strings.Contains(config, "pylint-chttp")
	}
}