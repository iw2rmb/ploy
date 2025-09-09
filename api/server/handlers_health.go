package server

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

// handleCoordinationHealth handles coordination and leader election health checks
func (s *Server) handleCoordinationHealth(c *fiber.Ctx) error {
	if s.dependencies.CoordinationManager == nil {
		return c.JSON(fiber.Map{
			"status":  "disabled",
			"message": "Coordination manager not initialized",
		})
	}

	isLeader := s.dependencies.CoordinationManager.IsLeader()
	status := "follower"
	if isLeader {
		status = "leader"
	}

	response := fiber.Map{
		"status":    status,
		"is_leader": isLeader,
		"timestamp": time.Now(),
	}

	// Add TTL cleanup status if we're the leader
	if isLeader {
		// Note: TTL cleanup stats would be available through the coordination manager
		// This is a placeholder for future implementation
		response["coordination_tasks"] = fiber.Map{
			"ttl_cleanup": "active",
		}
	}

	return c.JSON(response)
}
