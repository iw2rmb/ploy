package domains

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/ploy/ploy/controller/routing"
)

// DomainHandler handles domain management API endpoints
type DomainHandler struct {
	router *routing.TraefikRouter
}

// NewDomainHandler creates a new domain handler
func NewDomainHandler(router *routing.TraefikRouter) *DomainHandler {
	return &DomainHandler{router: router}
}

// DomainRequest represents a domain registration request
type DomainRequest struct {
	Domain string `json:"domain"`
}

// DomainResponse represents a domain API response
type DomainResponse struct {
	Status  string   `json:"status"`
	App     string   `json:"app,omitempty"`
	Domain  string   `json:"domain,omitempty"`
	Domains []string `json:"domains,omitempty"`
	Message string   `json:"message,omitempty"`
}

// SetupDomainRoutes configures domain management API routes
func SetupDomainRoutes(app *fiber.App, handler *DomainHandler) {
	// Domain management (matches REST.md spec)
	app.Post("/v1/apps/:app/domains", handler.AddDomain)
	app.Get("/v1/apps/:app/domains", handler.ListDomains)
	app.Delete("/v1/apps/:app/domains/:domain", handler.RemoveDomain)
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
	
	log.Printf("Domain registered for app %s: %s", appName, req.Domain)
	
	return c.JSON(DomainResponse{
		Status:  "added",
		App:     appName,
		Domain:  req.Domain,
		Message: "Domain registered successfully",
	})
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
	for domain := range domainMap {
		domains = append(domains, domain)
	}
	
	return c.JSON(DomainResponse{
		Status:  "success",
		App:     appName,
		Domains: domains,
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