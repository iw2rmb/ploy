package domains

import (
	"fmt"
	"log"
	"sort"

	"github.com/gofiber/fiber/v2"
)

func (h *DomainHandler) ListDomains(c *fiber.Ctx) error {
	appName := c.Params("app")
	if appName == "" {
		return badRequest(c, "App name is required")
	}

	storedDomains, err := h.getStoredDomains(appName)
	if err != nil {
		log.Printf("Failed to get stored domains for app %s: %v", appName, err)
	}

	routes, err := h.router.GetAppRoutes(appName)
	if err != nil {
		log.Printf("Failed to get routes for app %s: %v", appName, err)
	}

	domainMap := make(map[string]bool)
	for _, domain := range storedDomains {
		domainMap[domain] = true
	}
	for _, route := range routes {
		if route.Domain != "" {
			domainMap[route.Domain] = true
		}
		for _, alias := range route.Aliases {
			domainMap[alias] = true
		}
	}

	defaultDomain := fmt.Sprintf("%s.%s", appName, h.router.GetPlatformAppsDomain())
	domainMap[defaultDomain] = true

	domains := make([]string, 0, len(domainMap))
	certInfos := make([]*CertificateInfo, 0)

	for domain := range domainMap {
		domains = append(domains, domain)
		if h.certManager == nil {
			continue
		}
		if cert, err := h.certManager.GetDomainCertificate(appName, domain); err == nil {
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
			certInfos = append(certInfos, info)
		}
	}

	sort.Strings(domains)

	return c.JSON(DomainResponse{Status: "success", App: appName, Domains: domains, Certificates: certInfos})
}

func (h *DomainHandler) RemoveDomain(c *fiber.Ctx) error {
	appName := c.Params("app")
	domain := c.Params("domain")
	if appName == "" || domain == "" {
		return badRequest(c, "App name and domain are required")
	}

	routes, err := h.router.GetAppRoutes(appName)
	if err != nil {
		return serverError(c, "Failed to retrieve app routes")
	}

	if err := h.removeDomainConfig(appName, domain); err != nil {
		log.Printf("Failed to remove domain config for %s: %v", appName, err)
	}

	for _, route := range routes {
		if route.Domain == domain || contains(route.Aliases, domain) {
			if err := h.router.UnregisterApp(appName, route.AllocID); err != nil {
				log.Printf("Failed to unregister route for %s: %v", domain, err)
			}
		}
	}

	if h.certManager != nil {
		if err := h.certManager.RemoveDomainCertificate(appName, domain); err != nil {
			log.Printf("Warning: failed to remove certificate for domain %s: %v", domain, err)
		}
	}

	log.Printf("Removed domain %s from app %s", domain, appName)
	return c.JSON(DomainResponse{Status: "removed", App: appName, Domain: domain, Message: "Domain removed successfully"})
}
