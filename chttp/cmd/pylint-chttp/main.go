package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/chttp/internal/server"
)

const (
	defaultConfigPath = "configs/pylint-chttp-service.yaml"
	serviceName       = "pylint-chttp"
	version           = "1.0.0"
	pylintVersion     = "3.0.0"
)

func main() {
	// Show usage if requested
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		showUsage()
		return
	}

	// Show version if requested
	if len(os.Args) > 1 && (os.Args[1] == "-v" || os.Args[1] == "--version") {
		showVersion()
		return
	}

	// Parse command line arguments
	configPath := parseConfigPath(os.Args)

	// Validate config file exists
	if err := validateConfigFile(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// Validate Pylint installation
	if err := validatePylintInstallation(); err != nil {
		fmt.Fprintf(os.Stderr, "Pylint validation error: %v\n", err)
		os.Exit(1)
	}

	// Create and start specialized Pylint server
	srv, err := server.NewPylintServer(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create server: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Pylint CHTTP Service v%s - Python Static Analysis over HTTP\n", version)
	fmt.Printf("Using Pylint v%s\n", pylintVersion)
	fmt.Printf("Config: %s\n", configPath)
	fmt.Printf("Ready to analyze Python code via HTTP...\n")

	// Start server (blocks until shutdown)
	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

// parseConfigPath extracts the config path from command line arguments
func parseConfigPath(args []string) string {
	for i, arg := range args {
		if arg == "-config" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, "-config=") {
			return strings.TrimPrefix(arg, "-config=")
		}
	}
	return defaultConfigPath
}

// validateConfigFile checks that the configuration file exists
func validateConfigFile(configPath string) error {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", configPath)
	}
	return nil
}

// validatePylintInstallation checks that Pylint is available and working
func validatePylintInstallation() error {
	// Check if pylint command is available
	if _, err := os.Stat("/usr/local/bin/pylint"); err != nil {
		if _, err := os.Stat("/usr/bin/pylint"); err != nil {
			// Try to find pylint in PATH
			if err := checkCommandInPath("pylint"); err != nil {
				return fmt.Errorf("pylint not found in system PATH")
			}
		}
	}
	
	// TODO: Could add version check here if needed
	// cmd := exec.Command("pylint", "--version")
	// output, err := cmd.CombinedOutput()
	
	return nil
}

// checkCommandInPath checks if a command exists in system PATH
func checkCommandInPath(command string) error {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return fmt.Errorf("PATH environment variable not set")
	}
	
	paths := strings.Split(pathEnv, ":")
	for _, path := range paths {
		if path == "" {
			continue
		}
		fullPath := path + "/" + command
		if _, err := os.Stat(fullPath); err == nil {
			return nil
		}
	}
	
	return fmt.Errorf("command %s not found in PATH", command)
}

// showVersion displays version information
func showVersion() {
	fmt.Printf("Pylint CHTTP Service v%s\n", version)
	fmt.Printf("Built for Pylint v%s\n", pylintVersion)
	fmt.Printf("Service: %s\n", serviceName)
}

// showUsage displays usage information
func showUsage() {
	fmt.Printf(`Pylint CHTTP Service v%s - Python Static Analysis over HTTP

Usage: %s [options]

Options:
  -config <path>   Configuration file path (default: %s)
  -v, --version    Show version information
  -h, --help       Show this help message

API Endpoints:
  POST /analyze            Analyze Python code archive
  POST /api/v1/execute     Execute CLI commands (limited to Python tools)
  GET  /health             Health check and service status

Example Analysis Request:
  curl -X POST http://localhost:8080/analyze \
    -H "Content-Type: application/gzip" \
    -H "X-API-Key: your-api-key" \
    --data-binary @python-code.tar.gz

Example CLI Request:
  curl -X POST http://localhost:8080/api/v1/execute \
    -H "Content-Type: application/json" \
    -H "X-API-Key: your-api-key" \
    -d '{"command": "pylint", "args": ["--version"]}'

Service Configuration:
  The service requires a YAML configuration file with:
  - server.host and server.port
  - security.api_key
  - commands.allowed (Python analysis tools only)
  - pylint-specific settings for analysis

Features:
  - Python code static analysis via Pylint
  - Secure sandboxed execution environment
  - JSON result format compatible with ARF workflows
  - Integration with Ploy deployment platform
  - Health monitoring and service discovery

For detailed documentation, see the Ploy static analysis integration guide.
`, version, serviceName, defaultConfigPath)
}