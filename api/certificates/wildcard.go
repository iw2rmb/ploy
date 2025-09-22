package certificates

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/acme"
	"github.com/iw2rmb/ploy/api/dns"
)

// PlatformWildcardCertificateManager handles automatic wildcard certificate provisioning
// for the platform domain specified by PLOY_APPS_DOMAIN environment variable
type PlatformWildcardCertificateManager struct {
	acmeClient         *acme.Client
	certificateStorage *acme.CertificateStorage
	renewalService     *acme.RenewalService
	dnsProvider        dns.Provider
	platformDomain     string
	enabled            bool
}

// NewPlatformWildcardCertificateManager creates a new platform wildcard certificate manager
func NewPlatformWildcardCertificateManager(certManager *CertificateManager) (*PlatformWildcardCertificateManager, error) {
	platformDomain := os.Getenv("PLOY_APPS_DOMAIN")
	if platformDomain == "" {
		log.Println("PLOY_APPS_DOMAIN not set, platform wildcard certificate provisioning disabled")
		return &PlatformWildcardCertificateManager{enabled: false}, nil
	}

	// Check if this is a dev environment
	environment := os.Getenv("PLOY_ENVIRONMENT")
	if environment == "dev" {
		// For dev environment, use dev subdomain
		devSubdomain := os.Getenv("PLOY_DEV_SUBDOMAIN")
		if devSubdomain == "" {
			devSubdomain = "dev"
		}
		platformDomain = fmt.Sprintf("%s.%s", devSubdomain, platformDomain)
		log.Printf("Development environment detected, using dev subdomain: %s", platformDomain)
	}

	log.Printf("Platform wildcard certificate manager enabled for domain: %s", platformDomain)

	return &PlatformWildcardCertificateManager{
		acmeClient:         certManager.acmeClient,
		certificateStorage: certManager.certificateStorage,
		renewalService:     certManager.renewalService,
		dnsProvider:        certManager.dnsProvider,
		platformDomain:     platformDomain,
		enabled:            true,
	}, nil
}

// EnsurePlatformWildcardCertificate ensures a wildcard certificate exists and is valid for the platform domain
func (pwm *PlatformWildcardCertificateManager) EnsurePlatformWildcardCertificate(ctx context.Context) error {
	if !pwm.enabled {
		return nil
	}

	// Check if DNS provider is available for wildcard certificate provisioning
	if pwm.dnsProvider == nil {
		log.Printf("DNS provider not available, platform wildcard certificate provisioning disabled")
		return nil
	}

	wildcardDomain := "*." + pwm.platformDomain
	log.Printf("Ensuring platform wildcard certificate for %s", wildcardDomain)

	// Check if certificate already exists in SeaweedFS
	existing, _, err := pwm.certificateStorage.GetCertificate(ctx, wildcardDomain)
	if err == nil {
		// Certificate exists, check if it needs renewal
		renewalThreshold := 30 * 24 * time.Hour // 30 days
		if time.Until(existing.ExpiresAt) > renewalThreshold {
			log.Printf("Platform wildcard certificate for %s is valid until %v (%d days remaining)",
				wildcardDomain, existing.ExpiresAt, int(time.Until(existing.ExpiresAt).Hours()/24))
			return nil
		}
		log.Printf("Platform wildcard certificate for %s expires soon (%v), renewing...", wildcardDomain, existing.ExpiresAt)
	} else {
		log.Printf("Platform wildcard certificate for %s not found, provisioning new certificate", wildcardDomain)
	}

	// Provision new wildcard certificate
	log.Printf("Provisioning platform wildcard certificate for %s using DNS-01 challenge", wildcardDomain)
	cert, err := pwm.acmeClient.IssueCertificate(ctx, []string{wildcardDomain})
	if err != nil {
		return fmt.Errorf("failed to issue platform wildcard certificate: %w", err)
	}

	// Store in JetStream
	if _, err := pwm.certificateStorage.StoreCertificate(ctx, cert, acme.StoreOptions{
		App:       "platform",
		Provider:  "platform-wildcard",
		AutoRenew: false,
		Status:    "active",
		CertURL:   cert.CertURL,
	}); err != nil {
		return fmt.Errorf("failed to store platform wildcard certificate: %w", err)
	}

	// Note: Certificate renewal is handled automatically by the renewal service
	// The renewal service discovers certificates from storage and renews them as needed
	log.Printf("Platform wildcard certificate registered for automatic renewal")

	log.Printf("Platform wildcard certificate for %s provisioned successfully, expires: %v", wildcardDomain, cert.ExpiresAt)
	return nil
}

