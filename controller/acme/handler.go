package acme

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/iw2rmb/ploy/controller/dns"
	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/iw2rmb/ploy/internal/utils"
)

// Handler handles ACME certificate management operations
type Handler struct {
	client         *Client
	storage        *CertificateStorage
	renewalService *RenewalService
	dnsProvider    dns.Provider
	config         *ACMEConfig
}

// ACMEConfig represents ACME configuration
type ACMEConfig struct {
	Email              string        `json:"email"`
	Staging            bool          `json:"staging"`
	DefaultDomain      string        `json:"default_domain"`
	AutoRenew          bool          `json:"auto_renew"`
	RenewalThreshold   time.Duration `json:"renewal_threshold"`
	NotificationWebhook string       `json:"notification_webhook"`
}

// NewHandler creates a new ACME handler
func NewHandler(consulClient *consulapi.Client, storageClient storage.StorageProvider, dnsProvider dns.Provider) (*Handler, error) {
	// Load configuration
	config, err := loadACMEConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load ACME configuration: %w", err)
	}

	// Create ACME client
	client, err := NewClient(config.Email, dnsProvider, config.Staging)
	if err != nil {
		return nil, fmt.Errorf("failed to create ACME client: %w", err)
	}

	// Create certificate storage
	certStorage := NewCertificateStorage(consulClient, storageClient)

	// Create renewal service
	renewalConfig := DefaultRenewalConfig()
	if config.RenewalThreshold > 0 {
		renewalConfig.RenewalThreshold = config.RenewalThreshold
	}
	renewalService := NewRenewalService(client, certStorage, dnsProvider, renewalConfig)

	handler := &Handler{
		client:         client,
		storage:        certStorage,
		renewalService: renewalService,
		dnsProvider:    dnsProvider,
		config:         config,
	}

	// Start renewal service if auto-renew is enabled
	if config.AutoRenew {
		ctx := context.Background()
		if err := renewalService.Start(ctx); err != nil {
			log.Printf("Warning: failed to start renewal service: %v", err)
		}
	}

	return handler, nil
}

// SetupACMERoutes configures ACME certificate management API routes
func SetupACMERoutes(app *fiber.App, handler *Handler) {
	api := app.Group("/v1/certs")
	
	// Certificate issuance
	api.Post("/issue", handler.IssueCertificate)
	api.Post("/issue/wildcard", handler.IssueWildcardCertificate)
	
	// Certificate management
	api.Get("/", handler.ListCertificates)
	api.Get("/:domain", handler.GetCertificate)
	api.Delete("/:domain", handler.DeleteCertificate)
	
	// Certificate renewal
	api.Post("/renew/:domain", handler.RenewCertificate)
	api.Post("/renew/all", handler.RenewAllCertificates)
	api.Post("/renew/check", handler.CheckRenewal)
	
	// Renewal service management
	api.Get("/renewal/status", handler.GetRenewalStatus)
	api.Post("/renewal/start", handler.StartRenewalService)
	api.Post("/renewal/stop", handler.StopRenewalService)
	api.Get("/renewal/stats", handler.GetRenewalStats)
	
	// Configuration
	api.Get("/config", handler.GetConfig)
	api.Post("/config", handler.UpdateConfig)
}

