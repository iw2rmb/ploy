package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_WithDefaults(t *testing.T) {
	config, err := Load()
	
	require.NoError(t, err, "Loading default config should not fail")
	assert.Equal(t, "0.0.0.0", config.Server.Host)
	assert.Equal(t, 8082, config.Server.Port)
	assert.Equal(t, 30*time.Second, config.Server.ReadTimeout)
	assert.Equal(t, 30*time.Second, config.Server.WriteTimeout)
}

func TestLoad_WithEnvironmentVariables(t *testing.T) {
	os.Setenv("CLLM_SERVER_HOST", "127.0.0.1")
	os.Setenv("CLLM_SERVER_PORT", "9090")
	defer os.Unsetenv("CLLM_SERVER_HOST")
	defer os.Unsetenv("CLLM_SERVER_PORT")
	
	config, err := Load()
	
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", config.Server.Host)
	assert.Equal(t, 9090, config.Server.Port)
}

func TestLoad_WithConfigFile(t *testing.T) {
	// Create temporary config file
	configContent := `
server:
  host: "192.168.1.1"
  port: 8083
sandbox:
  max_memory: "2GB"
  max_cpu_time: "600s"
`
	tmpfile, err := os.CreateTemp("", "cllm-config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())
	
	_, err = tmpfile.WriteString(configContent)
	require.NoError(t, err)
	tmpfile.Close()
	
	config, err := LoadFromFile(tmpfile.Name())
	
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.1", config.Server.Host)
	assert.Equal(t, 8083, config.Server.Port)
	assert.Equal(t, "2GB", config.Sandbox.MaxMemory)
	assert.Equal(t, "600s", config.Sandbox.MaxCPUTime)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name: "valid config",
			config: &Config{
				Server: ServerConfig{
					Host:         "0.0.0.0",
					Port:         8082,
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
				},
			},
			expectError: false,
		},
		{
			name: "invalid port",
			config: &Config{
				Server: ServerConfig{
					Host: "0.0.0.0",
					Port: 0,
				},
			},
			expectError: true,
		},
		{
			name: "invalid host",
			config: &Config{
				Server: ServerConfig{
					Host: "",
					Port: 8082,
				},
			},
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSandboxConfig_ParseMemoryLimit(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		hasError bool
	}{
		{"1GB", 1024 * 1024 * 1024, false},
		{"512MB", 512 * 1024 * 1024, false},
		{"2048KB", 2048 * 1024, false},
		{"invalid", 0, true},
		{"", 0, false}, // Empty should be valid (no limit)
	}
	
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseMemoryLimit(tt.input)
			
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestLoad_ConfigFileNotFound(t *testing.T) {
	// Should load with defaults when no config file exists
	config, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0", config.Server.Host)
	assert.Equal(t, 8082, config.Server.Port)
}

func TestLoadFromEnvironment_AllVariables(t *testing.T) {
	// Set various environment variables
	env := map[string]string{
		"CLLM_SERVER_READ_TIMEOUT":  "45s",
		"CLLM_SERVER_WRITE_TIMEOUT": "50s",
		"CLLM_SANDBOX_WORK_DIR":     "/custom/work/dir",
		"CLLM_SANDBOX_MAX_MEMORY":   "4GB",
		"CLLM_OLLAMA_URL":           "http://custom-ollama:11434",
		"CLLM_OLLAMA_MODEL":         "custom-model",
	}
	
	// Set environment variables
	for key, value := range env {
		os.Setenv(key, value)
		defer os.Unsetenv(key)
	}
	
	config, err := Load()
	require.NoError(t, err)
	
	assert.Equal(t, 45*time.Second, config.Server.ReadTimeout)
	assert.Equal(t, 50*time.Second, config.Server.WriteTimeout)
	assert.Equal(t, "/custom/work/dir", config.Sandbox.WorkDir)
	assert.Equal(t, "4GB", config.Sandbox.MaxMemory)
	assert.Equal(t, "http://custom-ollama:11434", config.Providers.Ollama.BaseURL)
	assert.Equal(t, "custom-model", config.Providers.Ollama.Model)
}

func TestLoadFromEnvironment_InvalidValues(t *testing.T) {
	testCases := []struct {
		name   string
		envVar string
		value  string
	}{
		{"invalid read timeout", "CLLM_SERVER_READ_TIMEOUT", "invalid-duration"},
		{"invalid write timeout", "CLLM_SERVER_WRITE_TIMEOUT", "not-a-duration"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv(tc.envVar, tc.value)
			defer os.Unsetenv(tc.envVar)
			
			_, err := Load()
			assert.Error(t, err, "Should fail with invalid environment variable")
		})
	}
}

func TestConfig_Validate_SandboxMemoryLimit(t *testing.T) {
	config := &Config{
		Server: ServerConfig{
			Host:         "0.0.0.0",
			Port:         8082,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Sandbox: SandboxConfig{
			MaxMemory: "invalid-memory-format",
		},
	}
	
	err := config.Validate()
	assert.Error(t, err, "Should fail with invalid memory limit")
	assert.Contains(t, err.Error(), "invalid sandbox max memory")
}