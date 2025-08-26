package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/iw2rmb/ploy/chttp/internal/config"
	"github.com/iw2rmb/ploy/chttp/internal/server"
)

const (
	defaultConfigPath = "/etc/chttp/config.yaml"
	version           = "1.0.0"
)

func main() {
	// Parse command line arguments
	configPath := parseConfigPath(os.Args)

	// Validate environment
	if err := validateEnvironment(); err != nil {
		fmt.Fprintf(os.Stderr, "Environment validation failed: %v\n", err)
		os.Exit(1)
	}

	// Validate config file
	if err := validateConfigFile(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
		os.Exit(1)
	}

	// Create and start server
	srv, err := createServerFromConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create server: %v\n", err)
		os.Exit(1)
	}

	// Setup signal handling
	setupSignalHandling()

	fmt.Printf("CHTTP Server v%s\n", version)
	fmt.Printf("Starting with config: %s\n", configPath)

	// Start server
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

// validateEnvironment checks that the runtime environment is suitable
func validateEnvironment() error {
	// Check for required directories
	requiredDirs := []string{"/tmp"}
	for _, dir := range requiredDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return fmt.Errorf("required directory does not exist: %s", dir)
		}
	}

	// Check that we can create temporary directories
	tempDir, err := os.MkdirTemp("/tmp", "chttp-test-*")
	if err != nil {
		return fmt.Errorf("cannot create temporary directories: %w", err)
	}
	os.RemoveAll(tempDir)

	return nil
}

// validateConfigFile validates the configuration file
func validateConfigFile(configPath string) error {
	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", configPath)
	}

	// Try to load and validate the config
	_, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	return nil
}

// createServerFromConfig creates a server instance from the configuration
func createServerFromConfig(configPath string) (*server.Server, error) {
	return server.NewServer(configPath)
}

// setupSignalHandling sets up graceful shutdown signal handling
func setupSignalHandling() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-c
		fmt.Printf("\nReceived signal: %v\n", sig)
		fmt.Println("Initiating graceful shutdown...")
		// The server's Start() method handles the actual shutdown
	}()
}