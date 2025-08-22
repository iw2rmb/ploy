package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gofiber/fiber/v2"
	consulapi "github.com/hashicorp/consul/api"
)

// Handler handles DNS management operations
type Handler struct {
	manager      *Manager
	provider     Provider
	consulClient *consulapi.Client
}

// DNSConfig represents the DNS configuration from environment or file
type DNSConfig struct {
	Provider       string            `json:"provider"`       // cloudflare, namecheap, route53, etc.
	Domain         string            `json:"domain"`         // Base domain (e.g., "ployd.app")
	TargetIP       string            `json:"target_ip"`      // Target IP for wildcard
	TargetCNAME    string            `json:"target_cname"`   // Alternative target CNAME
	TTL            int               `json:"ttl"`
	EnableIPv6     bool              `json:"enable_ipv6"`
	IPv6Target     string            `json:"ipv6_target"`
	LoadBalancerIPs []string         `json:"load_balancer_ips,omitempty"`
	
	// Provider-specific configuration
	Cloudflare     *CloudflareConfig `json:"cloudflare,omitempty"`
	Namecheap      *NamecheapConfig  `json:"namecheap,omitempty"`
	// Add other providers as needed
}

// SetupDNSRoutes configures DNS management API routes
func SetupDNSRoutes(app *fiber.App, handler *Handler) {
	api := app.Group("/v1/dns")
	
	// Wildcard DNS management
	api.Post("/wildcard/setup", handler.SetupWildcard)
	api.Delete("/wildcard", handler.RemoveWildcard)
	api.Get("/wildcard/validate", handler.ValidateWildcard)
	
	// DNS records management
	api.Get("/records", handler.ListRecords)
	api.Post("/records", handler.CreateRecord)
	api.Put("/records", handler.UpdateRecord)
	api.Delete("/records/:hostname/:type", handler.DeleteRecord)
	
	// DNS configuration and status
	api.Get("/status", handler.GetStatus)
	api.Get("/config", handler.GetConfig)
	api.Post("/config/validate", handler.ValidateConfig)
}

// NewHandler creates a new DNS handler
func NewHandler(consulAddr string) (*Handler, error) {
	// Load DNS configuration
	config, err := LoadDNSConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load DNS configuration: %w", err)
	}
	
	// Create provider based on configuration
	provider, err := createProvider(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create DNS provider: %w", err)
	}
	
	// Create wildcard configuration
	wildcardConfig := &WildcardConfig{
		Domain:       config.Domain,
		TargetIP:     config.TargetIP,
		TargetCNAME:  config.TargetCNAME,
		TTL:          config.TTL,
		EnableIPv6:   config.EnableIPv6,
		IPv6Target:   config.IPv6Target,
		LoadBalancer: config.LoadBalancerIPs,
	}
	
	// Create DNS manager
	manager := NewManager(provider, wildcardConfig)
	
	// Initialize Consul client for storing DNS configurations
	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = consulAddr
	consulClient, err := consulapi.NewClient(consulConfig)
	if err != nil {
		log.Printf("Warning: Failed to initialize Consul client: %v", err)
	}
	
	return &Handler{
		manager:      manager,
		provider:     provider,
		consulClient: consulClient,
	}, nil
}

