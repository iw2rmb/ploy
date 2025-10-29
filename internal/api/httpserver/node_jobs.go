package httpserver

import (
    "github.com/gofiber/fiber/v2"
)

// JobProvider exposes node-local job snapshots.
type JobProvider interface {
    // List returns newest-first job records.
    List() []map[string]any
    // Get returns a job record by id.
    GetMap(id string) (map[string]any, bool)
}

// handleNodeJobsList serves GET /v1/node/jobs.
func (s *Server) handleNodeJobsList(c *fiber.Ctx) error {
    if s.jobs == nil {
        return c.Status(fiber.StatusOK).JSON([]any{})
    }
    return c.Status(fiber.StatusOK).JSON(s.jobs.List())
}

// handleNodeJobsDetail serves GET /v1/node/jobs/:jobID.
func (s *Server) handleNodeJobsDetail(c *fiber.Ctx) error {
    if s.jobs == nil {
        return fiber.NewError(fiber.StatusNotFound, "job not found")
    }
    id := c.Params("jobID")
    rec, ok := s.jobs.GetMap(id)
    if !ok {
        return fiber.NewError(fiber.StatusNotFound, "job not found")
    }
    return c.Status(fiber.StatusOK).JSON(rec)
}

