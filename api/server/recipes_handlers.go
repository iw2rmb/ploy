package server

import (
    "github.com/gofiber/fiber/v2"
)

// handleARFRecipesPing is a minimal endpoint to exercise the internal ARF recipes facade.
func (s *Server) handleARFRecipesPing(c *fiber.Ctx) error {
    if s.dependencies == nil || s.dependencies.ARFRecipes == nil {
        return c.Status(503).JSON(fiber.Map{"error": "recipes registry unavailable"})
    }
    if err := s.dependencies.ARFRecipes.Ping(c.Context()); err != nil {
        return c.Status(503).JSON(fiber.Map{"error": "recipes registry unhealthy", "details": err.Error()})
    }
    return c.JSON(fiber.Map{"status": "ok"})
}

// handleARFRecipesList returns a minimal list of recipes via the internal ARF facade.
func (s *Server) handleARFRecipesList(c *fiber.Ctx) error {
    if s.dependencies == nil || s.dependencies.ARFRecipes == nil {
        return c.Status(503).JSON(fiber.Map{"error": "recipes registry unavailable"})
    }
    filters := struct{
        Language string
        Tag      string
    }{
        Language: c.Query("language"),
        Tag:      c.Query("tag"),
    }
    list, err := s.dependencies.ARFRecipes.List(c.Context(), struct{
        Language string
        Tag      string
    }(filters))
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": "list failed", "details": err.Error()})
    }
    return c.JSON(fiber.Map{"recipes": list})
}
