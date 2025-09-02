package cert

import (
	"log"

	"github.com/gofiber/fiber/v2"
)

// Legacy handlers - deprecated in favor of /v1/certs ACME endpoints
// These are kept for backward compatibility

func IssueCertificate(c *fiber.Ctx) error {
	log.Printf("WARNING: Using deprecated certificate endpoint. Please use /v1/certs/issue instead")

	return c.JSON(fiber.Map{
		"status":        "deprecated",
		"message":       "This endpoint is deprecated. Please use /v1/certs/issue for ACME certificate management",
		"new_endpoint":  "/v1/certs/issue",
		"documentation": "See API.md for complete ACME certificate management endpoints",
	})
}

func ListCertificates(c *fiber.Ctx) error {
	log.Printf("WARNING: Using deprecated certificate endpoint. Please use /v1/certs instead")

	return c.JSON(fiber.Map{
		"status":        "deprecated",
		"message":       "This endpoint is deprecated. Please use /v1/certs for ACME certificate management",
		"new_endpoint":  "/v1/certs",
		"documentation": "See API.md for complete ACME certificate management endpoints",
	})
}
