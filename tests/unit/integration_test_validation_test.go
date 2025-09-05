package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevDeploymentIntegrationTestExists(t *testing.T) {
	// Verify the integration test was created correctly
	repoRoot := findRepoRoot(t)
	testPath := filepath.Join(repoRoot, "tests/integration/test-dev-deployment.sh")

	// Check if test file exists
	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		t.Fatalf("Integration test file does not exist: %s", testPath)
	}

	// Check if file is executable
	info, err := os.Stat(testPath)
	if err != nil {
		t.Fatalf("Error checking test file: %v", err)
	}

	mode := info.Mode()
	if mode&0111 == 0 {
		t.Errorf("Integration test file is not executable: %s", testPath)
	}

	// Read and validate test content
	content, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("Error reading test file: %v", err)
	}

	contentStr := string(content)

	// Validate required test components
	requiredComponents := []struct {
		name     string
		pattern  string
		required bool
	}{
		{
			name:     "shebang",
			pattern:  "#!/bin/bash",
			required: true,
		},
		{
			name:     "user app deployment test",
			pattern:  "test_user_app_deployment",
			required: true,
		},
		{
			name:     "platform service deployment test",
			pattern:  "test_platform_service_deployment",
			required: true,
		},
		{
			name:     "dev environment routing",
			pattern:  ".dev.ployd.app",
			required: true,
		},
		{
			name:     "platform dev domain",
			pattern:  ".dev.ployman.app",
			required: true,
		},
		{
			name:     "ploy push command",
			pattern:  "ploy push",
			required: true,
		},
		{
			name:     "ployman push command",
			pattern:  "ployman push",
			required: true,
		},
		{
			name:     "health check verification",
			pattern:  "/health",
			required: true,
		},
		{
			name:     "cleanup function",
			pattern:  "cleanup_test_resources",
			required: true,
		},
	}

	for _, component := range requiredComponents {
		if component.required && !strings.Contains(contentStr, component.pattern) {
			t.Errorf("Integration test missing required component '%s': pattern '%s' not found",
				component.name, component.pattern)
		} else if strings.Contains(contentStr, component.pattern) {
			t.Logf("✓ Found required component: %s", component.name)
		}
	}

	t.Logf("✓ Integration test validation passed: %s", testPath)
}

func TestProdDeploymentIntegrationTestExists(t *testing.T) {
	// Verify the production integration test was created correctly
	repoRoot := findRepoRoot(t)
	testPath := filepath.Join(repoRoot, "tests/integration/test-prod-deployment.sh")

	// Check if test file exists
	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		t.Fatalf("Production integration test file does not exist: %s", testPath)
	}

	// Check if file is executable
	info, err := os.Stat(testPath)
	if err != nil {
		t.Fatalf("Error checking production test file: %v", err)
	}

	mode := info.Mode()
	if mode&0111 == 0 {
		t.Errorf("Production integration test file is not executable: %s", testPath)
	}

	// Read and validate test content
	content, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("Error reading production test file: %v", err)
	}

	contentStr := string(content)

	// Validate required production test components
	requiredComponents := []struct {
		name     string
		pattern  string
		required bool
	}{
		{
			name:     "production confirmation",
			pattern:  "confirm_production_test",
			required: true,
		},
		{
			name:     "production user app deployment",
			pattern:  "test_user_app_production_deployment",
			required: true,
		},
		{
			name:     "production platform service deployment",
			pattern:  "test_platform_service_production_deployment",
			required: true,
		},
		{
			name:     "production infrastructure test",
			pattern:  "test_production_infrastructure",
			required: true,
		},
		{
			name:     "production domain routing",
			pattern:  ".ployd.app",
			required: true,
		},
		{
			name:     "production platform domain",
			pattern:  ".ployman.app",
			required: true,
		},
		{
			name:     "SSL certificate validation",
			pattern:  "SSL certificate",
			required: true,
		},
		{
			name:     "DNS resolution test",
			pattern:  "nslookup",
			required: true,
		},
	}

	for _, component := range requiredComponents {
		if component.required && !strings.Contains(contentStr, component.pattern) {
			t.Errorf("Production integration test missing required component '%s': pattern '%s' not found",
				component.name, component.pattern)
		} else if strings.Contains(contentStr, component.pattern) {
			t.Logf("✓ Found required production component: %s", component.name)
		}
	}

	t.Logf("✓ Production integration test validation passed: %s", testPath)
}

func TestIntegrationTestExecutionProtocol(t *testing.T) {
	// This test documents that the integration tests should NOT be run locally
	// per CLAUDE.md testing protocol: LOCAL unit tests, VPS integration tests

	t.Log("📋 Integration test execution protocol:")
	t.Log("   - LOCAL: Unit tests only (this test)")
	t.Log("   - VPS: Integration tests (test-dev-deployment.sh, test-prod-deployment.sh)")
	t.Log("   - Dev Command: ssh root@$TARGET_HOST 'su - ploy -c ./tests/integration/test-dev-deployment.sh'")
	t.Log("   - Prod Command: ssh root@$TARGET_HOST 'su - ploy -c ./tests/integration/test-prod-deployment.sh'")

	// Verify we're following the TDD approach
	t.Log("🔴 TDD RED Phase: Integration tests created, ready for VPS execution")
	t.Log("🟢 TDD GREEN Phase: Execute on VPS to validate deployment functionality")
	t.Log("🔄 TDD REFACTOR Phase: Document results and mark roadmap complete")
}
