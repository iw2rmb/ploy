package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCHTTPIntegrationScript(t *testing.T) {
	// Get the test script path
	scriptPath := filepath.Join(".", "test-chttp-integration.sh")

	// Test that script exists
	info, err := os.Stat(scriptPath)
	assert.NoError(t, err, "test-chttp-integration.sh should exist")

	if err == nil {
		// Test that script is executable
		mode := info.Mode()
		assert.True(t, mode&0100 != 0, "test-chttp-integration.sh should be executable")
	}

	// Read and validate script content
	content, err := os.ReadFile(scriptPath)
	require.NoError(t, err)

	scriptStr := string(content)

	// Validate script structure
	assert.Contains(t, scriptStr, "#!/bin/bash", "Should be a bash script")
	assert.Contains(t, scriptStr, "set -euo pipefail", "Should use strict mode")

	// Validate required functions
	requiredFunctions := []string{
		"validate_environment",
		"deploy_chttp_services",
		"deploy_api",
		"wait_for_service",
		"test_chttp_health",
		"test_python_analysis",
		"test_controller_health",
		"generate_report",
	}

	for _, function := range requiredFunctions {
		assert.Contains(t, scriptStr, function+"()", "Should contain function: %s", function)
	}

	// Validate environment variables
	requiredEnvVars := []string{
		"TARGET_HOST",
		"CHTTP_SERVICE_URL",
		"API_BASE_URL",
		"TEST_TIMEOUT",
	}

	for _, envVar := range requiredEnvVars {
		assert.Contains(t, scriptStr, envVar, "Should reference environment variable: %s", envVar)
	}

	// Validate Ansible integration
	assert.Contains(t, scriptStr, "ansible-playbook site.yml", "Should call Ansible playbook")
	assert.Contains(t, scriptStr, "-e deploy_chttp=true", "Should enable CHTTP deployment")

	// Validate API testing
	assert.Contains(t, scriptStr, "/health", "Should test health endpoints")
	assert.Contains(t, scriptStr, "/analyze", "Should test analysis endpoint")
	assert.Contains(t, scriptStr, "python", "Should test Python analysis")

	// Validate test reporting
	assert.Contains(t, scriptStr, "TESTS_TOTAL", "Should track test counts")
	assert.Contains(t, scriptStr, "TESTS_PASSED", "Should track passed tests")
	assert.Contains(t, scriptStr, "TESTS_FAILED", "Should track failed tests")
}

func TestAnsibleCHTTPPlaybook(t *testing.T) {
	// Get the playbook path
	playbookPath := filepath.Join("..", "..", "iac", "dev", "playbooks", "chttp.yml")

	// Test that playbook exists
	_, err := os.Stat(playbookPath)
	assert.NoError(t, err, "chttp.yml playbook should exist")

	// Read and validate playbook content
	content, err := os.ReadFile(playbookPath)
	require.NoError(t, err)

	playbookStr := string(content)

	// Validate YAML structure
	assert.Contains(t, playbookStr, "---", "Should be valid YAML")
	assert.Contains(t, playbookStr, "name: Deploy CHTTP Services", "Should have correct playbook name")

	// Validate host targeting
	assert.Contains(t, playbookStr, "hosts: linux_hosts", "Should target linux hosts")
	assert.Contains(t, playbookStr, "become: true", "Should use privilege escalation")

	// Validate variable files
	assert.Contains(t, playbookStr, "vars_files:", "Should include variable files")
	assert.Contains(t, playbookStr, "../vars/main.yml", "Should include main variables")

	// Validate CHTTP user management
	assert.Contains(t, playbookStr, "chttp_user", "Should define CHTTP user")
	assert.Contains(t, playbookStr, "Create CHTTP system user", "Should create system user")

	// Validate service deployment
	assert.Contains(t, playbookStr, "chttp_services:", "Should define services")
	assert.Contains(t, playbookStr, "pylint-chttp", "Should include Pylint service")

	// Validate systemd integration
	assert.Contains(t, playbookStr, "systemd:", "Should manage systemd services")
	assert.Contains(t, playbookStr, "enabled: true", "Should enable services")

	// Validate health checks
	assert.Contains(t, playbookStr, "/health", "Should include health checks")
	assert.Contains(t, playbookStr, "uri:", "Should use URI module for health checks")

	// Validate security
	assert.Contains(t, playbookStr, "owner: \"{{ chttp_user }}\"", "Should set proper ownership")
	assert.Contains(t, playbookStr, "mode:", "Should set file permissions")
}

