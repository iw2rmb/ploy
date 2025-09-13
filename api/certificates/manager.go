package certificates

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/iw2rmb/ploy/api/acme"
	"github.com/iw2rmb/ploy/api/dns"
	"github.com/iw2rmb/ploy/internal/storage"
)

// CertificateManager manages Heroku-style certificate provisioning for domains
type CertificateManager struct {
	acmeClient              *acme.Client
	certificateStorage      *acme.CertificateStorage
	renewalService          *acme.RenewalService
	dnsProvider             dns.Provider
	consulClient            *consulapi.Client
	config                  *CertConfig
	platformWildcardManager *PlatformWildcardCertificateManager
}

// CertConfig holds certificate management configuration
type CertConfig struct {
	AutoProvision    bool          `json:"auto_provision"`    // Auto-provision certificates for new domains
	Email            string        `json:"email"`             // Let's Encrypt account email
	Staging          bool          `json:"staging"`           // Use staging environment
	RenewalThreshold time.Duration `json:"renewal_threshold"` // When to renew certificates
	DefaultDomain    string        `json:"default_domain"`    // Default domain (e.g., ployd.app)
}

// DomainCertificate represents a certificate associated with a domain
type DomainCertificate struct {
	Domain    string    `json:"domain"`
	AppName   string    `json:"app_name"`
	Status    string    `json:"status"`   // "provisioning", "active", "expired", "failed"
	Provider  string    `json:"provider"` // "letsencrypt", "custom"
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	AutoRenew bool      `json:"auto_renew"`
	CertPath  string    `json:"cert_path,omitempty"`  // Storage path for certificate
	KeyPath   string    `json:"key_path,omitempty"`   // Storage path for private key
	LastError string    `json:"last_error,omitempty"` // Last error message
}

// NewCertificateManager creates a new certificate manager
func NewCertificateManager(consulClient *consulapi.Client, storageClient storage.Storage, dnsProvider dns.Provider) (*CertificateManager, error) {
	// Load configuration
	config := loadCertConfig()

	// Create ACME client
	acmeClient, err := acme.NewClient(config.Email, dnsProvider, config.Staging)
	if err != nil {
		return nil, fmt.Errorf("failed to create ACME client: %w", err)
	}

	// Create certificate storage directly with Storage interface
	certStorage := acme.NewCertificateStorage(consulClient, storageClient)

	// Create renewal service
	renewalConfig := acme.DefaultRenewalConfig()
	renewalConfig.RenewalThreshold = config.RenewalThreshold
	renewalService := acme.NewRenewalService(acmeClient, certStorage, dnsProvider, renewalConfig)

	manager := &CertificateManager{
		acmeClient:         acmeClient,
		certificateStorage: certStorage,
		renewalService:     renewalService,
		dnsProvider:        dnsProvider,
		consulClient:       consulClient,
		config:             config,
	}

	// Start renewal service if auto-provision is enabled
	if config.AutoProvision {
		ctx := context.Background()
		if err := renewalService.Start(ctx); err != nil {
			log.Printf("Warning: failed to start renewal service: %v", err)
		}
	}

	return manager, nil
}

// SetPlatformWildcardManager sets the platform wildcard certificate manager
func (cm *CertificateManager) SetPlatformWildcardManager(pwm *PlatformWildcardCertificateManager) {
	cm.platformWildcardManager = pwm
	log.Printf("Platform wildcard certificate manager integrated with certificate manager")
}

