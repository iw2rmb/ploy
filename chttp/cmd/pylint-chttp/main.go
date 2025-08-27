package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/iw2rmb/ploy/chttp/internal/server"
)

const (
	defaultPort        = 8080
	serviceName        = "pylint-chttp"
	pylintExecutable   = "pylint"
	version           = "1.0.0"
)

func main() {
	fmt.Printf("Pylint CHTTP Service v%s\n", version)
	
	// Validate environment before starting
	if err := validatePylintEnvironment(); err != nil {
		fmt.Fprintf(os.Stderr, "Environment validation failed: %v\n", err)
		os.Exit(1)
	}
	
	// Create Pylint-specific configuration file
	configPath, err := createTempPylintConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create Pylint config: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(filepath.Dir(configPath))
	
	fmt.Printf("Using Pylint configuration: %s\n", configPath)
	
	// Create and start the CHTTP server with Pylint configuration
	srv, err := server.NewServer(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create Pylint CHTTP server: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("Starting %s on port %d\n", serviceName, defaultPort)
	
	// Start server
	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

// validatePylintEnvironment checks that Pylint is available and working
func validatePylintEnvironment() error {
	// Check if Pylint executable exists
	if _, err := exec.LookPath(pylintExecutable); err != nil {
		return fmt.Errorf("pylint executable not found in PATH: %w", err)
	}
	
	// Test Pylint version
	cmd := exec.Command(pylintExecutable, "--version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get pylint version: %w", err)
	}
	
	fmt.Printf("Found Pylint: %s", string(output))
	return nil
}

// createTempPylintConfig creates a temporary Pylint-specific CHTTP configuration
func createTempPylintConfig() (string, error) {
	tempDir, err := os.MkdirTemp("", "pylint-chttp-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	
	configPath := filepath.Join(tempDir, "pylint-config.yaml")
	configContent := createPylintConfig()
	
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to write config file: %w", err)
	}
	
	return configPath, nil
}

// createPylintConfig returns the YAML configuration for Pylint CHTTP service
func createPylintConfig() string {
	return fmt.Sprintf(`
service:
  name: "%s"
  port: %d

executable:
  path: "%s"
  args: ["--output-format=json", "--reports=no"]
  timeout: "5m"

security:
  auth_method: "public_key"
  public_key_path: "/etc/chttp/public.pem"
  run_as_user: "pylint"
  max_memory: "512MB"
  max_cpu: "1.0"

input:
  formats: ["tar.gz", "tar", "zip"]
  allowed_extensions: [".py", ".pyw"]
  max_archive_size: "100MB"

output:
  format: "json"
  parser: "pylint_json"
`, serviceName, defaultPort, pylintExecutable)
}

// getPylintServiceInfo returns service name and port for testing
func getPylintServiceInfo() (string, int) {
	return serviceName, defaultPort
}