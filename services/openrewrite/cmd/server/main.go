package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/iw2rmb/ploy/services/openrewrite/internal/executor"
	"github.com/iw2rmb/ploy/services/openrewrite/internal/handlers"
	"github.com/iw2rmb/ploy/services/openrewrite/internal/jobs"
	"github.com/iw2rmb/ploy/services/openrewrite/internal/storage"
)

func main() {
	app := fiber.New(fiber.Config{
		AppName: "OpenRewrite Service",
	})

	// Middleware
	app.Use(logger.New())
	app.Use(recover.New())

	// Get configuration from environment
	consulAddr := os.Getenv("CONSUL_ADDRESS")
	if consulAddr == "" {
		consulAddr = "consul.service.consul:8500"
	}

	seaweedAddr := os.Getenv("SEAWEEDFS_MASTER")
	if seaweedAddr == "" {
		seaweedAddr = "seaweedfs.service.consul:9333"
	}

	// Initialize components
	log.Printf("Initializing storage clients...")
	storageClient, err := storage.NewStorageClient(consulAddr, seaweedAddr)
	if err != nil {
		log.Fatalf("Failed to create storage client: %v", err)
	}

	log.Printf("Initializing executor...")
	exec := executor.New()

	log.Printf("Initializing job manager...")
	jobManager := jobs.NewManager(exec, storageClient)

	log.Printf("Initializing HTTP handlers...")
	handler := handlers.New(exec, jobManager, storageClient)

	// Register routes
	api := app.Group("/v1/openrewrite")

	// Health endpoints
	api.Get("/health", handler.Health)
	api.Get("/ready", handler.Ready)

	// Transform endpoint (synchronous)
	api.Post("/transform", handler.Transform)

	// Job endpoints (asynchronous)
	api.Post("/jobs", handler.CreateJob)
	api.Get("/jobs/:id", handler.GetJob)
	api.Get("/jobs/:id/status", handler.GetJobStatus)
	api.Get("/jobs/:id/diff", handler.GetJobDiff)
	api.Delete("/jobs/:id", handler.CancelJob)

	// Metrics endpoint
	api.Get("/metrics", handler.Metrics)

	// Setup graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Printf("Shutting down OpenRewrite Service...")
		jobManager.Shutdown()
		app.Shutdown()
		os.Exit(0)
	}()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	log.Printf("OpenRewrite Service starting on port %s", port)
	log.Printf("Consul: %s, SeaweedFS: %s", consulAddr, seaweedAddr)
	log.Printf("All components initialized successfully")
	
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}