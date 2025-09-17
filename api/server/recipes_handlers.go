package server

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// handleRecipeCatalogPing exercises the internal recipe catalog facade.
func (s *Server) handleRecipeCatalogPing(c *fiber.Ctx) error {
	if s.dependencies == nil || s.dependencies.RecipeCatalog == nil {
		return c.Status(503).JSON(fiber.Map{"error": "recipes registry unavailable"})
	}
	if err := s.dependencies.RecipeCatalog.Ping(c.Context()); err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "recipes registry unhealthy", "details": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

// handleRecipeCatalogList returns a minimal list of recipes via the internal catalog facade.
func (s *Server) handleRecipeCatalogList(c *fiber.Ctx) error {
	if s.dependencies == nil || s.dependencies.RecipeCatalog == nil {
		return c.Status(503).JSON(fiber.Map{"error": "recipes registry unavailable"})
	}
	filters := struct {
		Language string
		Tag      string
	}{
		Language: c.Query("language"),
		Tag:      c.Query("tag"),
	}
	list, err := s.dependencies.RecipeCatalog.List(c.Context(), struct {
		Language string
		Tag      string
	}(filters))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "list failed", "details": err.Error()})
	}
	return c.JSON(fiber.Map{"recipes": list})
}

// handleRecipeCatalogGet returns a single recipe by ID via the internal catalog facade.
func (s *Server) handleRecipeCatalogGet(c *fiber.Ctx) error {
	if s.dependencies == nil || s.dependencies.RecipeCatalog == nil {
		return c.Status(503).JSON(fiber.Map{"error": "recipes registry unavailable"})
	}
	id := c.Params("id")
	rec, err := s.dependencies.RecipeCatalog.Get(c.Context(), id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "get failed", "details": err.Error()})
	}
	if rec == nil {
		return c.Status(404).JSON(fiber.Map{"error": "recipe not found"})
	}
	return c.JSON(fiber.Map{"recipe": rec})
}

// handleRecipeCatalogSearch performs a simple substring search over ID, name, and tags.
func (s *Server) handleRecipeCatalogSearch(c *fiber.Ctx) error {
	if s.dependencies == nil || s.dependencies.RecipeCatalog == nil {
		return c.Status(503).JSON(fiber.Map{"error": "recipes registry unavailable"})
	}
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		return c.Status(400).JSON(fiber.Map{"error": "query parameter 'q' is required"})
	}
	// List all and filter client-side (registry may optimize later)
	list, err := s.dependencies.RecipeCatalog.List(c.Context(), struct {
		Language string
		Tag      string
	}{})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "list failed", "details": err.Error()})
	}
	ql := strings.ToLower(q)
	filtered := make([]interface{}, 0, len(list))
	for _, r := range list {
		if strings.Contains(strings.ToLower(r.Name), ql) || strings.Contains(strings.ToLower(r.ID), ql) {
			filtered = append(filtered, r)
			continue
		}
		hit := false
		for _, t := range r.Tags {
			if strings.Contains(strings.ToLower(t), ql) {
				hit = true
				break
			}
		}
		if hit {
			filtered = append(filtered, r)
		}
	}
	return c.JSON(fiber.Map{"recipes": filtered})
}
