package common

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSharedPush(t *testing.T) {
	tests := []struct {
		name       string
		config     DeployConfig
		wantErr    bool
		wantDomain string
	}{
		{
			name: "user app deployment",
			config: DeployConfig{
				App:           "test-app",
				IsPlatform:    false,
				ControllerURL: "http://localhost:8081",
			},
			wantErr:    false,
			wantDomain: "test-app.ployd.app",
		},
		{
			name: "platform service deployment",
			config: DeployConfig{
				App:           "ploy-api",
				IsPlatform:    true,
				ControllerURL: "http://localhost:8081",
			},
			wantErr:    false,
			wantDomain: "ploy-api.ployman.app",
		},
		{
			name: "user app dev environment",
			config: DeployConfig{
				App:           "test-app",
				IsPlatform:    false,
				Environment:   "dev",
				ControllerURL: "http://localhost:8081",
			},
			wantErr:    false,
			wantDomain: "test-app.dev.ployd.app",
		},
		{
			name: "platform service dev environment",
			config: DeployConfig{
				App:           "ploy-api",
				IsPlatform:    true,
				Environment:   "dev",
				ControllerURL: "http://localhost:8081",
			},
			wantErr:    false,
			wantDomain: "ploy-api.dev.ployman.app",
		},
		{
			name: "missing app name",
			config: DeployConfig{
				App:           "",
				ControllerURL: "http://localhost:8081",
			},
			wantErr: true,
		},
		{
			name: "missing controller URL",
			config: DeployConfig{
				App:           "test-app",
				ControllerURL: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify headers
				if tt.config.IsPlatform {
					if r.Header.Get("X-Platform-Service") != "true" {
						t.Errorf("Expected X-Platform-Service header for platform deployment")
					}
					if r.Header.Get("X-Target-Domain") != "ployman.app" {
						t.Errorf("Expected X-Target-Domain to be ployman.app")
					}
				} else {
					if r.Header.Get("X-Target-Domain") != "ployd.app" {
						t.Errorf("Expected X-Target-Domain to be ployd.app")
					}
				}

				// Send success response
				w.Header().Set("X-Deployment-ID", "test-deployment-123")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status": "success"}`))
			}))
			defer server.Close()

			// Update config with test server URL
			if tt.config.ControllerURL != "" {
				tt.config.ControllerURL = server.URL
			}

			result, err := SharedPush(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("SharedPush() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && result != nil {
				if result.URL != "https://"+tt.wantDomain {
					t.Errorf("SharedPush() URL = %v, want %v", result.URL, "https://"+tt.wantDomain)
				}
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  DeployConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: DeployConfig{
				App:           "test-app",
				ControllerURL: "http://localhost:8081",
			},
			wantErr: false,
		},
		{
			name: "missing app name",
			config: DeployConfig{
				App:           "",
				ControllerURL: "http://localhost:8081",
			},
			wantErr: true,
		},
		{
			name: "missing controller URL",
			config: DeployConfig{
				App:           "test-app",
				ControllerURL: "",
			},
			wantErr: true,
		},
		{
			name: "both missing",
			config: DeployConfig{
				App:           "",
				ControllerURL: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateConfig(tt.config); (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildDeployURL(t *testing.T) {
	tests := []struct {
		name   string
		config DeployConfig
		want   string
	}{
		{
			name: "basic URL",
			config: DeployConfig{
				App:           "test-app",
				SHA:           "abc123",
				ControllerURL: "http://localhost:8081",
			},
			want: "http://localhost:8081/apps/test-app/builds?sha=abc123",
		},
		{
			name: "with lane",
			config: DeployConfig{
				App:           "test-app",
				SHA:           "abc123",
				Lane:          "C",
				ControllerURL: "http://localhost:8081",
			},
			want: "http://localhost:8081/apps/test-app/builds?sha=abc123&lane=C",
		},
		{
			name: "with main class",
			config: DeployConfig{
				App:           "test-app",
				SHA:           "abc123",
				MainClass:     "com.example.Main",
				ControllerURL: "http://localhost:8081",
			},
			want: "http://localhost:8081/apps/test-app/builds?sha=abc123&main=com.example.Main",
		},
		{
			name: "platform service",
			config: DeployConfig{
				App:           "ploy-api",
				SHA:           "abc123",
				IsPlatform:    true,
				ControllerURL: "http://localhost:8081",
			},
			want: "http://localhost:8081/apps/ploy-api/builds?sha=abc123&platform=true",
		},
		{
			name: "blue-green deployment",
			config: DeployConfig{
				App:           "test-app",
				SHA:           "abc123",
				BlueGreen:     true,
				ControllerURL: "http://localhost:8081",
			},
			want: "http://localhost:8081/apps/test-app/builds?sha=abc123&blue_green=true",
		},
		{
			name: "with environment",
			config: DeployConfig{
				App:           "test-app",
				SHA:           "abc123",
				Environment:   "staging",
				ControllerURL: "http://localhost:8081",
			},
			want: "http://localhost:8081/apps/test-app/builds?sha=abc123&env=staging",
		},
		{
			name: "all parameters",
			config: DeployConfig{
				App:           "ploy-api",
				SHA:           "abc123",
				Lane:          "E",
				MainClass:     "com.example.Main",
				IsPlatform:    true,
				BlueGreen:     true,
				Environment:   "prod",
				ControllerURL: "http://localhost:8081",
			},
			want: "http://localhost:8081/apps/ploy-api/builds?sha=abc123&main=com.example.Main&lane=E&platform=true&blue_green=true&env=prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildDeployURL(tt.config); got != tt.want {
				t.Errorf("buildDeployURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTargetDomain(t *testing.T) {
	tests := []struct {
		name   string
		config DeployConfig
		want   string
	}{
		{
			name: "user app prod",
			config: DeployConfig{
				IsPlatform:  false,
				Environment: "prod",
			},
			want: "ployd.app",
		},
		{
			name: "user app dev",
			config: DeployConfig{
				IsPlatform:  false,
				Environment: "dev",
			},
			want: "dev.ployd.app",
		},
		{
			name: "platform service prod",
			config: DeployConfig{
				IsPlatform:  true,
				Environment: "prod",
			},
			want: "ployman.app",
		},
		{
			name: "platform service dev",
			config: DeployConfig{
				IsPlatform:  true,
				Environment: "dev",
			},
			want: "dev.ployman.app",
		},
		{
			name: "user app no env (defaults to prod)",
			config: DeployConfig{
				IsPlatform:  false,
				Environment: "",
			},
			want: "ployd.app",
		},
		{
			name: "platform service no env (defaults to prod)",
			config: DeployConfig{
				IsPlatform:  true,
				Environment: "",
			},
			want: "ployman.app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getTargetDomain(tt.config); got != tt.want {
				t.Errorf("getTargetDomain() = %v, want %v", got, tt.want)
			}
		})
	}
}
