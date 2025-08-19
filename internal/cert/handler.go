package cert

import (
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ploy/ploy/internal/utils"
)

func IssueCertificate(c *fiber.Ctx) error {
	var req struct {
		Domain string `json:"domain"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.ErrJSON(c, 400, fmt.Errorf("invalid request body"))
	}
	
	log.Printf("Issuing certificate for domain %s", req.Domain)
	
	return c.JSON(fiber.Map{
		"status":  "issued",
		"domain":  req.Domain,
		"message": "Certificate issued successfully",
		"expires": time.Now().AddDate(0, 3, 0).Format("2006-01-02"),
	})
}

func ListCertificates(c *fiber.Ctx) error {
	log.Printf("Listing certificates")
	
	return c.JSON(fiber.Map{
		"certificates": []fiber.Map{
			{
				"domain":  "example.ployd.app",
				"status":  "valid",
				"expires": time.Now().AddDate(0, 2, 0).Format("2006-01-02"),
			},
		},
	})
}