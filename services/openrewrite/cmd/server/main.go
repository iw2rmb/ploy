package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	
	"github.com/iw2rmb/ploy/services/openrewrite/internal/executor"
	"github.com/iw2rmb/ploy/services/openrewrite/internal/handlers"
	"github.com/iw2rmb/ploy/services/openrewrite/internal/storage"
)

func main() {
	app := fiber.New(fiber.Config{
		AppName: "OpenRewrite Service",
	})

	// Middleware
	app.Use(logger.New())
	app.Use(recover.New())

	// Initialize storage clients
	consulAddr := os.Getenv("CONSUL_ADDRESS")
	if consulAddr == "" {
		consulAddr = "consul.service.consul:8500"
	}

	seaweedAddr := os.Getenv("SEAWEEDFS_MASTER")
	if seaweedAddr == "" {
		seaweedAddr = "seaweedfs.service.consul:9333"
	}

	// Create components
	storageClient := storage.NewOpenRewriteStorage(consulAddr, seaweedAddr)
	exec := executor.NewExecutor(executor.DefaultConfig())
	handler := handlers.NewHandlerWithStorage(exec, storageClient)

	// Register routes
	api := app.Group("/v1/openrewrite")

	// Health endpoints
	api.Get("/health", handler.HandleHealth)
	api.Get("/ready", handler.HandleReady)

	// Transform endpoints (synchronous)
	api.Post("/transform", handler.HandleTransform)

	// Job endpoints (asynchronous) 
	api.Post("/jobs", handler.HandleCreateJob)
	api.Get("/jobs/:id", handler.HandleGetJob)
	api.Get("/jobs/:id/status", handler.HandleGetJobStatus)
	api.Get("/jobs/:id/diff", handler.HandleGetJobDiff)
	api.Delete("/jobs/:id", handler.HandleCancelJob)

	// Metrics endpoint
	api.Get("/metrics", handler.HandleMetrics)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	log.Printf("OpenRewrite Service starting on port %s", port)
	log.Printf("Consul: %s, SeaweedFS: %s", consulAddr, seaweedAddr)
	log.Fatal(app.Listen(":" + port))
}