// IssueCertificate issues a certificate for specified domains
func (h *Handler) IssueCertificate(c *fiber.Ctx) error {
	var req struct {
		Domains []string `json:"domains"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.ErrJSON(c, 400, fmt.Errorf("invalid request body"))
	}

	if len(req.Domains) == 0 {
		return utils.ErrJSON(c, 400, fmt.Errorf("at least one domain is required"))
	}

	log.Printf("Issuing certificate for domains: %v", req.Domains)

	ctx := context.Background()
	cert, err := h.client.IssueCertificate(ctx, req.Domains)
	if err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to issue certificate: %w", err))
	}

	// Store certificate
	if err := h.storage.StoreCertificate(ctx, cert); err != nil {
		log.Printf("Warning: failed to store certificate: %v", err)
	}

	return c.JSON(fiber.Map{
		"status":     "issued",
		"domain":     req.Domains[0],
		"domains":    req.Domains,
		"expires":    cert.ExpiresAt.Format("2006-01-02"),
		"is_wildcard": cert.IsWildcard,
		"message":    "Certificate issued successfully",
	})
}

// IssueWildcardCertificate issues a wildcard certificate for a domain
func (h *Handler) IssueWildcardCertificate(c *fiber.Ctx) error {
	var req struct {
		Domain string `json:"domain"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.ErrJSON(c, 400, fmt.Errorf("invalid request body"))
	}

	if req.Domain == "" {
		return utils.ErrJSON(c, 400, fmt.Errorf("domain is required"))
	}

	log.Printf("Issuing wildcard certificate for domain: %s", req.Domain)

	ctx := context.Background()
	cert, err := h.client.IssueWildcardCertificate(ctx, req.Domain)
	if err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to issue wildcard certificate: %w", err))
	}

	// Store certificate
	if err := h.storage.StoreCertificate(ctx, cert); err != nil {
		log.Printf("Warning: failed to store certificate: %v", err)
	}

	return c.JSON(fiber.Map{
		"status":     "issued",
		"domain":     req.Domain,
		"wildcard":   fmt.Sprintf("*.%s", req.Domain),
		"expires":    cert.ExpiresAt.Format("2006-01-02"),
		"is_wildcard": true,
		"message":    "Wildcard certificate issued successfully",
	})
}

// ListCertificates lists all managed certificates
func (h *Handler) ListCertificates(c *fiber.Ctx) error {
	ctx := context.Background()
	certificates, err := h.storage.ListCertificates(ctx)
	if err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to list certificates: %w", err))
	}

	var certList []fiber.Map
	for _, cert := range certificates {
		status := "valid"
		if time.Now().After(cert.ExpiresAt) {
			status = "expired"
		} else if time.Until(cert.ExpiresAt) < 30*24*time.Hour {
			status = "expiring_soon"
		}

		certList = append(certList, fiber.Map{
			"domain":        cert.Domain,
			"status":        status,
			"expires":       cert.ExpiresAt.Format("2006-01-02"),
			"issued":        cert.IssuedAt.Format("2006-01-02"),
			"is_wildcard":   cert.IsWildcard,
			"auto_renew":    cert.AutoRenew,
			"renewal_count": cert.RenewalCount,
		})
	}

	return c.JSON(fiber.Map{
		"certificates": certList,
		"count":        len(certList),
	})
}

// GetCertificate gets information about a specific certificate
func (h *Handler) GetCertificate(c *fiber.Ctx) error {
	domain := c.Params("domain")
	if domain == "" {
		return utils.ErrJSON(c, 400, fmt.Errorf("domain parameter is required"))
	}

	ctx := context.Background()
	_, metadata, err := h.storage.GetCertificate(ctx, domain)
	if err != nil {
		return utils.ErrJSON(c, 404, fmt.Errorf("certificate not found: %w", err))
	}

	status := "valid"
	if time.Now().After(metadata.ExpiresAt) {
		status = "expired"
	} else if time.Until(metadata.ExpiresAt) < 30*24*time.Hour {
		status = "expiring_soon"
	}

	return c.JSON(fiber.Map{
		"domain":        metadata.Domain,
		"status":        status,
		"expires":       metadata.ExpiresAt.Format("2006-01-02 15:04:05"),
		"issued":        metadata.IssuedAt.Format("2006-01-02 15:04:05"),
		"is_wildcard":   metadata.IsWildcard,
		"auto_renew":    metadata.AutoRenew,
		"renewal_count": metadata.RenewalCount,
		"last_renewal":  metadata.LastRenewal.Format("2006-01-02 15:04:05"),
		"cert_url":      metadata.CertURL,
	})
}

// DeleteCertificate deletes a certificate
func (h *Handler) DeleteCertificate(c *fiber.Ctx) error {
	domain := c.Params("domain")
	if domain == "" {
		return utils.ErrJSON(c, 400, fmt.Errorf("domain parameter is required"))
	}

	ctx := context.Background()
	if err := h.storage.DeleteCertificate(ctx, domain); err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to delete certificate: %w", err))
	}

	return c.JSON(fiber.Map{
		"status":  "deleted",
		"domain":  domain,
		"message": "Certificate deleted successfully",
	})
}

