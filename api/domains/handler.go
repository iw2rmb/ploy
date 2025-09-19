package domains

import (
	"context"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/certificates"
	"github.com/iw2rmb/ploy/api/routing"
)

// DomainHandler handles domain management API endpoints.
type DomainHandler struct {
	router      *routing.TraefikRouter
	certManager *certificates.CertificateManager
}

// NewDomainHandler creates a new domain handler.
func NewDomainHandler(router *routing.TraefikRouter, certManager *certificates.CertificateManager) *DomainHandler {
	return &DomainHandler{router: router, certManager: certManager}
}

// AddDomain registers a new domain for the given app.
func (h *DomainHandler) AddDomain(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return badRequest(c, "App name is required")
	}

	var req DomainRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, fmt.Sprintf("Invalid request body: %v", err))
	}

	if req.Domain == "" {
		return badRequest(c, "Domain is required")
	}
	if err := validateDomain(req.Domain); err != nil {
		return badRequest(c, fmt.Sprintf("Invalid domain format: %v", err))
	}

	if err := h.storeDomainConfig(appName, req.Domain); err != nil {
		log.Printf("Failed to store domain config for %s: %v", appName, err)
		return serverError(c, "Failed to store domain configuration")
	}

	if h.router != nil && h.router.IsPlatformSubdomain(req.Domain) {
		log.Printf("Platform subdomain detected (%s), routing configured for app %s", req.Domain, appName)
	}

	response := DomainResponse{Status: "added", App: appName, Domain: req.Domain, Message: "Domain registered successfully"}

	mode := req.Certificate
	if mode == "" {
		mode = "auto"
	}
	if mode == "auto" && h.certManager != nil {
		log.Printf("Auto-provisioning certificate for domain %s", req.Domain)
		go h.startProvisioning(context.Background(), appName, req.Domain)
		response.Certificate = &CertificateInfo{Domain: req.Domain, Status: "provisioning", Provider: "letsencrypt", AutoRenew: true}
		response.Message = "Domain registered successfully, certificate provisioning started"
	}

	log.Printf("Domain registered for app %s: %s", appName, req.Domain)
	return c.JSON(response)
}

// RegisterAppPlatformDomain registers the platform subdomain for a deployed app.
func (h *DomainHandler) RegisterAppPlatformDomain(appName, allocID, allocIP string, port int) error {
	if h.router == nil {
		return fmt.Errorf("traefik router not available")
	}

	platformDomain := h.router.GenerateAppDomain(appName)
	if err := h.router.RegisterAppWithPlatformDomain(appName, allocID, allocIP, port, nil); err != nil {
		return fmt.Errorf("failed to register app with platform domain: %w", err)
	}

	if err := h.storeDomainConfig(appName, platformDomain); err != nil {
		log.Printf("Warning: failed to store platform domain config for %s: %v", appName, err)
	}

	log.Printf("App %s automatically registered with platform domain: %s", appName, platformDomain)
	return nil
}
