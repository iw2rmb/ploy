package server

import (
	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/config"
	"github.com/iw2rmb/ploy/api/platform"
)

// handlePlatformDeploy handles platform service deployment
func (s *Server) handlePlatformDeploy(c *fiber.Ctx) error {
	// Use factory pattern to get unified storage interface
	storage, err := config.CreateStorageFromFactory(s.dependencies.StorageConfigPath)
	if err != nil {
		return c.Status(503).JSON(fiber.Map{
			"error":   "Storage initialization failed",
			"details": err.Error(),
		})
	}

	handler := platform.NewHandlerWithStorage(storage, s.dependencies.EnvStore)
	return handler.DeployPlatformService(c)
}

// handlePlatformStatus handles platform service status requests
func (s *Server) handlePlatformStatus(c *fiber.Ctx) error {
	// Use factory pattern to get unified storage interface
	storage, err := config.CreateStorageFromFactory(s.dependencies.StorageConfigPath)
	if err != nil {
		return c.Status(503).JSON(fiber.Map{
			"error":   "Storage initialization failed",
			"details": err.Error(),
		})
	}

	handler := platform.NewHandlerWithStorage(storage, s.dependencies.EnvStore)
	return handler.GetPlatformStatus(c)
}

// handlePlatformRollback handles platform service rollback
func (s *Server) handlePlatformRollback(c *fiber.Ctx) error {
	serviceName := c.Params("service")
	targetVersion := c.Query("version")

	if targetVersion == "" {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Target version is required",
			"details": "Provide version parameter",
		})
	}

	// TODO: Implement platform rollback logic
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Platform service rollback initiated",
		"service": serviceName,
		"version": targetVersion,
	})
}

// handlePlatformRemove handles platform service removal
func (s *Server) handlePlatformRemove(c *fiber.Ctx) error {
	serviceName := c.Params("service")

	// TODO: Implement platform service removal
	// This should:
	// 1. Stop the Nomad job
	// 2. Clean up storage artifacts
	// 3. Remove DNS entries
	// 4. Clean up certificates

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Platform service removal initiated",
		"service": serviceName,
	})
}

// handlePlatformLogs handles platform service log retrieval
func (s *Server) handlePlatformLogs(c *fiber.Ctx) error {
	serviceName := c.Params("service")
	lines := c.QueryInt("lines", 100)
	follow := c.QueryBool("follow", false)

	// TODO: Implement platform log streaming
	// This should connect to Nomad API to stream logs

	return c.JSON(fiber.Map{
		"service": serviceName,
		"lines":   lines,
		"follow":  follow,
		"logs": []string{
			"Platform service log streaming not yet implemented",
		},
	})
}
