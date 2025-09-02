package config

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RegistryConfig holds Docker Registry configuration
type RegistryConfig struct {
	Endpoint string // Registry endpoint (e.g., registry.dev.ployman.app)
	Username string // Registry username (optional)
	Password string // Registry password (optional)
	Insecure bool   // Allow insecure registry connections
}

// AppType represents the type of application for namespace routing
type AppType string

const (
	PlatformApp AppType = "platform" // Platform infrastructure services
	UserApp     AppType = "user"     // User applications
)

// GetRegistryConfig returns Docker Registry configuration
func GetRegistryConfig() *RegistryConfig {
	return &RegistryConfig{
		Endpoint: getEnvOrDefault("REGISTRY_ENDPOINT", "registry.dev.ployman.app"),
		Username: getEnvOrDefault("REGISTRY_USERNAME", ""),
		Password: getEnvOrDefault("REGISTRY_PASSWORD", ""),
		Insecure: getEnvOrDefault("REGISTRY_INSECURE", "false") == "true",
	}
}

// GetRegistryConfigForAppType returns Docker Registry config (same for all app types)
func GetRegistryConfigForAppType(appType AppType) *RegistryConfig {
	return GetRegistryConfig()
}

// DetermineAppType determines if an app is platform or user based on API context
// This is deterministic based on which API endpoint was called
func DetermineAppType(context string) AppType {
	switch strings.ToLower(context) {
	case "platform":
		// Called from /v1/platform/:service/* endpoints
		return PlatformApp
	case "apps", "user":
		// Called from /v1/apps/:app/* endpoints
		return UserApp
	default:
		// Default to user app for safety (less restrictive)
		return UserApp
	}
}

// GetImageTag returns Docker Registry formatted image tag
func (r *RegistryConfig) GetImageTag(app, sha string, appType AppType) string {
	return fmt.Sprintf("%s/%s:%s", r.Endpoint, app, sha)
}

// GetDockerImageTag returns Docker Registry image tag for OCI builds (Lane E)
func (r *RegistryConfig) GetDockerImageTag(app, sha string, appType AppType) string {
	return fmt.Sprintf("%s/%s:%s", r.Endpoint, app, sha)
}

// GetProject returns empty string (Docker Registry doesn't use projects)
func (r *RegistryConfig) GetProject(appType AppType) string {
	return ""
}

// MustAuthenticate ensures Docker Registry authentication or returns error
func (r *RegistryConfig) MustAuthenticate() error {
	// Skip authentication if no credentials provided (registry allows anonymous access)
	if r.Username == "" && r.Password == "" {
		return nil
	}
	
	cmd := exec.Command("docker", "login", 
		r.Endpoint,
		"-u", r.Username,
		"-p", r.Password)
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Docker Registry authentication failed: %w", err)
	}
	
	return nil
}

// Authenticate attempts Docker Registry authentication and returns success status
func (r *RegistryConfig) Authenticate() error {
	return r.MustAuthenticate()
}

// Validate checks if the registry configuration is valid
func (r *RegistryConfig) Validate() error {
	if r.Endpoint == "" {
		return fmt.Errorf("Registry endpoint is required")
	}
	
	return nil
}

// IsLegacyRegistry returns false (we're now using Docker Registry v2 as the standard)
func (r *RegistryConfig) IsLegacyRegistry() bool {
	return false
}

// GetFullEndpoint returns the full Docker Registry endpoint with protocol
func (r *RegistryConfig) GetFullEndpoint() string {
	if strings.HasPrefix(r.Endpoint, "http://") || strings.HasPrefix(r.Endpoint, "https://") {
		return r.Endpoint
	}
	
	// Default to HTTPS unless explicitly insecure
	if r.Insecure {
		return fmt.Sprintf("http://%s", r.Endpoint)
	}
	return fmt.Sprintf("https://%s", r.Endpoint)
}

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}