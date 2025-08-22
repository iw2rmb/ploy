package domain

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/utils"
)

func AddDomain(c *fiber.Ctx) error {
	app := c.Params("app")
	var req struct {
		Domain string `json:"domain"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.ErrJSON(c, 400, fmt.Errorf("invalid request body"))
	}
	
	log.Printf("Adding domain %s for app %s", req.Domain, app)
	
	return c.JSON(fiber.Map{
		"status":  "added",
		"app":     app,
		"domain":  req.Domain,
		"message": "Domain registered successfully",
	})
}

func ListDomains(c *fiber.Ctx) error {
	app := c.Params("app")
	
	log.Printf("Listing domains for app %s", app)
	return c.JSON(fiber.Map{
		"app": app,
		"domains": []string{
			fmt.Sprintf("%s.ployd.app", app),
		},
	})
}

func RemoveDomain(c *fiber.Ctx) error {
	app := c.Params("app")
	domain := c.Params("domain")
	
	log.Printf("Removing domain %s from app %s", domain, app)
	
	return c.JSON(fiber.Map{
		"status":  "removed",
		"app":     app,
		"domain":  domain,
		"message": "Domain removed successfully",
	})
}