package testutils

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/iw2rmb/ploy/internal/cli/transflow"
)

// IntegrationConfig holds configuration for integration tests
type IntegrationConfig struct {
	ConsulAddr      string
	NomadAddr       string
	SeaweedFSMaster string
	SeaweedFSFiler  string
	GitLabURL       string
	GitLabToken     string
}

// SkipIfNoServices skips the test if required services are not available
func SkipIfNoServices(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check required services
	services := []struct {
		name string
		url  string
	}{
		{"Consul", "http://localhost:8500/v1/status/leader"},
		{"Nomad", "http://localhost:4646/v1/status/leader"},
		{"SeaweedFS", "http://localhost:9333/cluster/status"},
	}

	for _, service := range services {
		if !isServiceHealthy(ctx, service.url) {
			t.Skipf("%s service not available - run: docker-compose -f docker-compose.integration.yml up -d", service.name)
		}
	}
}

// RequireServices enforces that real services are running - no fallback to mocks
// This function fails the test if services are not available, unlike SkipIfNoServices
func RequireServices(t *testing.T, services ...string) *IntegrationConfig {
	t.Helper()

	config := &IntegrationConfig{}
	var failures []string

	for _, service := range services {
		switch service {
		case "consul":
			if !isConsulHealthy() {
				failures = append(failures, "Consul not available at localhost:8500")
			} else {
				config.ConsulAddr = "localhost:8500"
			}
		case "nomad":
			if !isNomadHealthy() {
				failures = append(failures, "Nomad not available at http://localhost:4646")
			} else {
				config.NomadAddr = "http://localhost:4646"
			}
		case "seaweedfs":
			if !isSeaweedFSHealthy() {
				failures = append(failures, "SeaweedFS not available at http://localhost:8888")
			} else {
				config.SeaweedFSFiler = "http://localhost:8888"
				config.SeaweedFSMaster = "http://localhost:9333"
			}
		case "gitlab":
			token := os.Getenv("GITLAB_TOKEN")
			if token == "" {
				failures = append(failures, "GITLAB_TOKEN environment variable required for real GitLab testing")
			} else {
				config.GitLabURL = "https://gitlab.com"
				config.GitLabToken = token
			}
		default:
			failures = append(failures, fmt.Sprintf("Unknown service: %s", service))
		}
	}

	if len(failures) > 0 {
		t.Fatalf("Required services not available:\n%s\n\nSetup:\n1. Run: docker-compose -f docker-compose.integration.yml up -d\n2. Set GITLAB_TOKEN environment variable for GitLab tests\n3. Wait for services to be healthy", strings.Join(failures, "\n"))
	}

	return config
}

// Service health check functions
func isConsulHealthy() bool {
	client, err := consulapi.NewClient(consulapi.DefaultConfig())
	if err != nil {
		return false
	}
	_, err = client.Status().Leader()
	return err == nil
}

func isNomadHealthy() bool {
	client, err := nomadapi.NewClient(nomadapi.DefaultConfig())
	if err != nil {
		return false
	}
	_, err = client.Status().Leader()
	return err == nil
}

func isSeaweedFSHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return isServiceHealthy(ctx, "http://localhost:8888/") && isServiceHealthy(ctx, "http://localhost:9333/cluster/status")
}

// SetupIntegrationEnvironment sets up the integration test environment
func SetupIntegrationEnvironment(t *testing.T) *IntegrationConfig {
	t.Helper()

	SkipIfNoServices(t)

	return &IntegrationConfig{
		ConsulAddr:      getEnvOrDefault("CONSUL_HTTP_ADDR", "localhost:8500"),
		NomadAddr:       getEnvOrDefault("NOMAD_ADDR", "http://localhost:4646"),
		SeaweedFSMaster: getEnvOrDefault("SEAWEEDFS_MASTER", "http://localhost:9333"),
		SeaweedFSFiler:  getEnvOrDefault("SEAWEEDFS_FILER", "http://localhost:8888"),
		GitLabURL:       getEnvOrDefault("GITLAB_URL", "https://gitlab.com"),
		GitLabToken:     os.Getenv("GITLAB_TOKEN"), // Optional for some tests
	}
}

// NewTransflowConfig creates a basic transflow config for integration tests
func NewTransflowConfig(testID string) *transflow.TransflowConfig {
	return &transflow.TransflowConfig{
		ID:           fmt.Sprintf("integration-test-%s", testID),
		TargetRepo:   "https://github.com/example/test-repo.git",
		BaseRef:      "refs/heads/main",
		BuildTimeout: "10m",
		Steps: []transflow.TransflowStep{
			{
				Type:    "recipe",
				ID:      "test-recipe",
				Engine:  "openrewrite",
				Recipes: []string{"org.openrewrite.java.migrate.Java11toJava17"},
			},
		},
		SelfHeal: &transflow.SelfHealConfig{
			Enabled:    true,
			MaxRetries: 2,
		},
	}
}

// WaitForServiceHealth waits for all services to be healthy
func WaitForServiceHealth(t *testing.T, timeout time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	services := []string{
		"http://localhost:8500/v1/status/leader",
		"http://localhost:4646/v1/status/leader",
		"http://localhost:9333/cluster/status",
		"http://localhost:8888/",
	}

	for _, url := range services {
		if !waitForService(ctx, url, 1*time.Second) {
			t.Fatalf("Service %s did not become healthy within %v", url, timeout)
		}
	}
}

// Helper functions

func isServiceHealthy(ctx context.Context, url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func waitForService(ctx context.Context, url string, interval time.Duration) bool {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if isServiceHealthy(ctx, url) {
				return true
			}
		}
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
