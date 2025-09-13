package server

import (
	"github.com/gofiber/fiber/v2"
)

// createMockServer provides a minimal Server instance suitable for handler tests.
// It initializes a Fiber app and empty dependencies; tests can override fields.
func createMockServer() *Server {
	return &Server{
		app:          fiber.New(),
		config:       &ControllerConfig{},
		dependencies: &ServiceDependencies{},
	}
}
