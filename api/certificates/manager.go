package certificates

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/acme"
	"github.com/iw2rmb/ploy/api/dns"
	certstore "github.com/iw2rmb/ploy/internal/certificates"
)

// CertificateManager coordinates ACME issuance, custom uploads, and JetStream persistence.
type CertificateManager struct {
	acmeClient              *acme.Client
	certificateStorage      *acme.CertificateStorage
	renewalService          *acme.RenewalService
	dnsProvider             dns.Provider
	store                   *certstore.Store
	config                  *CertConfig
	platformWildcardManager *PlatformWildcardCertificateManager
}

// CertConfig holds certificate management configuration.
type CertConfig struct {
	AutoProvision    bool          `json:"auto_provision"`
	Email            string        `json:"email"`
	Staging          bool          `json:"staging"`
	RenewalThreshold time.Duration `json:"renewal_threshold"`
	DefaultDomain    string        `json:"default_domain"`
}

// DomainCertificate represents a certificate associated with a domain.
type DomainCertificate struct {
	Domain       string    `json:"domain"`
	AppName      string    `json:"app_name"`
	Status       string    `json:"status"`
	Provider     string    `json:"provider"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	AutoRenew    bool      `json:"auto_renew"`
	BundleObject string    `json:"bundle_object,omitempty"`
	Revision     string    `json:"revision,omitempty"`
	Fingerprint  string    `json:"fingerprint_sha256"`
	LastError    string    `json:"last_error,omitempty"`
}

// NewCertificateManager creates a new certificate manager backed by JetStream.
func NewCertificateManager(store *certstore.Store, dnsProvider dns.Provider) (*CertificateManager, error) {
	if store == nil {
		return nil, fmt.Errorf("certificate store is required")
	}

	config := loadCertConfig()

	acmeClient, err := acme.NewClient(config.Email, dnsProvider, config.Staging)
	if err != nil {
		return nil, fmt.Errorf("failed to create ACME client: %w", err)
	}

	certStorage := acme.NewCertificateStorage(store)

	renewalConfig := acme.DefaultRenewalConfig()
	renewalConfig.RenewalThreshold = config.RenewalThreshold
	renewalService := acme.NewRenewalService(acmeClient, certStorage, dnsProvider, renewalConfig)

	manager := &CertificateManager{
		acmeClient:         acmeClient,
		certificateStorage: certStorage,
		renewalService:     renewalService,
		dnsProvider:        dnsProvider,
		store:              store,
		config:             config,
	}

	if config.AutoProvision {
		ctx := context.Background()
		if err := renewalService.Start(ctx); err != nil {
			log.Printf("Warning: failed to start renewal service: %v", err)
		}
	}

	return manager, nil
}

// SetPlatformWildcardManager sets the platform wildcard certificate manager.
func (cm *CertificateManager) SetPlatformWildcardManager(pwm *PlatformWildcardCertificateManager) {
	cm.platformWildcardManager = pwm
	log.Printf("Platform wildcard certificate manager integrated with certificate manager")
}

// ProvisionCertificate provisions or reuses a certificate for the provided domain.
func (cm *CertificateManager) ProvisionCertificate(ctx context.Context, appName, domain string) (*DomainCertificate, error) {
	if cm == nil {
		return nil, fmt.Errorf("certificate manager unavailable")
	}

	provider := "letsencrypt"
	autoRenew := true
	status := "active"

	var cert *acme.Certificate
	var err error

	if cm.platformWildcardManager != nil && cm.platformWildcardManager.IsPlatformSubdomain(domain) {
		cert, _, err = cm.platformWildcardManager.GetCertificateForDomain(ctx, domain)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch platform wildcard certificate: %w", err)
		}
		provider = "platform-wildcard"
		autoRenew = false
	} else {
		cert, err = cm.acmeClient.IssueCertificate(ctx, []string{domain})
		if err != nil {
			return nil, fmt.Errorf("failed to issue certificate: %w", err)
		}
	}

	meta, err := cm.certificateStorage.StoreCertificate(ctx, cert, acme.StoreOptions{
		App:          appName,
		Provider:     provider,
		AutoRenew:    autoRenew,
		Status:       status,
		CertURL:      cert.CertURL,
		RenewalCount: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to persist certificate: %w", err)
	}

	return convertMetadata(meta), nil
}

// GetDomainCertificate returns certificate metadata for the provided app/domain pair.
func (cm *CertificateManager) GetDomainCertificate(appName, domain string) (*DomainCertificate, error) {
	meta, err := cm.store.Get(context.Background(), domain)
	if err != nil {
		return nil, err
	}
	if meta.App != appName {
		return nil, fmt.Errorf("certificate for %s belongs to app %s", domain, meta.App)
	}
	return convertMetadata(meta), nil
}

// ListAppCertificates lists all certificates for an application.
func (cm *CertificateManager) ListAppCertificates(appName string) ([]*DomainCertificate, error) {
	entries, err := cm.store.List(context.Background())
	if err != nil {
		return nil, err
	}
	var result []*DomainCertificate
	for _, meta := range entries {
		if meta.App != appName {
			continue
		}
		result = append(result, convertMetadata(meta))
	}
	return result, nil
}

// RemoveDomainCertificate deletes certificate bundle and metadata for the provided domain.
func (cm *CertificateManager) RemoveDomainCertificate(appName, domain string) error {
	meta, err := cm.store.Get(context.Background(), domain)
	if err != nil {
		return err
	}
	if meta.App != appName {
		return fmt.Errorf("certificate for %s belongs to app %s", domain, meta.App)
	}
	return cm.certificateStorage.DeleteCertificate(context.Background(), domain)
}

// UploadCustomCertificate uploads a custom certificate bundle provided by the user.
func (cm *CertificateManager) UploadCustomCertificate(ctx context.Context, appName, domain string, certificate, privateKey, caCert []byte) (*DomainCertificate, error) {
	if len(certificate) == 0 {
		return nil, fmt.Errorf("certificate cannot be empty")
	}
	if len(privateKey) == 0 {
		return nil, fmt.Errorf("private key cannot be empty")
	}

	acmeCert := &acme.Certificate{
		Domain:      domain,
		Certificate: certificate,
		PrivateKey:  privateKey,
		IssuerCert:  caCert,
		CertURL:     "custom-uploaded",
		IssuedAt:    time.Now().UTC(),
		ExpiresAt:   time.Now().UTC().Add(365 * 24 * time.Hour),
		IsWildcard:  strings.HasPrefix(domain, "*"),
	}

	meta, err := cm.certificateStorage.StoreCertificate(ctx, acmeCert, acme.StoreOptions{
		App:       appName,
		Provider:  "custom",
		AutoRenew: false,
		Status:    "active",
		CertURL:   acmeCert.CertURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to store custom certificate: %w", err)
	}

	return convertMetadata(meta), nil
}

// CheckRenewalStatus returns certificates that require renewal soon.
func (cm *CertificateManager) CheckRenewalStatus(ctx context.Context) ([]*DomainCertificate, error) {
	entries, err := cm.certificateStorage.GetExpiringSoon(ctx, cm.config.RenewalThreshold)
	if err != nil {
		return nil, err
	}
	var result []*DomainCertificate
	for _, meta := range entries {
		result = append(result, convertMetadata(meta))
	}
	return result, nil
}

func convertMetadata(meta *certstore.Metadata) *DomainCertificate {
	if meta == nil {
		return nil
	}
	return &DomainCertificate{
		Domain:       meta.Domain,
		AppName:      meta.App,
		Status:       meta.Status,
		Provider:     meta.Provider,
		IssuedAt:     meta.IssuedAt,
		ExpiresAt:    meta.NotAfter,
		AutoRenew:    meta.AutoRenew,
		BundleObject: meta.BundleObject,
		Revision:     meta.Revision,
		Fingerprint:  meta.FingerprintSHA256,
		LastError:    meta.LastError,
	}
}

// GetRenewalService exposes the renewal service for wiring/testing.
func (cm *CertificateManager) GetRenewalService() *acme.RenewalService {
	return cm.renewalService
}

// loadCertConfig loads certificate manager configuration from environment.
func loadCertConfig() *CertConfig {
	config := &CertConfig{
		AutoProvision:    true,
		Email:            "admin@ployman.app",
		Staging:          false,
		RenewalThreshold: 30 * 24 * time.Hour,
		DefaultDomain:    os.Getenv("PLOY_APPS_DOMAIN"),
	}
	if threshold := os.Getenv("PLOY_CERT_RENEWAL_THRESHOLD_DAYS"); threshold != "" {
		if days, err := strconv.Atoi(threshold); err == nil && days > 0 {
			config.RenewalThreshold = time.Duration(days) * 24 * time.Hour
		}
	}
	if email := os.Getenv("PLOY_CERT_EMAIL"); email != "" {
		config.Email = email
	}
	if staging := os.Getenv("PLOY_CERT_STAGING"); strings.ToLower(staging) == "true" {
		config.Staging = true
	}
	return config
}