func TestAnsibleSiteYmlIntegration(t *testing.T) {
	// Get the site.yml path
	siteYmlPath := filepath.Join("..", "..", "iac", "dev", "site.yml")

	// Test that site.yml exists
	_, err := os.Stat(siteYmlPath)
	assert.NoError(t, err, "site.yml should exist")

	// Read and validate content
	content, err := os.ReadFile(siteYmlPath)
	require.NoError(t, err)

	siteYmlStr := string(content)

	// Validate CHTTP playbook inclusion
	assert.Contains(t, siteYmlStr, "playbooks/chttp.yml", "Should include CHTTP playbook")
	assert.Contains(t, siteYmlStr, "when: deploy_chttp is defined", "Should have conditional deployment")
	assert.Contains(t, siteYmlStr, "deploy_chttp | bool", "Should check boolean value")
}

func TestAnsibleTemplates(t *testing.T) {
	templatesDir := filepath.Join("..", "..", "iac", "common", "templates")

	// Expected template files
	expectedTemplates := []string{
		"pylint-chttp-config.yaml.j2",
		"chttp-service.service.j2",
		"chttp-logrotate.j2",
		"traefik-chttp.yml.j2",
	}

	for _, template := range expectedTemplates {
		templatePath := filepath.Join(templatesDir, template)

		// Test that template exists
		_, err := os.Stat(templatePath)
		assert.NoError(t, err, "Template should exist: %s", template)

		// Read and validate template content
		content, err := os.ReadFile(templatePath)
		require.NoError(t, err, "Should be able to read template: %s", template)

		templateStr := string(content)

		// Validate Jinja2 templating
		assert.True(t,
			strings.Contains(templateStr, "{{") || strings.Contains(templateStr, "{%"),
			"Template should contain Jinja2 syntax: %s", template)

		// Template-specific validations
		switch template {
		case "pylint-chttp-config.yaml.j2":
			assert.Contains(t, templateStr, "service:", "Should define service configuration")
			assert.Contains(t, templateStr, "pylint-chttp", "Should configure Pylint service")
			assert.Contains(t, templateStr, "security:", "Should include security settings")

		case "chttp-service.service.j2":
			assert.Contains(t, templateStr, "[Unit]", "Should be valid systemd unit")
			assert.Contains(t, templateStr, "[Service]", "Should define service section")
			assert.Contains(t, templateStr, "[Install]", "Should include install section")
			assert.Contains(t, templateStr, "{{ chttp_user }}", "Should use CHTTP user variable")

		case "chttp-logrotate.j2":
			assert.Contains(t, templateStr, "daily", "Should configure daily rotation")
			assert.Contains(t, templateStr, "{{ chttp_log_dir }}", "Should use log directory variable")

		case "traefik-chttp.yml.j2":
			assert.Contains(t, templateStr, "http:", "Should define HTTP routes")
			assert.Contains(t, templateStr, "routers:", "Should define Traefik routers")
			assert.Contains(t, templateStr, "/health", "Should include health check paths")
		}
	}
}

func TestVPSIntegrationTestComponents(t *testing.T) {
	// Test script components that are critical for VPS integration
	scriptPath := filepath.Join(".", "test-chttp-integration.sh")
	content, err := os.ReadFile(scriptPath)
	require.NoError(t, err)

	scriptStr := string(content)

	// Validate VPS-specific requirements
	vpsRequirements := []string{
		"ssh",                 // SSH access to VPS
		"ansible-playbook",    // Ansible for deployment
		"TARGET_HOST",         // VPS target specification
		"/health",             // Health check endpoints
		"api.dev.ployman.app", // Production-like API URL
		"deploy_chttp=true",   // CHTTP deployment flag
		"python",              // Python analysis testing
		"curl",                // HTTP client for testing
	}

	for _, requirement := range vpsRequirements {
		assert.Contains(t, scriptStr, requirement,
			"VPS integration should include: %s", requirement)
	}

	// Validate test result reporting
	reportingFeatures := []string{
		"TESTS_TOTAL",
		"TESTS_PASSED",
		"TESTS_FAILED",
		"generate_report",
		"track_test",
	}

	for _, feature := range reportingFeatures {
		assert.Contains(t, scriptStr, feature,
			"Should include reporting feature: %s", feature)
	}
}
