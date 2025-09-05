package domains

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/iw2rmb/ploy/api/certificates"
	"github.com/iw2rmb/ploy/api/routing"
)

// DomainHandler handles domain management API endpoints
type DomainHandler struct {
	router      *routing.TraefikRouter
	certManager *certificates.CertificateManager
}

// NewDomainHandler creates a new domain handler
func NewDomainHandler(router *routing.TraefikRouter, certManager *certificates.CertificateManager) *DomainHandler {
	return &DomainHandler{
		router:      router,
		certManager: certManager,
	}
}

// DomainRequest represents a domain registration request
type DomainRequest struct {
	Domain       string `json:"domain"`
	Certificate  string `json:"certificate,omitempty"`   // "auto", "manual", or "none"
	CertProvider string `json:"cert_provider,omitempty"` // "letsencrypt" (default)
}

// DomainResponse represents a domain API response
type DomainResponse struct {
	Status       string             `json:"status"`
	App          string             `json:"app,omitempty"`
	Domain       string             `json:"domain,omitempty"`
	Domains      []string           `json:"domains,omitempty"`
	Message      string             `json:"message,omitempty"`
	Certificate  *CertificateInfo   `json:"certificate,omitempty"`
	Certificates []*CertificateInfo `json:"certificates,omitempty"`
}

