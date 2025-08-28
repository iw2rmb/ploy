package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/chttp/internal/config"
	"github.com/iw2rmb/ploy/chttp/internal/executor"
	"github.com/iw2rmb/ploy/chttp/internal/server"
)

// TestBasicCLIExecution tests basic CLI command execution
func TestBasicCLIExecution(t *testing.T) {
	tests := []struct {
		name           string
		request        executor.ExecuteRequest
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "Simple echo command",
			request: executor.ExecuteRequest{
				Command: "echo",
				Args:    []string{"Hello, World!"},
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "List current directory",
			request: executor.ExecuteRequest{
				Command: "ls",
				Args:    []string{"-la"},
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "Disallowed command",
			request: executor.ExecuteRequest{
				Command: "rm",
				Args:    []string{"-rf", "/"},
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
		{
			name: "Command with timeout",
			request: executor.ExecuteRequest{
				Command: "echo",
				Args:    []string{"test"},
				Timeout: "1s",
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test configuration
			cfg := &config.Config{
				Server: config.ServerConfig{
					Host: "localhost",
					Port: 8081,
				},
				Security: config.SecurityConfig{
					APIKey: "test-api-key",
				},
				Commands: config.CommandsConfig{
					Allowed:        []string{"echo", "ls", "cat", "grep"},
					DefaultTimeout: 30 * time.Second,
				},
				Logging: config.LoggingConfig{
					Level:  "info",
					Format: "json",
				},
				Health: config.HealthConfig{
					Enabled:  true,
					Endpoint: "/health",
				},
			}

			// Create CLI executor
			exec := executor.NewCLIExecutor(cfg)

			// Test request validation
			err := exec.ValidateRequest(tt.request)
			if tt.expectSuccess && tt.expectedStatus == 200 {
				require.NoError(t, err, "Request validation should pass")
			} else if !tt.expectSuccess && tt.expectedStatus == 400 {
				require.Error(t, err, "Request validation should fail")
				return // Skip execution test for invalid requests
			}

			// Execute command
			response, err := exec.Execute(nil, tt.request)
			require.NoError(t, err, "Command execution should not return error")
			require.NotNil(t, response, "Response should not be nil")

			// Verify response
			if tt.expectSuccess {
				assert.True(t, response.Success, "Command should succeed")
				assert.Equal(t, 0, response.ExitCode, "Exit code should be 0")
				assert.NotEmpty(t, response.Duration, "Duration should be set")
			} else {
				assert.False(t, response.Success, "Command should fail")
				assert.NotEqual(t, 0, response.ExitCode, "Exit code should not be 0")
			}
		})
	}
}

// TestConfigValidation tests configuration validation
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  config.Config
		wantErr bool
	}{
		{
			name: "Valid configuration",
			config: config.Config{
				Server: config.ServerConfig{
					Host: "0.0.0.0",
					Port: 8080,
				},
				Security: config.SecurityConfig{
					APIKey: "secure-api-key",
				},
				Commands: config.CommandsConfig{
					Allowed: []string{"echo", "ls"},
				},
				Logging: config.LoggingConfig{
					Level:  "info",
					Format: "json",
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid port",
			config: config.Config{
				Server: config.ServerConfig{
					Port: 0,
				},
				Security: config.SecurityConfig{
					APIKey: "test-key",
				},
				Commands: config.CommandsConfig{
					Allowed: []string{"echo"},
				},
			},
			wantErr: true,
		},
		{
			name: "Missing API key",
			config: config.Config{
				Server: config.ServerConfig{
					Port: 8080,
				},
				Security: config.SecurityConfig{
					APIKey: "",
				},
				Commands: config.CommandsConfig{
					Allowed: []string{"echo"},
				},
			},
			wantErr: true,
		},
		{
			name: "No allowed commands",
			config: config.Config{
				Server: config.ServerConfig{
					Port: 8080,
				},
				Security: config.SecurityConfig{
					APIKey: "test-key",
				},
				Commands: config.CommandsConfig{
					Allowed: []string{},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCommandAllowList tests command allow list functionality
func TestCommandAllowList(t *testing.T) {
	cfg := &config.Config{
		Commands: config.CommandsConfig{
			Allowed: []string{"echo", "ls", "cat"},
		},
	}

	tests := []struct {
		command string
		allowed bool
	}{
		{"echo", true},
		{"ls", true},
		{"cat", true},
		{"rm", false},
		{"sudo", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := cfg.IsCommandAllowed(tt.command)
			assert.Equal(t, tt.allowed, result)
		})
	}
}

// mockServer creates a test server for integration testing
func mockServer(t *testing.T) *server.Server {
	// Create temporary config file
	configPath := "/tmp/test-chttp-config.yaml"
	configContent := `
server:
  host: "localhost"
  port: 8082
security:
  api_key: "test-api-key"
commands:
  allowed: ["echo", "ls", "cat", "grep"]
  default_timeout: "30s"
logging:
  level: "error"  # Reduce noise in tests
  format: "json"
health:
  enabled: true
  endpoint: "/health"
`
	require.NoError(t, writeFile(configPath, configContent))

	srv, err := server.NewServer(configPath)
	require.NoError(t, err)

	return srv
}

// writeFile writes content to a file (helper for tests)
func writeFile(path, content string) error {
	return nil // Placeholder - would implement file writing in real tests
}

// Integration test helpers would go here in a real implementation
// For now, focusing on unit tests to validate the core logic