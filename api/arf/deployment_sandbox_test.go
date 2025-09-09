package arf

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeploymentSandboxManager_CreateSandbox tests sandbox creation functionality
func TestDeploymentSandboxManager_CreateSandbox(t *testing.T) {
	tests := []struct {
		name        string
		config      SandboxConfig
		mockServer  func() *httptest.Server
		expectError bool
		checkResult func(t *testing.T, sandbox *Sandbox, err error)
	}{
		{
			name: "successful sandbox creation with local path",
			config: SandboxConfig{
				Repository: "https://github.com/example/repo",
				Branch:     "main",
				LocalPath:  "/tmp/test-repo",
				Language:   "java",
				BuildTool:  "maven",
				TTL:        30 * time.Minute,
			},
			mockServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/status/arf-benchmark-test":
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"status": "running"}`))
					default:
						if r.Method == "POST" && r.URL.Path == "/apps/arf-benchmark-test/builds" {
							w.WriteHeader(http.StatusAccepted)
							w.Write([]byte(`{"build_id": "test-build-123"}`))
						} else {
							w.WriteHeader(http.StatusNotFound)
						}
					}
				}))
			},
			expectError: false,
			checkResult: func(t *testing.T, sandbox *Sandbox, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, sandbox)
				assert.Equal(t, "java", sandbox.Metadata["language"])
				assert.Equal(t, "maven", sandbox.Metadata["build_tool"])
				assert.Contains(t, sandbox.Metadata["app_name"], "arf-benchmark-")
			},
		},
		{
			name: "sandbox creation without local path (mock mode)",
			config: SandboxConfig{
				Repository: "https://github.com/example/repo",
				Branch:     "main",
				Language:   "go",
				BuildTool:  "go",
				TTL:        15 * time.Minute,
			},
			mockServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
			},
			expectError: false,
			checkResult: func(t *testing.T, sandbox *Sandbox, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, sandbox)
				assert.Equal(t, "true", sandbox.Metadata["mock_deployment"])
				assert.Equal(t, SandboxStatusReady, sandbox.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests that require actual directory setup for now
			if tt.config.LocalPath != "" {
				t.Skip("Skipping test that requires actual directory setup")
			}

			mockServer := tt.mockServer()
			defer mockServer.Close()

			// Create manager with mock server URL
			manager := NewDeploymentSandboxManager(mockServer.URL, nil)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			sandbox, err := manager.CreateSandbox(ctx, tt.config)

			tt.checkResult(t, sandbox, err)
		})
	}
}

// TestDeploymentSandboxManager_DestroySandbox tests sandbox destruction
func TestDeploymentSandboxManager_DestroySandbox(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/apps/arf-benchmark-test123" {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	manager := NewDeploymentSandboxManager(mockServer.URL, nil)

	ctx := context.Background()
	err := manager.DestroySandbox(ctx, "test123")

	assert.NoError(t, err)
}

// TestDeploymentSandboxManager_ListSandboxes tests listing functionality
func TestDeploymentSandboxManager_ListSandboxes(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apps" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{
					"name": "arf-benchmark-abc123",
					"status": "running",
					"created_at": "2023-01-01T12:00:00Z"
				},
				{
					"name": "other-app",
					"status": "running",
					"created_at": "2023-01-01T12:00:00Z"
				}
			]`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	manager := NewDeploymentSandboxManager(mockServer.URL, nil)

	ctx := context.Background()
	sandboxes, err := manager.ListSandboxes(ctx)

	assert.NoError(t, err)
	assert.Len(t, sandboxes, 1) // Only ARF benchmark apps should be returned
	assert.Equal(t, "abc123", sandboxes[0].ID)
	assert.Equal(t, "arf-benchmark-abc123", sandboxes[0].JailName)
	assert.Equal(t, SandboxStatusReady, sandboxes[0].Status)
}

// TestDeploymentSandboxManager_CleanupExpiredSandboxes tests cleanup functionality
func TestDeploymentSandboxManager_CleanupExpiredSandboxes(t *testing.T) {
	deletedApps := make(map[string]bool)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apps" && r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{
					"name": "arf-benchmark-expired1",
					"status": "running",
					"created_at": "2020-01-01T12:00:00Z"
				}
			]`))
		} else if r.Method == "DELETE" && r.URL.Path == "/apps/arf-benchmark-expired1" {
			deletedApps["expired1"] = true
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	manager := NewDeploymentSandboxManager(mockServer.URL, nil)

	ctx := context.Background()
	err := manager.CleanupExpiredSandboxes(ctx)

	assert.NoError(t, err)
	assert.True(t, deletedApps["expired1"], "Expired sandbox should have been deleted")
}

// TestDeploymentSandboxManager_ExecuteCommand tests command execution
func TestDeploymentSandboxManager_ExecuteCommand(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/apps/test123/exec" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("command output"))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	manager := NewDeploymentSandboxManager(mockServer.URL, nil)

	ctx := context.Background()
	output, err := manager.ExecuteCommand(ctx, "arf-sandbox-test123", "echo", "hello")

	assert.NoError(t, err)
	assert.Equal(t, "command output", output)
}

// TestMapAppStatusToSandboxStatus tests status mapping
func TestMapAppStatusToSandboxStatus(t *testing.T) {
	manager := NewDeploymentSandboxManager("http://test", nil)

	tests := []struct {
		appStatus      string
		expectedStatus string
	}{
		{"building", string(SandboxStatusCreating)},
		{"deploying", string(SandboxStatusCreating)},
		{"running", string(SandboxStatusReady)},
		{"healthy", string(SandboxStatusReady)},
		{"stopped", string(SandboxStatusStopped)},
		{"failed", string(SandboxStatusError)},
		{"unknown", string(SandboxStatusCreating)},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%s", tt.appStatus), func(t *testing.T) {
			result := manager.mapAppStatusToSandboxStatus(tt.appStatus)
			assert.Equal(t, tt.expectedStatus, result)
		})
	}
}

// TestNewDeploymentSandboxManager tests manager creation
func TestNewDeploymentSandboxManager(t *testing.T) {
	t.Run("with custom URL", func(t *testing.T) {
		manager := NewDeploymentSandboxManager("https://custom.example.com", nil)
		assert.Equal(t, "https://custom.example.com", manager.controllerURL)
	})

	t.Run("with empty URL uses default", func(t *testing.T) {
		manager := NewDeploymentSandboxManager("", nil)
		assert.Equal(t, "https://api.dev.ployman.app/v1", manager.controllerURL)
	})

	t.Run("with logger", func(t *testing.T) {
		logCalled := false
		logger := func(level, stage, message, details string) {
			logCalled = true
		}

		manager := NewDeploymentSandboxManager("https://test.com", logger)
		assert.NotNil(t, manager.logger)

		// Test logger is set
		if manager.logger != nil {
			manager.logger("DEBUG", "test", "test message", "test details")
			assert.True(t, logCalled)
		}
	})
}

// TestGetAppURL tests URL construction
func TestGetAppURL(t *testing.T) {
	manager := NewDeploymentSandboxManager("http://test", nil)

	tests := []struct {
		name        string
		appName     string
		envDomain   string
		expectedURL string
		mockServer  func() *httptest.Server
	}{
		{
			name:        "default domain",
			appName:     "test-app",
			envDomain:   "",
			expectedURL: "https://test-app.ployd.app",
			mockServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound) // No custom domains
				}))
			},
		},
		{
			name:        "custom domain from env",
			appName:     "test-app",
			envDomain:   "custom.app",
			expectedURL: "https://test-app.custom.app",
			mockServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound) // No custom domains
				}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable if needed
			if tt.envDomain != "" {
				t.Setenv("PLOY_APPS_DOMAIN", tt.envDomain)
			}

			mockServer := tt.mockServer()
			defer mockServer.Close()

			manager.controllerURL = mockServer.URL

			ctx := context.Background()
			url, err := manager.getAppURL(ctx, tt.appName)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedURL, url)
		})
	}
}
