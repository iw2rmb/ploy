package server

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

// handleListAppCertificates lists all certificates for an app
func (s *Server) handleListAppCertificates(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name is required"})
	}

	if s.dependencies.CertificateManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Certificate management not available"})
	}

	certificates, err := s.dependencies.CertificateManager.ListAppCertificates(appName)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to list certificates: %v", err)})
	}

	return c.JSON(fiber.Map{
		"status":       "success",
		"app":          appName,
		"certificates": certificates,
		"count":        len(certificates),
	})
}

// handleGetDomainCertificate gets certificate info for a domain
func (s *Server) handleGetDomainCertificate(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")

	if appName == "" || domain == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name and domain are required"})
	}

	if s.dependencies.CertificateManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Certificate management not available"})
	}

	certificate, err := s.dependencies.CertificateManager.GetDomainCertificate(appName, domain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": fmt.Sprintf("Certificate not found: %v", err)})
	}

	return c.JSON(certificate)
}

// handleProvisionCertificate manually provisions a certificate for a domain
func (s *Server) handleProvisionCertificate(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")

	if appName == "" || domain == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name and domain are required"})
	}

	if s.dependencies.CertificateManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Certificate management not available"})
	}

	ctx := context.Background()
	certificate, err := s.dependencies.CertificateManager.ProvisionCertificate(ctx, appName, domain)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to provision certificate: %v", err)})
	}

	return c.JSON(fiber.Map{
		"status":      "provisioned",
		"app":         appName,
		"domain":      domain,
		"certificate": certificate,
	})
}

// handleRemoveCertificate removes a certificate for a domain
func (s *Server) handleRemoveCertificate(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")

	if appName == "" || domain == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name and domain are required"})
	}

	if s.dependencies.CertificateManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Certificate management not available"})
	}

	err := s.dependencies.CertificateManager.RemoveDomainCertificate(appName, domain)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to remove certificate: %v", err)})
	}

	return c.JSON(fiber.Map{
		"status": "removed",
		"app":    appName,
		"domain": domain,
	})
}

// handleUploadCertificate handles uploading custom certificate bundles
func (s *Server) handleUploadCertificate(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")

	if appName == "" || domain == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App name and domain are required"})
	}

	if s.dependencies.CertificateManager == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Certificate management not available"})
	}

	// Parse multipart form
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Failed to parse multipart form: %v", err)})
	}

	// Get certificate data
	certFiles := form.Value["certificate"]
	if len(certFiles) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Certificate is required"})
	}
	certificate := []byte(certFiles[0])

	// Get private key data
	keyFiles := form.Value["private_key"]
	if len(keyFiles) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Private key is required"})
	}
	privateKey := []byte(keyFiles[0])

	// Get CA certificate data (optional)
	var caCert []byte
	caFiles := form.Value["ca_certificate"]
	if len(caFiles) > 0 {
		caCert = []byte(caFiles[0])
	}

	// Create certificate record
	ctx := context.Background()
	domainCert, err := s.dependencies.CertificateManager.UploadCustomCertificate(ctx, appName, domain, certificate, privateKey, caCert)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to upload certificate: %v", err)})
	}

	return c.JSON(fiber.Map{
		"status":      "uploaded",
		"app":         appName,
		"domain":      domain,
		"certificate": domainCert,
		"message":     "Custom certificate uploaded successfully",
	})
}

// handlePlatformCertificateHealth handles platform wildcard certificate health checks
func (s *Server) handlePlatformCertificateHealth(c *fiber.Ctx) error {
	if s.dependencies.PlatformWildcardManager == nil || !s.dependencies.PlatformWildcardManager.IsEnabled() {
		return c.JSON(fiber.Map{
			"status":  "disabled",
			"message": "Platform wildcard certificate management disabled (PLOY_APPS_DOMAIN not set)",
		})
	}

	ctx := context.Background()
	cert, err := s.dependencies.PlatformWildcardManager.GetPlatformWildcardCertificate(ctx)
	if err != nil {
		return c.Status(503).JSON(fiber.Map{
			"status": "error",
			"error":  err.Error(),
			"domain": s.dependencies.PlatformWildcardManager.GetWildcardDomain(),
		})
	}

	daysUntilExpiry := int(time.Until(cert.ExpiresAt).Hours() / 24)

	// Determine health status based on expiry
	status := "healthy"
	if daysUntilExpiry <= 7 {
		status = "expiring_soon"
	} else if daysUntilExpiry <= 1 {
		status = "critical"
	}

	return c.JSON(fiber.Map{
		"status":             status,
		"platform_domain":    s.dependencies.PlatformWildcardManager.GetPlatformDomain(),
		"wildcard_domain":    cert.Domain,
		"expires_at":         cert.ExpiresAt,
		"days_until_expiry":  daysUntilExpiry,
		"issued_at":          cert.IssuedAt,
		"auto_renew_enabled": true,
	})
}
