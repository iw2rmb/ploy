package domains

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func badRequest(c *fiber.Ctx, message string) error {
	return c.Status(http.StatusBadRequest).JSON(DomainResponse{Status: "error", Message: message})
}

func serverError(c *fiber.Ctx, message string) error {
	return c.Status(http.StatusInternalServerError).JSON(DomainResponse{Status: "error", Message: message})
}

func validateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	if strings.Contains(domain, " ") {
		return fmt.Errorf("domain cannot contain spaces")
	}
	if !strings.Contains(domain, ".") {
		return fmt.Errorf("domain must contain at least one dot")
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return fmt.Errorf("domain cannot start or end with a dot")
	}
	if len(domain) > 253 {
		return fmt.Errorf("domain too long (max 253 characters)")
	}
	return nil
}

func contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
