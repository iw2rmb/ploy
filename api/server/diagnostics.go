package server

import (
	"github.com/gofiber/fiber/v2"
)

// handleDiagEcho reads the request body and echoes metadata for debugging ingress/proxy issues
func (s *Server) handleDiagEcho(c *fiber.Ctx) error {
	body := c.Body()
	headers := c.GetReqHeaders()
	return c.Status(200).JSON(fiber.Map{
		"ok":           true,
		"body_length":  len(body),
		"content_type": c.Get("Content-Type"),
		"content_len":  c.Get("Content-Length"),
		"headers":      headers,
	})
}