// RenewCertificate manually renews a specific certificate
func (h *Handler) RenewCertificate(c *fiber.Ctx) error {
	domain := c.Params("domain")
	if domain == "" {
		return utils.ErrJSON(c, 400, fmt.Errorf("domain parameter is required"))
	}

	ctx := context.Background()
	result, err := h.renewalService.RenewCertificate(ctx, domain)
	if err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to renew certificate: %w", err))
	}

	if !result.Success {
		return utils.ErrJSON(c, 500, fmt.Errorf("certificate renewal failed: %s", result.Error))
	}

	return c.JSON(fiber.Map{
		"status":   "renewed",
		"domain":   domain,
		"duration": result.Duration.String(),
		"message":  "Certificate renewed successfully",
	})
}

// RenewAllCertificates triggers renewal for all expiring certificates
func (h *Handler) RenewAllCertificates(c *fiber.Ctx) error {
	ctx := context.Background()
	results, err := h.renewalService.TriggerRenewal(ctx)
	if err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to trigger renewal: %w", err))
	}

	successful := 0
	failed := 0
	var errors []string

	for _, result := range results {
		if result.Success {
			successful++
		} else {
			failed++
			errors = append(errors, fmt.Sprintf("%s: %s", result.Domain, result.Error))
		}
	}

	response := fiber.Map{
		"status":     "completed",
		"successful": successful,
		"failed":     failed,
		"total":      len(results),
		"message":    fmt.Sprintf("Renewal completed: %d successful, %d failed", successful, failed),
	}

	if len(errors) > 0 {
		response["errors"] = errors
	}

	return c.JSON(response)
}

// CheckRenewal checks which certificates need renewal
func (h *Handler) CheckRenewal(c *fiber.Ctx) error {
	// Parse threshold from query parameter (default: 30 days)
	thresholdDays := 30
	if days := c.Query("days"); days != "" {
		if parsed, err := strconv.Atoi(days); err == nil && parsed > 0 {
			thresholdDays = parsed
		}
	}

	threshold := time.Duration(thresholdDays) * 24 * time.Hour
	ctx := context.Background()
	
	expiring, err := h.storage.GetExpiringSoon(ctx, threshold)
	if err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to check expiring certificates: %w", err))
	}

	var expiringList []fiber.Map
	for _, cert := range expiring {
		daysUntilExpiry := int(time.Until(cert.ExpiresAt).Hours() / 24)
		expiringList = append(expiringList, fiber.Map{
			"domain":            cert.Domain,
			"expires":           cert.ExpiresAt.Format("2006-01-02"),
			"days_until_expiry": daysUntilExpiry,
			"is_wildcard":       cert.IsWildcard,
			"auto_renew":        cert.AutoRenew,
		})
	}

	return c.JSON(fiber.Map{
		"expiring_certificates": expiringList,
		"count":                len(expiringList),
		"threshold_days":       thresholdDays,
		"message":              fmt.Sprintf("Found %d certificates expiring within %d days", len(expiringList), thresholdDays),
	})
}

// GetRenewalStatus gets the status of the renewal service
func (h *Handler) GetRenewalStatus(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"running":    h.renewalService.IsRunning(),
		"auto_renew": h.config.AutoRenew,
		"email":      h.config.Email,
		"staging":    h.config.Staging,
	})
}

// StartRenewalService starts the automatic renewal service
func (h *Handler) StartRenewalService(c *fiber.Ctx) error {
	if h.renewalService.IsRunning() {
		return c.JSON(fiber.Map{
			"status":  "already_running",
			"message": "Renewal service is already running",
		})
	}

	ctx := context.Background()
	if err := h.renewalService.Start(ctx); err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to start renewal service: %w", err))
	}

	return c.JSON(fiber.Map{
		"status":  "started",
		"message": "Renewal service started successfully",
	})
}