// ProvisionCertificate provisions a certificate for a domain (Heroku-style)
func (cm *CertificateManager) ProvisionCertificate(ctx context.Context, appName, domain string) (*DomainCertificate, error) {
	log.Printf("Provisioning certificate for domain %s (app: %s)", domain, appName)

	// Check if certificate already exists
	if existing, err := cm.getDomainCertificate(appName, domain); err == nil && existing.Status == "active" {
		log.Printf("Certificate already exists for domain %s", domain)
		return existing, nil
	}

	// Create domain certificate record
	domainCert := &DomainCertificate{
		Domain:    domain,
		AppName:   appName,
		Status:    "provisioning",
		Provider:  "letsencrypt",
		AutoRenew: true,
	}

	// Store initial status
	if err := cm.storeDomainCertificate(domainCert); err != nil {
		return nil, fmt.Errorf("failed to store initial certificate record: %w", err)
	}

	// Determine certificate type using platform wildcard manager
	var cert *acme.Certificate
	var err error
	var usedWildcard bool

	// Check if platform wildcard certificate manager is available and domain matches
	if cm.platformWildcardManager != nil && cm.platformWildcardManager.IsPlatformSubdomain(domain) {
		// Use platform wildcard certificate
		cert, usedWildcard, err = cm.platformWildcardManager.GetCertificateForDomain(ctx, domain)
		if err != nil {
			log.Printf("Failed to get platform wildcard certificate for %s: %v", domain, err)
		}
	}

	// Fallback to individual certificate for external domains or if wildcard fails
	if cert == nil {
		log.Printf("Issuing individual certificate for external domain: %s", domain)
		cert, err = cm.acmeClient.IssueCertificate(ctx, []string{domain})
		usedWildcard = false
	}

	if usedWildcard {
		log.Printf("Using platform wildcard certificate for domain: %s", domain)
	}

	if err != nil {
		domainCert.Status = "failed"
		domainCert.LastError = err.Error()
		if storeErr := cm.storeDomainCertificate(domainCert); storeErr != nil {
			log.Printf("warning: failed to persist failed cert status: %v", storeErr)
		}
		return nil, fmt.Errorf("failed to issue certificate: %w", err)
	}

	// Store certificate (skip for platform wildcard certificates - already stored)
	if !usedWildcard {
		if err := cm.certificateStorage.StoreCertificate(ctx, cert); err != nil {
			domainCert.Status = "failed"
			domainCert.LastError = fmt.Sprintf("failed to store certificate: %v", err)
			if storeErr := cm.storeDomainCertificate(domainCert); storeErr != nil {
				log.Printf("warning: failed to persist failed cert status: %v", storeErr)
			}
			return nil, fmt.Errorf("failed to store certificate: %w", err)
		}
	}

	// Update domain certificate record
	domainCert.Status = "active"
	domainCert.IssuedAt = cert.IssuedAt
	domainCert.ExpiresAt = cert.ExpiresAt
	domainCert.CertPath = fmt.Sprintf("certificates/%s/cert.pem", cert.Domain)
	domainCert.KeyPath = fmt.Sprintf("certificates/%s/key.pem", cert.Domain)
	domainCert.LastError = ""

	if err := cm.storeDomainCertificate(domainCert); err != nil {
		log.Printf("Warning: failed to update certificate record: %v", err)
	}

	log.Printf("Certificate provisioned successfully for domain %s", domain)
	return domainCert, nil
}

// GetDomainCertificate gets certificate information for a domain
func (cm *CertificateManager) GetDomainCertificate(appName, domain string) (*DomainCertificate, error) {
	return cm.getDomainCertificate(appName, domain)
}

// ListAppCertificates lists all certificates for an app
func (cm *CertificateManager) ListAppCertificates(appName string) ([]*DomainCertificate, error) {
	key := fmt.Sprintf("ploy/certificates/apps/%s", appName)

	pairs, _, err := cm.consulClient.KV().List(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list app certificates: %w", err)
	}

	var certificates []*DomainCertificate
	for _, pair := range pairs {
		var cert DomainCertificate
		if err := json.Unmarshal(pair.Value, &cert); err != nil {
			log.Printf("Warning: failed to unmarshal certificate record: %v", err)
			continue
		}
		certificates = append(certificates, &cert)
	}

	return certificates, nil
}

// RemoveDomainCertificate removes a certificate for a domain
func (cm *CertificateManager) RemoveDomainCertificate(appName, domain string) error {
	log.Printf("Removing certificate for domain %s (app: %s)", domain, appName)

	// Get certificate record
	_, err := cm.getDomainCertificate(appName, domain)
	if err != nil {
		return fmt.Errorf("certificate not found: %w", err)
	}

	// Don't remove wildcard certificates (they may be used by other domains)
	if !cm.isDefaultDomainSubdomain(domain) {
		// Remove from ACME storage (for custom domains only)
		ctx := context.Background()
		if err := cm.certificateStorage.DeleteCertificate(ctx, domain); err != nil {
			log.Printf("Warning: failed to delete certificate from storage: %v", err)
		}
	}

	// Remove domain certificate record
	key := fmt.Sprintf("ploy/certificates/apps/%s/%s", appName, domain)
	_, err = cm.consulClient.KV().Delete(key, nil)
	if err != nil {
		return fmt.Errorf("failed to remove certificate record: %w", err)
	}

	log.Printf("Certificate removed for domain %s", domain)
	return nil
}

// CheckRenewalStatus checks which certificates need renewal
func (cm *CertificateManager) CheckRenewalStatus(ctx context.Context) ([]*DomainCertificate, error) {
	// Get all certificates that need renewal
	expiring, err := cm.certificateStorage.GetExpiringSoon(ctx, cm.config.RenewalThreshold)
	if err != nil {
		return nil, err
	}

	var renewalNeeded []*DomainCertificate
	for _, meta := range expiring {
		// Find domain certificates that use this ACME certificate
		key := "ploy/certificates/apps/"
		pairs, _, err := cm.consulClient.KV().List(key, nil)
		if err != nil {
			continue
		}

		for _, pair := range pairs {
			var cert DomainCertificate
			if err := json.Unmarshal(pair.Value, &cert); err != nil {
				continue
			}

			// Check if this domain certificate uses the expiring ACME certificate
			if cert.Status == "active" && cert.ExpiresAt.Equal(meta.ExpiresAt) {
				renewalNeeded = append(renewalNeeded, &cert)
			}
		}
	}

	return renewalNeeded, nil
}

