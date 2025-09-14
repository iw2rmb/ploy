package server

import (
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/build"
	"github.com/iw2rmb/ploy/internal/debug"
	"github.com/iw2rmb/ploy/internal/env"
	"github.com/iw2rmb/ploy/internal/lifecycle"
)

// handleTriggerBuild handles build trigger requests with request-scoped storage
func (s *Server) handleTriggerBuild(c *fiber.Ctx) error {
	log.Printf("[Handler] triggerBuild ENTER method=%s url=%s app=%s sha=%s lane=%s env=%s body_len=%d",
		c.Method(), c.OriginalURL(), c.Params("app"), c.Query("sha"), c.Query("lane"), c.Query("env"), len(c.Body()))
	// Use factory pattern to get unified storage interface
	unifiedStorage, err := s.resolveUnifiedStorage()
	if err != nil {
		log.Printf("[Handler] triggerBuild resolveUnifiedStorage ERROR: %v", err)
		return c.Status(503).JSON(fiber.Map{"error": "Storage initialization failed", "details": err.Error()})
	}
	err = build.TriggerBuildWithStorage(c, unifiedStorage, s.dependencies.EnvStore)
	if err != nil {
		log.Printf("[Handler] triggerBuild EXIT with error: %v", err)
	} else {
		log.Printf("[Handler] triggerBuild EXIT success")
	}
	return err
}

// handleTriggerPlatformBuild handles platform service builds with platform namespace
func (s *Server) handleTriggerPlatformBuild(c *fiber.Ctx) error {
	log.Printf("[Handler] triggerPlatformBuild ENTER method=%s url=%s service=%s sha=%s lane=%s env=%s body_len=%d",
		c.Method(), c.OriginalURL(), c.Params("service"), c.Query("sha"), c.Query("lane"), c.Query("env"), len(c.Body()))
	// Use factory pattern to get unified storage interface
	unifiedStorage, err := s.resolveUnifiedStorage()
	if err != nil {
		log.Printf("[Handler] triggerPlatformBuild resolveUnifiedStorage ERROR: %v", err)
		return c.Status(503).JSON(fiber.Map{"error": "Storage initialization failed", "details": err.Error()})
	}
	err = build.TriggerPlatformBuildWithStorage(c, unifiedStorage, s.dependencies.EnvStore)
	if err != nil {
		log.Printf("[Handler] triggerPlatformBuild EXIT with error: %v", err)
	} else {
		log.Printf("[Handler] triggerPlatformBuild EXIT success")
	}
	return err
}

// handleTriggerAppBuild handles user application builds with apps namespace
func (s *Server) handleTriggerAppBuild(c *fiber.Ctx) error {
	log.Printf("[Handler] triggerAppBuild ENTER method=%s url=%s app=%s sha=%s lane=%s env=%s body_len=%d",
		c.Method(), c.OriginalURL(), c.Params("app"), c.Query("sha"), c.Query("lane"), c.Query("env"), len(c.Body()))
	// Use factory pattern to get unified storage interface
	unifiedStorage, err := s.resolveUnifiedStorage()
	if err != nil {
		log.Printf("[Handler] triggerAppBuild resolveUnifiedStorage ERROR: %v", err)
		return c.Status(503).JSON(fiber.Map{"error": "Storage initialization failed", "details": err.Error()})
	}
	// Async mode: accept upload and run build in background via local loopback call
	if strings.ToLower(c.Query("async", "false")) == "true" {
		app := c.Params("app")
		id, aerr := s.startAsyncBuild(c, app, c.Query("sha", "dev"), c.Query("lane", ""), c.Query("main", ""))
		if aerr != nil {
			return c.Status(500).JSON(fiber.Map{"error": aerr.Error()})
		}
		return c.Status(202).JSON(fiber.Map{
			"accepted": true,
			"id":       id,
			"status":   fmt.Sprintf("/v1/apps/%s/builds/%s/status", app, id),
		})
	}

	err = build.TriggerAppBuildWithStorage(c, unifiedStorage, s.dependencies.EnvStore)
	if err != nil {
		log.Printf("[Handler] triggerAppBuild EXIT with error: %v", err)
	} else {
		log.Printf("[Handler] triggerAppBuild EXIT success")
	}
	return err
}

// handleBuildsOptions responds to OPTIONS on /v1/apps/:app/builds for quick reachability checks
func (s *Server) handleBuildsOptions(c *fiber.Ctx) error {
	c.Set("Allow", "POST, OPTIONS")
	c.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	c.Set("Access-Control-Allow-Headers", "Content-Type, X-Target-Domain")
	c.Set("Access-Control-Max-Age", "300")
	return c.SendStatus(204)
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

// handleBuildsProbe provides a dev-only JSON POST endpoint to test ingress POST handling without binary bodies
func (s *Server) handleBuildsProbe(c *fiber.Ctx) error {
	app := c.Params("app")
	// Read small JSON body
	var payload map[string]interface{}
	_ = c.BodyParser(&payload)
	return c.JSON(fiber.Map{
		"status":  "ok",
		"app":     app,
		"len":     len(c.Body()),
		"headers": c.GetReqHeaders(),
	})
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
	// Centralized config service is required; map to legacy Root shape for clients
	if s.configService == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load storage config", "details": "config service not initialized"})
	}
	cfg := s.configService.Get()
	if cfg == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load storage config", "details": "config service returned nil"})
	}
	type legacyStorage struct {
		Provider   string `json:"provider"`
		Master     string `json:"master"`
		Filer      string `json:"filer"`
		Collection string `json:"collection"`
	}
	type legacyRoot struct {
		Storage legacyStorage `json:"storage"`
	}
	resp := legacyRoot{Storage: legacyStorage{
		Provider:   cfg.Storage.Provider,
		Master:     cfg.Storage.Endpoint,
		Filer:      cfg.Storage.Endpoint,
		Collection: cfg.Storage.Bucket,
	}}
	return c.JSON(resp)
}

// handleReloadStorageConfig handles storage configuration reload
func (s *Server) handleReloadStorageConfig(c *fiber.Ctx) error {
	if s.configService == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to reload storage config", "details": "config service not initialized"})
	}
	if err := s.configService.Reload(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to reload storage config", "details": err.Error()})
	}
	cfg := s.configService.Get()
	return c.JSON(fiber.Map{
		"reloaded": true,
		"config":   cfg,
		"message":  "Configuration reload completed",
	})
}

// handleValidateStorageConfig handles storage configuration validation
func (s *Server) handleValidateStorageConfig(c *fiber.Ctx) error {
	if s.configService == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Configuration validation failed", "details": "config service not initialized"})
	}
	if err := s.configService.Reload(); err != nil {
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
