package domains

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/certificates"
)

func (h *DomainHandler) ListCertificates(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return badRequest(c, "App name is required")
	}

	if h.certManager == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(DomainResponse{Status: "error", Message: "Certificate management not available"})
	}

	certs, err := h.certManager.ListAppCertificates(appName)
	if err != nil {
		log.Printf("Failed to list certificates for %s: %v", appName, err)
		return serverError(c, fmt.Sprintf("Failed to list certificates: %v", err))
	}

	infos := make([]*CertificateInfo, 0, len(certs))
	for _, cert := range certs {
		infos = append(infos, toCertInfo(cert))
	}

	return c.JSON(DomainResponse{Status: "success", App: appName, Certificates: infos})
}

func (h *DomainHandler) GetCertificate(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")
	if appName == "" || domain == "" {
		return badRequest(c, "App name and domain are required")
	}

	if h.certManager == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(DomainResponse{Status: "error", Message: "Certificate management not available"})
	}

	cert, err := h.certManager.GetDomainCertificate(appName, domain)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(DomainResponse{Status: "error", Message: fmt.Sprintf("Certificate not found: %v", err)})
	}

	return c.JSON(DomainResponse{Status: "success", App: appName, Domain: domain, Certificate: toCertInfo(cert)})
}

func (h *DomainHandler) ProvisionCertificate(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")
	if appName == "" || domain == "" {
		return badRequest(c, "App name and domain are required")
	}

	if h.certManager == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(DomainResponse{Status: "error", Message: "Certificate management not available"})
	}

	go h.startProvisioning(context.Background(), appName, domain)

	return c.JSON(DomainResponse{Status: "provisioning", App: appName, Domain: domain, Message: "Certificate provisioning started", Certificate: &CertificateInfo{
		Domain:    domain,
		Status:    "provisioning",
		Provider:  "letsencrypt",
		AutoRenew: true,
	}})
}

func (h *DomainHandler) RemoveCertificate(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")
	if appName == "" || domain == "" {
		return badRequest(c, "App name and domain are required")
	}

	if h.certManager == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(DomainResponse{Status: "error", Message: "Certificate management not available"})
	}

	if err := h.certManager.RemoveDomainCertificate(appName, domain); err != nil {
		return serverError(c, fmt.Sprintf("Failed to remove certificate: %v", err))
	}

	return c.JSON(DomainResponse{Status: "removed", App: appName, Domain: domain, Message: "Certificate removed successfully"})
}

func (h *DomainHandler) startProvisioning(ctx context.Context, appName, domain string) {
	cert, err := h.certManager.ProvisionCertificate(ctx, appName, domain)
	if err != nil {
		log.Printf("Failed to provision certificate for %s: %v", domain, err)
		return
	}
	log.Printf("Certificate provisioned for %s: %s", domain, cert.Status)
}

func toCertInfo(cert *certificates.DomainCertificate) *CertificateInfo {
	info := &CertificateInfo{
		Domain:    cert.Domain,
		Status:    cert.Status,
		Provider:  cert.Provider,
		AutoRenew: cert.AutoRenew,
	}
	if !cert.IssuedAt.IsZero() {
		info.IssuedAt = cert.IssuedAt.Format("2006-01-02 15:04:05")
	}
	if !cert.ExpiresAt.IsZero() {
		info.ExpiresAt = cert.ExpiresAt.Format("2006-01-02 15:04:05")
	}
	return info
}