// SetupWildcard sets up wildcard DNS configuration
func (h *Handler) SetupWildcard(c *fiber.Ctx) error {
	ctx := context.Background()
	
	// Parse request body for optional overrides
	var req struct {
		TargetIP    string   `json:"target_ip,omitempty"`
		TargetCNAME string   `json:"target_cname,omitempty"`
		TTL         int      `json:"ttl,omitempty"`
		LoadBalancer []string `json:"load_balancer,omitempty"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		// Use defaults from configuration
		log.Printf("Using default configuration for wildcard setup")
	} else {
		// Override configuration if provided
		if req.TargetIP != "" {
			h.manager.config.TargetIP = req.TargetIP
		}
		if req.TargetCNAME != "" {
			h.manager.config.TargetCNAME = req.TargetCNAME
		}
		if req.TTL > 0 {
			h.manager.config.TTL = req.TTL
		}
		if len(req.LoadBalancer) > 0 {
			h.manager.config.LoadBalancer = req.LoadBalancer
		}
	}
	
	// Setup wildcard DNS
	if err := h.manager.SetupWildcardDNS(ctx); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to setup wildcard DNS: %v", err),
		})
	}
	
	// Store configuration in Consul
	if h.consulClient != nil {
		h.storeDNSConfig()
	}
	
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": fmt.Sprintf("Wildcard DNS configured for *.%s", h.manager.config.Domain),
		"config": fiber.Map{
			"domain":      h.manager.config.Domain,
			"target_ip":   h.manager.config.TargetIP,
			"target_cname": h.manager.config.TargetCNAME,
			"ttl":         h.manager.config.TTL,
		},
	})
}

// RemoveWildcard removes wildcard DNS configuration
func (h *Handler) RemoveWildcard(c *fiber.Ctx) error {
	ctx := context.Background()
	
	if err := h.manager.RemoveWildcardDNS(ctx); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to remove wildcard DNS: %v", err),
		})
	}
	
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": fmt.Sprintf("Wildcard DNS removed for *.%s", h.manager.config.Domain),
	})
}

// ValidateWildcard validates wildcard DNS configuration
func (h *Handler) ValidateWildcard(c *fiber.Ctx) error {
	ctx := context.Background()
	
	if err := h.manager.ValidateWildcardDNS(ctx); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"status": "invalid",
			"error":  err.Error(),
		})
	}
	
	return c.JSON(fiber.Map{
		"status":  "valid",
		"message": fmt.Sprintf("Wildcard DNS is properly configured for *.%s", h.manager.config.Domain),
	})
}

// ListRecords lists DNS records
func (h *Handler) ListRecords(c *fiber.Ctx) error {
	ctx := context.Background()
	domain := c.Query("domain", h.manager.config.Domain)
	
	records, err := h.provider.ListRecords(ctx, domain)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to list records: %v", err),
		})
	}
	
	return c.JSON(fiber.Map{
		"domain":  domain,
		"records": records,
		"count":   len(records),
	})
}

// CreateRecord creates a new DNS record
func (h *Handler) CreateRecord(c *fiber.Ctx) error {
	ctx := context.Background()
	
	var record Record
	if err := c.BodyParser(&record); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}
	
	if err := h.provider.CreateRecord(ctx, record); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to create record: %v", err),
		})
	}
	
	return c.JSON(fiber.Map{
		"status":  "created",
		"record":  record,
	})
}

// UpdateRecord updates an existing DNS record
func (h *Handler) UpdateRecord(c *fiber.Ctx) error {
	ctx := context.Background()
	
	var record Record
	if err := c.BodyParser(&record); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}
	
	if err := h.provider.UpdateRecord(ctx, record); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to update record: %v", err),
		})
	}
	
	return c.JSON(fiber.Map{
		"status":  "updated",
		"record":  record,
	})
}

// DeleteRecord deletes a DNS record
func (h *Handler) DeleteRecord(c *fiber.Ctx) error {
	ctx := context.Background()
	
	hostname := c.Params("hostname")
	recordType := c.Params("type")
	
	if hostname == "" || recordType == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Hostname and type are required",
		})
	}
	
	if err := h.provider.DeleteRecord(ctx, hostname, recordType); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to delete record: %v", err),
		})
	}
	
	return c.JSON(fiber.Map{
		"status":   "deleted",
		"hostname": hostname,
		"type":     recordType,
	})
}

// GetConfig returns the current DNS configuration
func (h *Handler) GetConfig(c *fiber.Ctx) error {
	config := fiber.Map{
		"domain":        h.manager.config.Domain,
		"target_ip":     h.manager.config.TargetIP,
		"target_cname":  h.manager.config.TargetCNAME,
		"ttl":          h.manager.config.TTL,
		"enable_ipv6":  h.manager.config.EnableIPv6,
		"ipv6_target":  h.manager.config.IPv6Target,
		"load_balancer": h.manager.config.LoadBalancer,
	}
	
	return c.JSON(config)
}

// ValidateConfig validates the DNS provider configuration
func (h *Handler) ValidateConfig(c *fiber.Ctx) error {
	if err := h.provider.ValidateConfiguration(); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"status": "invalid",
			"error":  err.Error(),
		})
	}
	
	return c.JSON(fiber.Map{
		"status":  "valid",
		"message": "DNS provider configuration is valid",
	})
}

// GetStatus returns the DNS system status
func (h *Handler) GetStatus(c *fiber.Ctx) error {
	status := fiber.Map{
		"dns_provider": "available",
		"provider_type": "",
		"domain": h.manager.config.Domain,
		"target_ip": h.manager.config.TargetIP,
		"configuration": "loaded",
	}
	
	// Determine provider type based on configuration
	if h.manager.config.Domain != "" {
		switch {
		case os.Getenv("PLOY_DNS_PROVIDER") == "namecheap":
			status["provider_type"] = "namecheap"
		case os.Getenv("PLOY_DNS_PROVIDER") == "cloudflare":
			status["provider_type"] = "cloudflare"
		default:
			status["provider_type"] = "unknown"
		}
	}
	
	// Validate configuration
	if err := h.provider.ValidateConfiguration(); err != nil {
		status["dns_provider"] = "error"
		status["configuration"] = "invalid"
		status["error"] = err.Error()
		return c.Status(http.StatusServiceUnavailable).JSON(status)
	}
	
	return c.JSON(status)
}

// LoadDNSConfig loads DNS configuration from environment or file
func LoadDNSConfig() (*DNSConfig, error) {
	// Try to load from environment variables first
	if provider := os.Getenv("PLOY_DNS_PROVIDER"); provider != "" {
		config := &DNSConfig{
			Provider:    provider,
			Domain:      os.Getenv("PLOY_DNS_DOMAIN"),
			TargetIP:    os.Getenv("PLOY_DNS_TARGET_IP"),
			TargetCNAME: os.Getenv("PLOY_DNS_TARGET_CNAME"),
			TTL:         300, // Default TTL
		}
		
		// Load provider-specific configuration
		if provider == "cloudflare" {
			config.Cloudflare = &CloudflareConfig{
				APIToken: os.Getenv("CLOUDFLARE_API_TOKEN"),
				ZoneID:   os.Getenv("CLOUDFLARE_ZONE_ID"),
				Email:    os.Getenv("CLOUDFLARE_EMAIL"),
				APIKey:   os.Getenv("CLOUDFLARE_API_KEY"),
			}
		} else if provider == "namecheap" {
			// Use appropriate API key based on sandbox setting
			sandbox := os.Getenv("NAMECHEAP_SANDBOX") == "true"
			apiKey := os.Getenv("NAMECHEAP_API_KEY")
			if sandbox && os.Getenv("NAMECHEAP_SANDBOX_API_KEY") != "" {
				apiKey = os.Getenv("NAMECHEAP_SANDBOX_API_KEY")
			}
			
			config.Namecheap = &NamecheapConfig{
				APIUser:  os.Getenv("NAMECHEAP_API_USER"),
				APIKey:   apiKey,
				Username: os.Getenv("NAMECHEAP_USERNAME"),
				ClientIP: os.Getenv("NAMECHEAP_CLIENT_IP"),
				Sandbox:  sandbox,
			}
		}
		
		return config, nil
	}
	
	// Try to load from configuration file
	configPath := os.Getenv("PLOY_DNS_CONFIG_PATH")
	if configPath == "" {
		configPath = "/etc/ploy/dns/config.json"
	}
	
	data, err := os.ReadFile(configPath)
	if err != nil {
		// Return default configuration for development
		return &DNSConfig{
			Provider:    "cloudflare",
			Domain:      "ployd.app",
			TargetIP:    "127.0.0.1",
			TTL:         300,
		}, nil
	}
	
	var config DNSConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse DNS configuration: %w", err)
	}
	
	return &config, nil
}

// createProvider creates a DNS provider based on configuration
func createProvider(config *DNSConfig) (Provider, error) {
	switch config.Provider {
	case "cloudflare":
		if config.Cloudflare == nil {
			return nil, fmt.Errorf("cloudflare configuration is required")
		}
		return NewCloudflareProvider(*config.Cloudflare)
	case "namecheap":
		if config.Namecheap == nil {
			return nil, fmt.Errorf("namecheap configuration is required")
		}
		return NewNamecheapProvider(*config.Namecheap)
	// Add other providers here
	// case "route53":
	//     return NewRoute53Provider(config.Route53)
	default:
		return nil, fmt.Errorf("unsupported DNS provider: %s", config.Provider)
	}
}

// storeDNSConfig stores DNS configuration in Consul
func (h *Handler) storeDNSConfig() error {
	if h.consulClient == nil {
		return nil
	}
	
	kv := h.consulClient.KV()
	
	configData, err := json.Marshal(h.manager.config)
	if err != nil {
		return fmt.Errorf("failed to marshal DNS config: %w", err)
	}
	
	kvPair := &consulapi.KVPair{
		Key:   "ploy/dns/wildcard-config",
		Value: configData,
	}
	
	_, err = kv.Put(kvPair, nil)
	if err != nil {
		return fmt.Errorf("failed to store DNS config in Consul: %w", err)
	}
	
	log.Printf("Stored DNS configuration in Consul")
	return nil
}