// GetPlatformWildcardCertificate retrieves the platform wildcard certificate if available
func (pwm *PlatformWildcardCertificateManager) GetPlatformWildcardCertificate(ctx context.Context) (*acme.Certificate, error) {
	if !pwm.enabled {
		return nil, fmt.Errorf("platform wildcard certificate management disabled")
	}

	wildcardDomain := "*." + pwm.platformDomain
	cert, _, err := pwm.certificateStorage.GetCertificate(ctx, wildcardDomain)
	return cert, err
}

// IsPlatformSubdomain checks if a domain is a direct subdomain of the platform domain
func (pwm *PlatformWildcardCertificateManager) IsPlatformSubdomain(domain string) bool {
	if !pwm.enabled {
		return false
	}

	// Check if domain is a direct subdomain of the platform domain
	// Examples:
	//   Platform domain: ployd.app
	//   myapp.ployd.app -> TRUE (direct subdomain)
	//   api.myapp.ployd.app -> FALSE (nested subdomain, not covered by wildcard)
	//   external.com -> FALSE (different domain)
	//   ployd.app -> FALSE (apex domain, not subdomain)

	if !strings.HasSuffix(domain, "."+pwm.platformDomain) {
		return false
	}

	// Count dots to ensure it's a direct subdomain
	domainDotCount := strings.Count(domain, ".")
	platformDotCount := strings.Count(pwm.platformDomain, ".")

	return domainDotCount == platformDotCount+1
}

// GetCertificateForDomain returns the appropriate certificate for a domain
// Returns (certificate, isWildcard, error)
func (pwm *PlatformWildcardCertificateManager) GetCertificateForDomain(ctx context.Context, domain string) (*acme.Certificate, bool, error) {
	if pwm.IsPlatformSubdomain(domain) {
		// Use platform wildcard certificate
		cert, err := pwm.GetPlatformWildcardCertificate(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("failed to get platform wildcard certificate for domain %s: %w", domain, err)
		}
		return cert, true, nil // true indicates wildcard certificate used
	}
	// Return false to indicate individual certificate should be used
	return nil, false, nil
}

// GetPlatformDomain returns the platform domain
func (pwm *PlatformWildcardCertificateManager) GetPlatformDomain() string {
	return pwm.platformDomain
}

// IsEnabled returns whether platform wildcard certificate management is enabled
func (pwm *PlatformWildcardCertificateManager) IsEnabled() bool {
	return pwm.enabled
}

// GetWildcardDomain returns the wildcard domain pattern for the platform
func (pwm *PlatformWildcardCertificateManager) GetWildcardDomain() string {
	if !pwm.enabled {
		return ""
	}
	return "*." + pwm.platformDomain
}

// ValidatePlatformDomain validates that the platform domain is properly configured
func (pwm *PlatformWildcardCertificateManager) ValidatePlatformDomain() error {
	if !pwm.enabled {
		return fmt.Errorf("platform wildcard certificate management disabled - PLOY_APPS_DOMAIN not set")
	}

	if pwm.platformDomain == "" {
		return fmt.Errorf("platform domain is empty")
	}

	// Basic domain validation
	if !strings.Contains(pwm.platformDomain, ".") {
		return fmt.Errorf("platform domain %s appears to be invalid (no dots)", pwm.platformDomain)
	}

	if strings.HasPrefix(pwm.platformDomain, ".") || strings.HasSuffix(pwm.platformDomain, ".") {
		return fmt.Errorf("platform domain %s appears to be invalid (starts or ends with dot)", pwm.platformDomain)
	}

	return nil
}
