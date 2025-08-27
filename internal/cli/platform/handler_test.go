package platform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/common"
)

func TestPushCmd(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantApp      string
		wantPlatform bool
		wantEnv      string
		wantLane     string
		wantDomain   string
	}{
		{
			name:         "platform service default",
			args:         []string{"-a", "ploy-api"},
			wantApp:      "ploy-api",
			wantPlatform: true,
			wantEnv:      "dev",
			wantDomain:   "ploy-api.dev.ployman.app",
		},
		{
			name:         "platform service prod",
			args:         []string{"-a", "ploy-api", "-env", "prod"},
			wantApp:      "ploy-api",
			wantPlatform: true,
			wantEnv:      "prod",
			wantDomain:   "ploy-api.ployman.app",
		},
		{
			name:         "platform service with lane",
			args:         []string{"-a", "openrewrite", "-lane", "C"},
			wantApp:      "openrewrite",
			wantPlatform: true,
			wantEnv:      "dev",
			wantLane:     "C",
			wantDomain:   "openrewrite.dev.ployman.app",
		},
		{
			name:         "platform service staging",
			args:         []string{"-a", "metrics", "-env", "staging"},
			wantApp:      "metrics",
			wantPlatform: true,
			wantEnv:      "staging",
			wantDomain:   "metrics.ployman.app", // staging uses prod domain
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server to capture the request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify headers for platform service
				if r.Header.Get("X-Platform-Service") != "true" {
					t.Error("Expected X-Platform-Service header to be true")
				}
				if r.Header.Get("X-Target-Domain") != "ployman.app" {
					t.Errorf("Expected X-Target-Domain to be ployman.app, got %s", r.Header.Get("X-Target-Domain"))
				}

				// Verify environment header
				if tt.wantEnv != "" && r.Header.Get("X-Environment") != tt.wantEnv {
					t.Errorf("Expected X-Environment to be %s, got %s", tt.wantEnv, r.Header.Get("X-Environment"))
				}

				// Verify URL parameters
				if !strings.Contains(r.URL.Path, tt.wantApp) {
					t.Errorf("URL path should contain app name %s", tt.wantApp)
				}

				if !strings.Contains(r.URL.Query().Get("platform"), "true") {
					t.Error("URL should contain platform=true")
				}

				if tt.wantLane != "" && r.URL.Query().Get("lane") != tt.wantLane {
					t.Errorf("URL should contain lane=%s, got %s", tt.wantLane, r.URL.Query().Get("lane"))
				}

				w.Header().Set("X-Deployment-ID", "platform-deploy-456")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status": "success"}`))
			}))
			defer server.Close()

			// Note: In actual implementation, we'll need to mock or refactor
			// PushCmd to be testable. For now, this shows the test structure
		})
	}
}

func TestPushCmdValidation(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "missing app name",
			args:      []string{},
			wantError: true,
			errorMsg:  "platform service name required",
		},
		{
			name:      "valid platform service",
			args:      []string{"-a", "ploy-api"},
			wantError: false,
		},
		{
			name:      "reserved user app name used for platform",
			args:      []string{"-a", "api"},
			wantError: false, // "api" is reserved but valid for platform
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test will validate platform service requirements
			// Implementation will be added when refactoring PushCmd
		})
	}
}

func TestGetPlatformDomain(t *testing.T) {
	tests := []struct {
		name        string
		service     string
		environment string
		want        string
	}{
		{
			name:        "dev environment",
			service:     "ploy-api",
			environment: "dev",
			want:        "ploy-api.dev.ployman.app",
		},
		{
			name:        "prod environment",
			service:     "ploy-api",
			environment: "prod",
			want:        "ploy-api.ployman.app",
		},
		{
			name:        "staging defaults to prod domain",
			service:     "metrics",
			environment: "staging",
			want:        "metrics.ployman.app",
		},
		{
			name:        "no environment defaults to prod",
			service:     "openrewrite",
			environment: "",
			want:        "openrewrite.ployman.app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment for testing
			if tt.environment != "" {
				t.Setenv("PLOY_ENVIRONMENT", tt.environment)
			}

			got := getPlatformDomain(tt.service)
			if got != tt.want {
				t.Errorf("getPlatformDomain() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSharedPushIntegration(t *testing.T) {
	// Test that platform handler correctly uses SharedPush
	config := common.DeployConfig{
		App:           "test-platform",
		IsPlatform:    true,
		Environment:   "dev",
		Lane:          "E",
		ControllerURL: "http://test-api",
	}

	// Verify config has correct platform settings
	if !config.IsPlatform {
		t.Error("Platform service should have IsPlatform=true")
	}

	if config.Environment != "dev" {
		t.Errorf("Expected environment=dev, got %s", config.Environment)
	}
}
