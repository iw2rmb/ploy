package server

import (
    "github.com/gofiber/fiber/v2"
    "github.com/iw2rmb/ploy/api/config"
    "github.com/iw2rmb/ploy/internal/build"
    "github.com/iw2rmb/ploy/internal/debug"
    "github.com/iw2rmb/ploy/internal/env"
    "github.com/iw2rmb/ploy/internal/lifecycle"
)

// handleTriggerBuild handles build trigger requests with request-scoped storage
func (s *Server) handleTriggerBuild(c *fiber.Ctx) error {
	// Use factory pattern to get unified storage interface
    unifiedStorage, err := s.resolveUnifiedStorage()
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Storage initialization failed", "details": err.Error()})
	}
	return build.TriggerBuildWithStorage(c, unifiedStorage, s.dependencies.EnvStore)
}

// handleTriggerPlatformBuild handles platform service builds with Harbor platform namespace
func (s *Server) handleTriggerPlatformBuild(c *fiber.Ctx) error {
	// Use factory pattern to get unified storage interface
    unifiedStorage, err := s.resolveUnifiedStorage()
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Storage initialization failed", "details": err.Error()})
	}
	return build.TriggerPlatformBuildWithStorage(c, unifiedStorage, s.dependencies.EnvStore)
}

// handleTriggerAppBuild handles user application builds with Harbor apps namespace
func (s *Server) handleTriggerAppBuild(c *fiber.Ctx) error {
	// Use factory pattern to get unified storage interface
    unifiedStorage, err := s.resolveUnifiedStorage()
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Storage initialization failed", "details": err.Error()})
	}
	return build.TriggerAppBuildWithStorage(c, unifiedStorage, s.dependencies.EnvStore)
}

// handleDestroyApp handles app destruction with request-scoped storage
func (s *Server) handleDestroyApp(c *fiber.Ctx) error {
	// Use factory pattern to get unified storage interface
    unifiedStorage, err := s.resolveUnifiedStorage()
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Storage initialization failed", "details": err.Error()})
	}
	// Use the new DestroyAppWithStorage function that accepts unified storage
	return lifecycle.DestroyAppWithStorage(c, unifiedStorage, s.dependencies.EnvStore)
}

// handleStorageHealth handles storage health checks with request-scoped client
func (s *Server) handleStorageHealth(c *fiber.Ctx) error {
	storeClient, err := s.getStorageClient()
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Storage client initialization failed", "details": err.Error()})
	}
	// Use the new Health method from Storage interface
	ctx := c.Context()
	if err := storeClient.Health(ctx); err != nil {
		return c.Status(503).JSON(fiber.Map{"status": "unhealthy", "error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "healthy"})
}

// handleStorageMetrics handles storage metrics with request-scoped client
func (s *Server) handleStorageMetrics(c *fiber.Ctx) error {
	storeClient, err := s.getStorageClient()
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Storage client initialization failed", "details": err.Error()})
	}
	// Use the new Metrics method from Storage interface
	metrics := storeClient.Metrics()
	return c.JSON(metrics)
}

// handleGetStorageConfig handles storage configuration retrieval
func (s *Server) handleGetStorageConfig(c *fiber.Ctx) error {
	configManager := config.NewConfigManager(s.dependencies.StorageConfigPath)
	rootConfig, err := configManager.LoadConfig()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load storage config", "details": err.Error()})
	}
	return c.JSON(rootConfig)
}

// handleReloadStorageConfig handles storage configuration reload
func (s *Server) handleReloadStorageConfig(c *fiber.Ctx) error {
	configManager := config.NewConfigManager(s.dependencies.StorageConfigPath)
	rootConfig, reloaded, err := configManager.ReloadIfChanged()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to reload storage config", "details": err.Error()})
	}
	return c.JSON(fiber.Map{
		"reloaded": reloaded,
		"config":   rootConfig,
		"message":  "Configuration reload completed",
	})
}

// handleValidateStorageConfig handles storage configuration validation
func (s *Server) handleValidateStorageConfig(c *fiber.Ctx) error {
	_, err := config.Load(s.dependencies.StorageConfigPath)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Configuration validation failed", "details": err.Error()})
	}
	return c.JSON(fiber.Map{"valid": true, "message": "Configuration is valid"})
}

// handleSetEnvVars handles setting environment variables with injected env store
func (s *Server) handleSetEnvVars(c *fiber.Ctx) error {
	return env.SetEnvVars(c, s.dependencies.EnvStore)
}

// handleGetEnvVars handles getting environment variables with injected env store
func (s *Server) handleGetEnvVars(c *fiber.Ctx) error {
	return env.GetEnvVars(c, s.dependencies.EnvStore)
}

// handleSetEnvVar handles setting single environment variable with injected env store
func (s *Server) handleSetEnvVar(c *fiber.Ctx) error {
	return env.SetEnvVar(c, s.dependencies.EnvStore)
}

// handleDeleteEnvVar handles deleting environment variable with injected env store
func (s *Server) handleDeleteEnvVar(c *fiber.Ctx) error {
	return env.DeleteEnvVar(c, s.dependencies.EnvStore)
}

// handleDebugApp handles debug app requests with injected env store
func (s *Server) handleDebugApp(c *fiber.Ctx) error {
	return debug.DebugApp(c, s.dependencies.EnvStore)
}
