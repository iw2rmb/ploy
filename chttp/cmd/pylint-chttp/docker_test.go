package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerBuildFiles(t *testing.T) {
	// Get the chttp directory path
	chttpDir := filepath.Join("..", "..")
	
	// Test that Dockerfile.pylint exists
	dockerfilePath := filepath.Join(chttpDir, "Dockerfile.pylint")
	_, err := os.Stat(dockerfilePath)
	assert.NoError(t, err, "Dockerfile.pylint should exist")
	
	// Read and validate Dockerfile content
	content, err := os.ReadFile(dockerfilePath)
	require.NoError(t, err)
	
	dockerfileStr := string(content)
	
	// Validate multi-stage build structure
	assert.Contains(t, dockerfileStr, "FROM python:3.11-alpine AS builder", "Should use Alpine Python as builder")
	assert.Contains(t, dockerfileStr, "FROM gcr.io/distroless/python3-debian11", "Should use distroless for runtime")
	
	// Validate security settings
	assert.Contains(t, dockerfileStr, "USER 1000:1000", "Should run as non-root user")
	assert.Contains(t, dockerfileStr, "EXPOSE 8080", "Should expose port 8080")
	
	// Validate Pylint installation
	assert.Contains(t, dockerfileStr, "pylint==3.0.0", "Should install specific Pylint version")
	
	// Validate health check
	assert.Contains(t, dockerfileStr, "HEALTHCHECK", "Should include health check")
	
	// Validate build optimization
	assert.Contains(t, dockerfileStr, "-ldflags=\"-w -s\"", "Should strip binary for size")
}

func TestDockerComposeConfiguration(t *testing.T) {
	chttpDir := filepath.Join("..", "..")
	composePath := filepath.Join(chttpDir, "docker-compose.yml")
	
	// Test that docker-compose.yml exists
	_, err := os.Stat(composePath)
	assert.NoError(t, err, "docker-compose.yml should exist")
	
	// Read and validate content
	content, err := os.ReadFile(composePath)
	require.NoError(t, err)
	
	composeStr := string(content)
	
	// Validate service definition
	assert.Contains(t, composeStr, "pylint-chttp:", "Should define pylint-chttp service")
	assert.Contains(t, composeStr, "dockerfile: Dockerfile.pylint", "Should use correct Dockerfile")
	
	// Validate port mapping
	assert.Contains(t, composeStr, "8080:8080", "Should map port 8080")
	
	// Validate security settings
	assert.Contains(t, composeStr, "user: \"1000:1000\"", "Should run as non-root")
	assert.Contains(t, composeStr, "read_only: true", "Should use read-only filesystem")
	assert.Contains(t, composeStr, "no-new-privileges:true", "Should prevent privilege escalation")
	
	// Validate health check
	assert.Contains(t, composeStr, "healthcheck:", "Should include health check")
	assert.Contains(t, composeStr, "/health", "Should check health endpoint")
	
	// Validate development features
	assert.Contains(t, composeStr, "CHTTP_AUTH_DISABLED=true", "Should disable auth for development")
}

func TestBuildScript(t *testing.T) {
	chttpDir := filepath.Join("..", "..")
	scriptPath := filepath.Join(chttpDir, "scripts", "build-docker.sh")
	
	// Test that build script exists
	info, err := os.Stat(scriptPath)
	assert.NoError(t, err, "build-docker.sh should exist")
	
	// Test that script is executable
	mode := info.Mode()
	assert.True(t, mode&0100 != 0, "build-docker.sh should be executable")
	
	// Read and validate script content
	content, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	
	scriptStr := string(content)
	
	// Validate script structure
	assert.Contains(t, scriptStr, "#!/bin/bash", "Should be a bash script")
	assert.Contains(t, scriptStr, "set -euo pipefail", "Should use strict mode")
	
	// Validate Docker build command
	assert.Contains(t, scriptStr, "docker build", "Should contain docker build command")
	assert.Contains(t, scriptStr, "Dockerfile.pylint", "Should use correct Dockerfile")
	
	// Validate image tagging
	assert.Contains(t, scriptStr, "$REPO/$SERVICE:$VERSION", "Should tag with version")
	assert.Contains(t, scriptStr, "$REPO/$SERVICE:latest", "Should tag as latest")
	
	// Validate size checking
	assert.Contains(t, scriptStr, "35", "Should check 35MB size limit")
	assert.Contains(t, scriptStr, "25", "Should check 25MB minimum")
	
	// Validate security features
	assert.Contains(t, scriptStr, "trivy", "Should support security scanning")
}

func TestClientTestScript(t *testing.T) {
	chttpDir := filepath.Join("..", "..")
	scriptPath := filepath.Join(chttpDir, "scripts", "test-chttp-client.sh")
	
	// Test that test script exists
	info, err := os.Stat(scriptPath)
	assert.NoError(t, err, "test-chttp-client.sh should exist")
	
	// Test that script is executable
	mode := info.Mode()
	assert.True(t, mode&0100 != 0, "test-chttp-client.sh should be executable")
	
	// Read and validate script content
	content, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	
	scriptStr := string(content)
	
	// Validate test functions
	assert.Contains(t, scriptStr, "test_health_endpoint", "Should test health endpoint")
	assert.Contains(t, scriptStr, "test_analysis_endpoint", "Should test analysis endpoint")
	assert.Contains(t, scriptStr, "wait_for_server", "Should wait for server readiness")
	
	// Validate test data creation
	assert.Contains(t, scriptStr, "create_test_archive", "Should create test data")
	assert.Contains(t, scriptStr, "import os", "Should create Python test files")
	assert.Contains(t, scriptStr, "tar -czf", "Should create tar.gz archive")
	
	// Validate HTTP testing
	assert.Contains(t, scriptStr, "curl", "Should use curl for HTTP testing")
	assert.Contains(t, scriptStr, "/health", "Should test health endpoint")
	assert.Contains(t, scriptStr, "/analyze", "Should test analysis endpoint")
}

