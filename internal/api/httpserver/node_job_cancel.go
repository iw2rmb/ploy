package httpserver

import (
    "github.com/gofiber/fiber/v2"
    "strings"
)

// handleNodeJobCancel serves POST /v1/node/jobs/:jobID/cancel.
func (s *Server) handleNodeJobCancel(c *fiber.Ctx) error {
    if s.jobCtrl == nil {
        return fiber.NewError(fiber.StatusServiceUnavailable, "job control unavailable")
    }
    id := strings.TrimSpace(c.Params("jobID"))
    if id == "" {
        return fiber.NewError(fiber.StatusBadRequest, "job id required")
    }
    switch err := s.jobCtrl.Cancel(id); err {
    case nil:
        return c.SendStatus(fiber.StatusAccepted)
    default:
        switch {
        case err == ErrJobNotFound:
            return fiber.NewError(fiber.StatusNotFound, err.Error())
        case err == ErrJobNotRunning:
            return fiber.NewError(fiber.StatusConflict, err.Error())
        default:
            return fiber.NewError(fiber.StatusBadGateway, err.Error())
        }
    }
}

