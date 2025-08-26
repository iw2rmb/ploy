package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
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

	// Register routes
	api := app.Group("/v1/openrewrite")

	// Health endpoints - simplified for now
	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "healthy",
			"version": "1.0.0",
		})
	})
	api.Get("/ready", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ready": true})
	})

	// Transform endpoint - placeholder
	api.Post("/transform", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message": "Transform endpoint - to be implemented",
		})
	})

	// Job endpoints - placeholders
	api.Post("/jobs", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "Create job - to be implemented"})
	})
	api.Get("/jobs/:id", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "Get job - to be implemented"})
	})
	api.Get("/jobs/:id/status", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "Get job status - to be implemented"})
	})
	api.Get("/jobs/:id/diff", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "Get job diff - to be implemented"})
	})
	api.Delete("/jobs/:id", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "Cancel job - to be implemented"})
	})

	// Metrics endpoint
	api.Get("/metrics", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "Metrics - to be implemented"})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	log.Printf("OpenRewrite Service starting on port %s", port)
	log.Printf("Consul: %s, SeaweedFS: %s", consulAddr, seaweedAddr)
	log.Fatal(app.Listen(":" + port))
}