// StopRenewalService stops the automatic renewal service
func (h *Handler) StopRenewalService(c *fiber.Ctx) error {
	if !h.renewalService.IsRunning() {
		return c.JSON(fiber.Map{
			"status":  "not_running",
			"message": "Renewal service is not running",
		})
	}

	if err := h.renewalService.Stop(); err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to stop renewal service: %w", err))
	}

	return c.JSON(fiber.Map{
		"status":  "stopped",
		"message": "Renewal service stopped successfully",
	})
}

// GetRenewalStats gets renewal statistics
func (h *Handler) GetRenewalStats(c *fiber.Ctx) error {
	ctx := context.Background()
	stats, err := h.renewalService.GetRenewalStats(ctx)
	if err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to get renewal stats: %w", err))
	}

	return c.JSON(stats)
}

// GetConfig gets current ACME configuration
func (h *Handler) GetConfig(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"email":         h.config.Email,
		"staging":       h.config.Staging,
		"default_domain": h.config.DefaultDomain,
		"auto_renew":    h.config.AutoRenew,
		"renewal_threshold_days": int(h.config.RenewalThreshold.Hours() / 24),
	})
}

// UpdateConfig updates ACME configuration
func (h *Handler) UpdateConfig(c *fiber.Ctx) error {
	var req struct {
		Email            string `json:"email,omitempty"`
		Staging          *bool  `json:"staging,omitempty"`
		DefaultDomain    string `json:"default_domain,omitempty"`
		AutoRenew        *bool  `json:"auto_renew,omitempty"`
		RenewalThresholdDays *int `json:"renewal_threshold_days,omitempty"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return utils.ErrJSON(c, 400, fmt.Errorf("invalid request body"))
	}

	// Update configuration
	if req.Email != "" {
		h.config.Email = req.Email
	}
	if req.Staging != nil {
		h.config.Staging = *req.Staging
	}
	if req.DefaultDomain != "" {
		h.config.DefaultDomain = req.DefaultDomain
	}
	if req.AutoRenew != nil {
		h.config.AutoRenew = *req.AutoRenew
	}
	if req.RenewalThresholdDays != nil && *req.RenewalThresholdDays > 0 {
		h.config.RenewalThreshold = time.Duration(*req.RenewalThresholdDays) * 24 * time.Hour
	}

	// Save configuration (this would typically save to a file or database)
	if err := saveACMEConfig(h.config); err != nil {
		log.Printf("Warning: failed to save ACME configuration: %v", err)
	}

	return c.JSON(fiber.Map{
		"status":  "updated",
		"message": "Configuration updated successfully",
		"config":  h.config,
	})
}

// loadACMEConfig loads ACME configuration from environment variables
func loadACMEConfig() (*ACMEConfig, error) {
	config := &ACMEConfig{
		Email:            getEnvOrDefault("ACME_EMAIL", "admin@ployd.app"),
		Staging:          getEnvOrDefault("ACME_STAGING", "true") == "true",
		DefaultDomain:    getEnvOrDefault("ACME_DEFAULT_DOMAIN", "ployd.app"),
		AutoRenew:        getEnvOrDefault("ACME_AUTO_RENEW", "true") == "true",
		RenewalThreshold: 30 * 24 * time.Hour, // Default 30 days
	}

	if threshold := getEnvOrDefault("ACME_RENEWAL_THRESHOLD_DAYS", ""); threshold != "" {
		if days, err := strconv.Atoi(threshold); err == nil && days > 0 {
			config.RenewalThreshold = time.Duration(days) * 24 * time.Hour
		}
	}

	return config, nil
}

// saveACMEConfig saves ACME configuration (placeholder implementation)
func saveACMEConfig(config *ACMEConfig) error {
	// This would typically save to a configuration file or database
	// For now, just log the configuration
	log.Printf("ACME configuration updated: email=%s, staging=%v, auto_renew=%v", 
		config.Email, config.Staging, config.AutoRenew)
	return nil
}

// getEnvOrDefault gets an environment variable or returns a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return defaultValue
}