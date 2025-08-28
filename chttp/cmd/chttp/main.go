package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/chttp/internal/server"
)

const (
	defaultConfigPath = "configs/config.yaml"
	version           = "1.0.0"
)

func main() {
	// Show usage if requested
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		showUsage()
		return
	}

	// Parse command line arguments
	configPath := parseConfigPath(os.Args)

	// Validate config file exists
	if err := validateConfigFile(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// Create and start server
	srv, err := server.NewServer(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create server: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("CHTTP Server v%s - Simple CLI-to-HTTP Bridge\n", version)
	fmt.Printf("Using config: %s\n", configPath)

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

// showUsage displays usage information
func showUsage() {
	fmt.Printf(`CHTTP Server v%s - Simple CLI-to-HTTP Bridge

Usage: chttp [options]

Options:
  -config <path>   Configuration file path (default: %s)
  -h, --help       Show this help message

API Endpoints:
  POST /api/v1/execute   Execute CLI command
  GET  /health           Health check

Example Request:
  curl -X POST http://localhost:8080/api/v1/execute \
    -H "Content-Type: application/json" \
    -H "X-API-Key: your-api-key" \
    -d '{"command": "echo", "args": ["Hello, World!"]}'

Configuration:
  The server requires a YAML configuration file with:
  - server.host and server.port
  - security.api_key
  - commands.allowed (list of allowed commands)

For more information, see the documentation.
`, version, defaultConfigPath)
}