func TestDockerfileClientConfiguration(t *testing.T) {
	chttpDir := filepath.Join("..", "..")
	dockerfilePath := filepath.Join(chttpDir, "Dockerfile.client")
	
	// Test that Dockerfile.client exists
	_, err := os.Stat(dockerfilePath)
	assert.NoError(t, err, "Dockerfile.client should exist")
	
	// Read and validate content
	content, err := os.ReadFile(dockerfilePath)
	require.NoError(t, err)
	
	dockerfileStr := string(content)
	
	// Validate base image
	assert.Contains(t, dockerfileStr, "FROM alpine:3.18", "Should use Alpine Linux")
	
	// Validate tools installation
	assert.Contains(t, dockerfileStr, "curl", "Should install curl")
	assert.Contains(t, dockerfileStr, "wget", "Should install wget") 
	assert.Contains(t, dockerfileStr, "jq", "Should install jq")
	assert.Contains(t, dockerfileStr, "tar", "Should install tar")
	
	// Validate user creation
	assert.Contains(t, dockerfileStr, "adduser -D -u 1000", "Should create test user")
	assert.Contains(t, dockerfileStr, "USER testuser", "Should run as non-root")
}

func TestREADMEDocumentation(t *testing.T) {
	chttpDir := filepath.Join("..", "..")
	readmePath := filepath.Join(chttpDir, "README.md")
	
	// Test that README exists
	_, err := os.Stat(readmePath)
	assert.NoError(t, err, "README.md should exist")
	
	// Read and validate content
	content, err := os.ReadFile(readmePath)
	require.NoError(t, err)
	
	readmeStr := string(content)
	
	// Validate key sections
	assert.Contains(t, readmeStr, "# CHTTP", "Should have main title")
	assert.Contains(t, readmeStr, "## Architecture", "Should document architecture")
	assert.Contains(t, readmeStr, "## Development", "Should document development")
	assert.Contains(t, readmeStr, "## Production Deployment", "Should document deployment")
	
	// Validate Docker documentation
	assert.Contains(t, readmeStr, "docker-compose up", "Should document Docker Compose usage")
	assert.Contains(t, readmeStr, "./scripts/build-docker.sh", "Should document build script")
	assert.Contains(t, readmeStr, "25-35MB", "Should document size target")
	
	// Validate API documentation
	assert.Contains(t, readmeStr, "GET /health", "Should document health endpoint")
	assert.Contains(t, readmeStr, "POST /analyze", "Should document analysis endpoint")
	
	// Validate security documentation
	assert.Contains(t, readmeStr, "Process Isolation", "Should document security features")
	assert.Contains(t, readmeStr, "UID 1000", "Should document non-root execution")
}

func TestConfigurationFiles(t *testing.T) {
	chttpDir := filepath.Join("..", "..")
	configPath := filepath.Join(chttpDir, "configs", "pylint-chttp-config.yaml")
	
	// Test that config exists
	_, err := os.Stat(configPath)
	assert.NoError(t, err, "pylint-chttp-config.yaml should exist")
	
	// Read and validate content
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)
	
	configStr := string(content)
	
	// Validate service configuration
	assert.Contains(t, configStr, "name: \"pylint-chttp\"", "Should configure service name")
	assert.Contains(t, configStr, "port: 8080", "Should configure port")
	
	// Validate security settings
	assert.Contains(t, configStr, "max_memory: \"512MB\"", "Should set memory limit")
	assert.Contains(t, configStr, "max_cpu: \"1.0\"", "Should set CPU limit")
	assert.Contains(t, configStr, "run_as_user: \"pylint\"", "Should set user")
	
	// Validate Pylint settings
	assert.Contains(t, configStr, "--output-format=json", "Should use JSON format")
	assert.Contains(t, configStr, ".py", "Should accept Python files")
}

func TestDirectoryStructure(t *testing.T) {
	chttpDir := filepath.Join("..", "..")
	
	// Validate expected directories exist
	expectedDirs := []string{
		"scripts",
		"configs", 
		"internal/analyzers",
		"cmd/pylint-chttp",
	}
	
	for _, dir := range expectedDirs {
		dirPath := filepath.Join(chttpDir, dir)
		info, err := os.Stat(dirPath)
		assert.NoError(t, err, "Directory %s should exist", dir)
		if err == nil {
			assert.True(t, info.IsDir(), "%s should be a directory", dir)
		}
	}
	
	// Validate expected files exist
	expectedFiles := []string{
		"go.mod",
		"Dockerfile.pylint",
		"Dockerfile.client", 
		"docker-compose.yml",
		"README.md",
		"scripts/build-docker.sh",
		"scripts/test-chttp-client.sh",
		"configs/pylint-chttp-config.yaml",
	}
	
	for _, file := range expectedFiles {
		filePath := filepath.Join(chttpDir, file)
		_, err := os.Stat(filePath)
		assert.NoError(t, err, "File %s should exist", file)
	}
}