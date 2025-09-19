package domains

import "github.com/gofiber/fiber/v2"

// SetupDomainRoutes configures domain management API routes.
func SetupDomainRoutes(app *fiber.App, handler *DomainHandler) {
	app.Post("/v1/apps/:app/domains", handler.AddDomain)
	app.Get("/v1/apps/:app/domains", handler.ListDomains)
	app.Delete("/v1/apps/:app/domains/:domain", handler.RemoveDomain)

	app.Get("/v1/apps/:app/certificates", handler.ListCertificates)
	app.Get("/v1/apps/:app/certificates/:domain", handler.GetCertificate)
	app.Post("/v1/apps/:app/certificates/:domain/provision", handler.ProvisionCertificate)
	app.Delete("/v1/apps/:app/certificates/:domain", handler.RemoveCertificate)
}
