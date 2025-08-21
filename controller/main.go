package main

import (
	"log"
	"os"

	"github.com/ploy/ploy/controller/server"
)

func main() {
	log.Printf("Starting Ploy Controller with stateless initialization patterns")

	// Load configuration from environment variables
	config := server.LoadConfigFromEnv()

	// Create server with dependency injection
	srv, err := server.NewServer(config)
	if err != nil {
		log.Printf("Failed to create server: %v", err)
		os.Exit(1)
	}

	// Start server with graceful shutdown
	if err := srv.Start(); err != nil {
		log.Printf("Server shutdown error: %v", err)
		os.Exit(1)
	}

	log.Printf("Ploy Controller shutdown completed")
}