// CertificateInfo represents certificate information in API responses
type CertificateInfo struct {
	Domain    string `json:"domain"`
	Status    string `json:"status"`   // "active", "provisioning", "failed", "expired"
	Provider  string `json:"provider"` // "letsencrypt", "custom"
	IssuedAt  string `json:"issued_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	AutoRenew bool   `json:"auto_renew"`
}

// SetupDomainRoutes configures domain management API routes
func SetupDomainRoutes(app *fiber.App, handler *DomainHandler) {
	// Domain management (matches API.md spec)
	app.Post("/v1/apps/:app/domains", handler.AddDomain)
	app.Get("/v1/apps/:app/domains", handler.ListDomains)
	app.Delete("/v1/apps/:app/domains/:domain", handler.RemoveDomain)

	// Certificate management for domains (Heroku-style)
	app.Get("/v1/apps/:app/certificates", handler.ListCertificates)
	app.Get("/v1/apps/:app/certificates/:domain", handler.GetCertificate)
	app.Post("/v1/apps/:app/certificates/:domain/provision", handler.ProvisionCertificate)
	app.Delete("/v1/apps/:app/certificates/:domain", handler.RemoveCertificate)
}

// AddDomain adds a domain to an app
func (h *DomainHandler) AddDomain(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(http.StatusBadRequest).JSON(DomainResponse{
			Status:  "error",
			Message: "App name is required",
		})
	}

	var req DomainRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(DomainResponse{
			Status:  "error",
			Message: fmt.Sprintf("Invalid request body: %v", err),
		})
	}

	// Validate domain
	if req.Domain == "" {
		return c.Status(http.StatusBadRequest).JSON(DomainResponse{
			Status:  "error",
			Message: "Domain is required",
		})
	}

	// Validate domain format
	if err := validateDomain(req.Domain); err != nil {
		return c.Status(http.StatusBadRequest).JSON(DomainResponse{
			Status:  "error",
			Message: fmt.Sprintf("Invalid domain format: %v", err),
		})
	}

	// Store domain configuration in Consul KV for persistence
	if err := h.storeDomainConfig(appName, req.Domain); err != nil {
		log.Printf("Failed to store domain config for %s: %v", appName, err)
		return c.Status(http.StatusInternalServerError).JSON(DomainResponse{
			Status:  "error",
			Message: "Failed to store domain configuration",
		})
	}

	// Register app routing with Traefik if it's a platform subdomain
	if h.router != nil && h.router.IsPlatformSubdomain(req.Domain) {
		// For platform subdomains, automatically register with platform routing
		// This would typically be called when the app is deployed, but we ensure it's available for routing
		log.Printf("Platform subdomain detected (%s), routing configured for app %s", req.Domain, appName)

		// Note: Actual Traefik registration happens during app deployment via Nomad
		// This just ensures the domain-to-app mapping is stored for future routing
	}

	response := DomainResponse{
		Status:  "added",
		App:     appName,
		Domain:  req.Domain,
		Message: "Domain registered successfully",
	}

	// Handle certificate provisioning (Heroku-style)
	certificateMode := req.Certificate
	if certificateMode == "" {
		certificateMode = "auto" // Default to automatic certificate provisioning
	}

	if certificateMode == "auto" && h.certManager != nil {
		log.Printf("Auto-provisioning certificate for domain %s", req.Domain)

		// Start certificate provisioning in background
		go func() {
			ctx := context.Background()
			cert, err := h.certManager.ProvisionCertificate(ctx, appName, req.Domain)
			if err != nil {
				log.Printf("Failed to provision certificate for %s: %v", req.Domain, err)
			} else {
				log.Printf("Certificate provisioned for %s: %s", req.Domain, cert.Status)
			}
		}()

		response.Certificate = &CertificateInfo{
			Domain:    req.Domain,
			Status:    "provisioning",
			Provider:  "letsencrypt",
			AutoRenew: true,
		}
		response.Message = "Domain registered successfully, certificate provisioning started"
	}

	log.Printf("Domain registered for app %s: %s", appName, req.Domain)
	return c.JSON(response)
}

// RegisterAppPlatformDomain automatically registers platform subdomain routing for deployed apps
func (h *DomainHandler) RegisterAppPlatformDomain(appName, allocID, allocIP string, port int) error {
	if h.router == nil {
		return fmt.Errorf("traefik router not available")
	}

	// Generate platform subdomain for the app
	platformDomain := h.router.GenerateAppDomain(appName)

	// Register with Traefik using platform subdomain pattern
	if err := h.router.RegisterAppWithPlatformDomain(appName, allocID, allocIP, port, nil); err != nil {
		return fmt.Errorf("failed to register app with platform domain: %w", err)
	}

	// Store the platform domain mapping
	if err := h.storeDomainConfig(appName, platformDomain); err != nil {
		log.Printf("Warning: Failed to store platform domain config for %s: %v", appName, err)
		// Don't fail the registration if storage fails
	}

	log.Printf("App %s automatically registered with platform domain: %s", appName, platformDomain)
	return nil
}

// ListDomains lists all domains for an app
func (h *DomainHandler) ListDomains(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(http.StatusBadRequest).JSON(DomainResponse{
			Status:  "error",
			Message: "App name is required",
		})
	}

	// Get stored domain configurations and active routes
	storedDomains, err := h.getStoredDomains(appName)
	if err != nil {
		log.Printf("Failed to get stored domains for app %s: %v", appName, err)
	}

	// Also get active routes from Traefik/Consul
	routes, err := h.router.GetAppRoutes(appName)
	if err != nil {
		log.Printf("Failed to get routes for app %s: %v", appName, err)
	}

	// Combine stored domains and active routes
	domainMap := make(map[string]bool)

	// Add stored domains
	for _, domain := range storedDomains {
		domainMap[domain] = true
	}

	// Add domains from active routes
	for _, route := range routes {
		if route.Domain != "" {
			domainMap[route.Domain] = true
		}
		for _, alias := range route.Aliases {
			domainMap[alias] = true
		}
	}

	// Always include default domain
	defaultDomain := fmt.Sprintf("%s.ployd.app", appName)
	domainMap[defaultDomain] = true

	var domains []string
	var certificates []*CertificateInfo

	for domain := range domainMap {
		domains = append(domains, domain)

		// Get certificate info for each domain if certificate manager is available
		if h.certManager != nil {
			if cert, err := h.certManager.GetDomainCertificate(appName, domain); err == nil {
				certInfo := &CertificateInfo{
					Domain:    cert.Domain,
					Status:    cert.Status,
					Provider:  cert.Provider,
					AutoRenew: cert.AutoRenew,
				}
				if !cert.IssuedAt.IsZero() {
					certInfo.IssuedAt = cert.IssuedAt.Format("2006-01-02 15:04:05")
				}
				if !cert.ExpiresAt.IsZero() {
					certInfo.ExpiresAt = cert.ExpiresAt.Format("2006-01-02 15:04:05")
				}
				certificates = append(certificates, certInfo)
			}
		}
	}

	return c.JSON(DomainResponse{
		Status:       "success",
		App:          appName,
		Domains:      domains,
		Certificates: certificates,
	})
}

// RemoveDomain removes a domain from an app
func (h *DomainHandler) RemoveDomain(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")

	if appName == "" || domain == "" {
		return c.Status(http.StatusBadRequest).JSON(DomainResponse{
			Status:  "error",
			Message: "App name and domain are required",
		})
	}

	// Get current routes
	routes, err := h.router.GetAppRoutes(appName)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(DomainResponse{
			Status:  "error",
			Message: "Failed to retrieve app routes",
		})
	}

	// Remove from stored configuration
	if err := h.removeDomainConfig(appName, domain); err != nil {
		log.Printf("Failed to remove domain config for %s: %v", appName, err)
	}

	// Find routes with this domain and remove them
	for _, route := range routes {
		if route.Domain == domain || contains(route.Aliases, domain) {
			if err := h.router.UnregisterApp(appName, route.AllocID); err != nil {
				log.Printf("Failed to unregister route for %s: %v", domain, err)
			}
		}
	}

	// Remove associated certificate
	if h.certManager != nil {
		if err := h.certManager.RemoveDomainCertificate(appName, domain); err != nil {
			log.Printf("Warning: Failed to remove certificate for domain %s: %v", domain, err)
		} else {
			log.Printf("Certificate removed for domain %s", domain)
		}
	}

	log.Printf("Removed domain %s from app %s", domain, appName)

	return c.JSON(DomainResponse{
		Status:  "removed",
		App:     appName,
		Domain:  domain,
		Message: "Domain removed successfully",
	})
}

// Helper functions

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

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// storeDomainConfig stores domain configuration in Consul KV
func (h *DomainHandler) storeDomainConfig(appName, domain string) error {
	if h.router == nil {
		return fmt.Errorf("router not initialized")
	}

	// Get existing domains for this app
	domains, err := h.getStoredDomains(appName)
	if err != nil {
		domains = []string{}
	}

	// Add domain if not already present
	for _, existing := range domains {
		if existing == domain {
			return nil // Domain already exists
		}
	}

	domains = append(domains, domain)

	// Store updated domains list
	key := fmt.Sprintf("ploy/domains/%s/config", appName)
	data, err := json.Marshal(domains)
	if err != nil {
		return fmt.Errorf("failed to marshal domains: %w", err)
	}

	// Use the router's consul client to store the configuration
	pair := &consulapi.KVPair{
		Key:   key,
		Value: data,
	}

	_, err = h.router.GetConsulClient().KV().Put(pair, nil)
	if err != nil {
		return fmt.Errorf("failed to store domain config in Consul KV: %w", err)
	}

	return nil
}

// getStoredDomains retrieves stored domain configurations from Consul KV
func (h *DomainHandler) getStoredDomains(appName string) ([]string, error) {
	if h.router == nil {
		return nil, fmt.Errorf("router not initialized")
	}

	key := fmt.Sprintf("ploy/domains/%s/config", appName)

	pair, _, err := h.router.GetConsulClient().KV().Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain config: %w", err)
	}

	if pair == nil {
		return []string{}, nil
	}

	var domains []string
	if err := json.Unmarshal(pair.Value, &domains); err != nil {
		return nil, fmt.Errorf("failed to unmarshal domain config: %w", err)
	}

	return domains, nil
}

// removeDomainConfig removes a domain from stored configuration
func (h *DomainHandler) removeDomainConfig(appName, domain string) error {
	domains, err := h.getStoredDomains(appName)
	if err != nil {
		return err
	}

	// Filter out the domain to remove
	var filtered []string
	for _, d := range domains {
		if d != domain {
			filtered = append(filtered, d)
		}
	}

	// Store updated domains list
	key := fmt.Sprintf("ploy/domains/%s/config", appName)
	data, err := json.Marshal(filtered)
	if err != nil {
		return fmt.Errorf("failed to marshal domains: %w", err)
	}

	pair := &consulapi.KVPair{
		Key:   key,
		Value: data,
	}

	_, err = h.router.GetConsulClient().KV().Put(pair, nil)
	if err != nil {
		return fmt.Errorf("failed to update domain config in Consul KV: %w", err)
	}

	return nil
}

// Certificate management handlers (Heroku-style)

// ListCertificates lists all certificates for an app
func (h *DomainHandler) ListCertificates(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return c.Status(http.StatusBadRequest).JSON(DomainResponse{
			Status:  "error",
			Message: "App name is required",
		})
	}

	if h.certManager == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(DomainResponse{
			Status:  "error",
			Message: "Certificate management not available",
		})
	}

	certs, err := h.certManager.ListAppCertificates(appName)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(DomainResponse{
			Status:  "error",
			Message: fmt.Sprintf("Failed to list certificates: %v", err),
		})
	}

	var certInfos []*CertificateInfo
	for _, cert := range certs {
		certInfo := &CertificateInfo{
			Domain:    cert.Domain,
			Status:    cert.Status,
			Provider:  cert.Provider,
			AutoRenew: cert.AutoRenew,
		}
		if !cert.IssuedAt.IsZero() {
			certInfo.IssuedAt = cert.IssuedAt.Format("2006-01-02 15:04:05")
		}
		if !cert.ExpiresAt.IsZero() {
			certInfo.ExpiresAt = cert.ExpiresAt.Format("2006-01-02 15:04:05")
		}
		certInfos = append(certInfos, certInfo)
	}

	return c.JSON(DomainResponse{
		Status:       "success",
		App:          appName,
		Certificates: certInfos,
	})
}

// GetCertificate gets certificate information for a specific domain
func (h *DomainHandler) GetCertificate(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")

	if appName == "" || domain == "" {
		return c.Status(http.StatusBadRequest).JSON(DomainResponse{
			Status:  "error",
			Message: "App name and domain are required",
		})
	}

	if h.certManager == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(DomainResponse{
			Status:  "error",
			Message: "Certificate management not available",
		})
	}

	cert, err := h.certManager.GetDomainCertificate(appName, domain)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(DomainResponse{
			Status:  "error",
			Message: fmt.Sprintf("Certificate not found: %v", err),
		})
	}

	certInfo := &CertificateInfo{
		Domain:    cert.Domain,
		Status:    cert.Status,
		Provider:  cert.Provider,
		AutoRenew: cert.AutoRenew,
	}
	if !cert.IssuedAt.IsZero() {
		certInfo.IssuedAt = cert.IssuedAt.Format("2006-01-02 15:04:05")
	}
	if !cert.ExpiresAt.IsZero() {
		certInfo.ExpiresAt = cert.ExpiresAt.Format("2006-01-02 15:04:05")
	}

	return c.JSON(DomainResponse{
		Status:      "success",
		App:         appName,
		Domain:      domain,
		Certificate: certInfo,
	})
}

// ProvisionCertificate manually provisions a certificate for a domain
func (h *DomainHandler) ProvisionCertificate(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")

	if appName == "" || domain == "" {
		return c.Status(http.StatusBadRequest).JSON(DomainResponse{
			Status:  "error",
			Message: "App name and domain are required",
		})
	}

	if h.certManager == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(DomainResponse{
			Status:  "error",
			Message: "Certificate management not available",
		})
	}

	// Start certificate provisioning in background
	go func() {
		ctx := context.Background()
		cert, err := h.certManager.ProvisionCertificate(ctx, appName, domain)
		if err != nil {
			log.Printf("Failed to provision certificate for %s: %v", domain, err)
		} else {
			log.Printf("Certificate provisioned for %s: %s", domain, cert.Status)
		}
	}()

	return c.JSON(DomainResponse{
		Status:  "provisioning",
		App:     appName,
		Domain:  domain,
		Message: "Certificate provisioning started",
		Certificate: &CertificateInfo{
			Domain:    domain,
			Status:    "provisioning",
			Provider:  "letsencrypt",
			AutoRenew: true,
		},
	})
}

// RemoveCertificate removes a certificate for a domain
func (h *DomainHandler) RemoveCertificate(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")

	if appName == "" || domain == "" {
		return c.Status(http.StatusBadRequest).JSON(DomainResponse{
			Status:  "error",
			Message: "App name and domain are required",
		})
	}

	if h.certManager == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(DomainResponse{
			Status:  "error",
			Message: "Certificate management not available",
		})
	}

	if err := h.certManager.RemoveDomainCertificate(appName, domain); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(DomainResponse{
			Status:  "error",
			Message: fmt.Sprintf("Failed to remove certificate: %v", err),
		})
	}

	return c.JSON(DomainResponse{
		Status:  "removed",
		App:     appName,
		Domain:  domain,
		Message: "Certificate removed successfully",
	})
}