// ensureWildcardCertificate ensures a wildcard certificate exists for a domain
func (cm *CertificateManager) ensureWildcardCertificate(ctx context.Context, baseDomain string) (*acme.Certificate, error) {
	wildcardDomain := fmt.Sprintf("*.%s", baseDomain)

	// Check if wildcard certificate already exists
	if cert, _, err := cm.certificateStorage.GetCertificate(ctx, wildcardDomain); err == nil {
		// Check if it needs renewal
		if !cm.acmeClient.NeedsRenewal(cert) {
			return cert, nil
		}
		// Renew if needed
		return cm.acmeClient.RenewCertificate(ctx, cert)
	}

	// Issue new wildcard certificate
	return cm.acmeClient.IssueWildcardCertificate(ctx, baseDomain)
}

// isDefaultDomainSubdomain checks if a domain is a subdomain of the default domain
func (cm *CertificateManager) isDefaultDomainSubdomain(domain string) bool {
	return strings.HasSuffix(domain, "."+cm.config.DefaultDomain)
}

// storeDomainCertificate stores domain certificate metadata in Consul
func (cm *CertificateManager) storeDomainCertificate(cert *DomainCertificate) error {
	key := fmt.Sprintf("ploy/certificates/apps/%s/%s", cert.AppName, cert.Domain)

	data, err := json.Marshal(cert)
	if err != nil {
		return fmt.Errorf("failed to marshal certificate: %w", err)
	}

	pair := &consulapi.KVPair{
		Key:   key,
		Value: data,
	}

	_, err = cm.consulClient.KV().Put(pair, nil)
	if err != nil {
		return fmt.Errorf("failed to store certificate in Consul: %w", err)
	}

	return nil
}

// getDomainCertificate retrieves domain certificate metadata from Consul
func (cm *CertificateManager) getDomainCertificate(appName, domain string) (*DomainCertificate, error) {
	key := fmt.Sprintf("ploy/certificates/apps/%s/%s", appName, domain)

	pair, _, err := cm.consulClient.KV().Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get certificate from Consul: %w", err)
	}

	if pair == nil {
		return nil, fmt.Errorf("certificate not found for domain: %s", domain)
	}

	var cert DomainCertificate
	if err := json.Unmarshal(pair.Value, &cert); err != nil {
		return nil, fmt.Errorf("failed to unmarshal certificate: %w", err)
	}

	return &cert, nil
}

// UploadCustomCertificate uploads a custom certificate bundle provided by the user
func (cm *CertificateManager) UploadCustomCertificate(ctx context.Context, appName, domain string, certificate, privateKey, caCert []byte) (*DomainCertificate, error) {
	log.Printf("Uploading custom certificate for domain %s (app: %s)", domain, appName)

	// Basic validation of certificate content
	if len(certificate) == 0 {
		return nil, fmt.Errorf("certificate cannot be empty")
	}
	if len(privateKey) == 0 {
		return nil, fmt.Errorf("private key cannot be empty")
	}

	// Check if certificate already exists and remove it
	if existing, err := cm.getDomainCertificate(appName, domain); err == nil {
		log.Printf("Replacing existing certificate for domain %s", domain)
		// Remove existing certificate from ACME storage if it was auto-generated
		if existing.Provider == "letsencrypt" {
			if err := cm.certificateStorage.DeleteCertificate(ctx, domain); err != nil {
				log.Printf("Warning: failed to delete existing ACME certificate: %v", err)
			}
		}
	}

	// Create ACME certificate object for storage
	acmeCert := &acme.Certificate{
		Domain:      domain,
		Certificate: certificate,
		PrivateKey:  privateKey,
		IssuerCert:  caCert,
		CertURL:     "custom-uploaded",
		IssuedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(365 * 24 * time.Hour), // Default 1 year, would need proper parsing
		IsWildcard:  strings.HasPrefix(domain, "*."),
	}

	// Store certificate in ACME storage
	if err := cm.certificateStorage.StoreCertificate(ctx, acmeCert); err != nil {
		return nil, fmt.Errorf("failed to store custom certificate: %w", err)
	}

	// Create domain certificate record
	domainCert := &DomainCertificate{
		Domain:    domain,
		AppName:   appName,
		Status:    "active",
		Provider:  "custom",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour), // Default 1 year
		AutoRenew: false,                                // Custom certificates don't auto-renew
		CertPath:  fmt.Sprintf("certificates/%s/cert.pem", domain),
		KeyPath:   fmt.Sprintf("certificates/%s/key.pem", domain),
	}

	// Store domain certificate record
	if err := cm.storeDomainCertificate(domainCert); err != nil {
		return nil, fmt.Errorf("failed to store domain certificate record: %w", err)
	}

	log.Printf("Custom certificate uploaded successfully for domain %s", domain)
	return domainCert, nil
}

// loadCertConfig loads certificate configuration from environment
func loadCertConfig() *CertConfig {
	return &CertConfig{
		AutoProvision:    getEnvOrDefault("CERT_AUTO_PROVISION", "true") == "true",
		Email:            getEnvOrDefault("CERT_EMAIL", "admin@ployd.app"),
		Staging:          getEnvOrDefault("CERT_STAGING", "true") == "true",
		RenewalThreshold: 30 * 24 * time.Hour, // 30 days
		DefaultDomain:    getEnvOrDefault("CERT_DEFAULT_DOMAIN", "ployd.app"),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return defaultValue
}
