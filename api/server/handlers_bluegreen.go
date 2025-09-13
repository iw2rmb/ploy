package server

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
)

// handleStartBlueGreenDeployment starts a new blue-green deployment
func (s *Server) handleStartBlueGreenDeployment(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
	}

	if s.dependencies.BlueGreenManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Blue-Green deployment not available"})
	}

	// Parse request body for version information
	var req struct {
		Version string `json:"version"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Version == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Version is required"})
	}

	// Start blue-green deployment
	ctx := c.Context()
	state, err := s.dependencies.BlueGreenManager.StartBlueGreenDeployment(ctx, appName, req.Version)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to start blue-green deployment: %v", err),
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"status":     "deployment_started",
		"message":    "Blue-green deployment initiated successfully",
		"deployment": state,
	})
}

// handleGetBlueGreenStatus gets the current blue-green deployment status
func (s *Server) handleGetBlueGreenStatus(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
	}

	if s.dependencies.BlueGreenManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Blue-Green deployment not available"})
	}

	// Get deployment state
	ctx := c.Context()
	state, err := s.dependencies.BlueGreenManager.GetDeploymentState(ctx, appName)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to get deployment state: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"status":     "success",
		"deployment": state,
	})
}

// handleShiftTraffic manually shifts traffic between blue and green deployments
func (s *Server) handleShiftTraffic(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
	}

	if s.dependencies.BlueGreenManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Blue-Green deployment not available"})
	}

	// Parse request body for target weight
	var req struct {
		TargetWeight int `json:"target_weight"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.TargetWeight < 0 || req.TargetWeight > 100 {
		return c.Status(400).JSON(fiber.Map{"error": "Target weight must be between 0 and 100"})
	}

	// Shift traffic
	ctx := c.Context()
	if err := s.dependencies.BlueGreenManager.ShiftTraffic(ctx, appName, req.TargetWeight); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to shift traffic: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"status":        "success",
		"message":       "Traffic shifted successfully",
		"target_weight": req.TargetWeight,
	})
}

// handleAutoShiftTraffic automatically shifts traffic using the default strategy
func (s *Server) handleAutoShiftTraffic(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
	}

	if s.dependencies.BlueGreenManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Blue-Green deployment not available"})
	}

	// Start automatic traffic shifting in background
	ctx := c.Context()
	go func() {
		if err := s.dependencies.BlueGreenManager.AutoShiftTraffic(ctx, appName); err != nil {
			log.Printf("Auto traffic shift failed for app %s: %v", appName, err)
		}
	}()

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Automatic traffic shifting started",
	})
}

// handleCompleteBlueGreenDeployment completes the blue-green deployment
func (s *Server) handleCompleteBlueGreenDeployment(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
	}

	if s.dependencies.BlueGreenManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Blue-Green deployment not available"})
	}

	// Complete deployment
	ctx := c.Context()
	if err := s.dependencies.BlueGreenManager.CompleteDeployment(ctx, appName); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to complete deployment: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Blue-green deployment completed successfully",
	})
}

// handleRollbackBlueGreenDeployment rolls back the blue-green deployment
func (s *Server) handleRollbackBlueGreenDeployment(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
	}

	if s.dependencies.BlueGreenManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Blue-Green deployment not available"})
	}

	// Rollback deployment
	ctx := c.Context()
	if err := s.dependencies.BlueGreenManager.RollbackDeployment(ctx, appName); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to rollback deployment: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Blue-green deployment rolled back successfully",
	})
}
