package acme

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"github.com/iw2rmb/ploy/controller/dns"
)

// Client represents an ACME client for Let's Encrypt operations
type Client struct {
	client       *lego.Client
	dnsProvider  dns.Provider
	email        string
	staging      bool
	user         *ACMEUser
}

// ACMEUser represents a user account for ACME operations
type ACMEUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

// GetEmail returns the user's email
func (u *ACMEUser) GetEmail() string {
	return u.Email
}

// GetRegistration returns the user's registration
func (u *ACMEUser) GetRegistration() *registration.Resource {
	return u.Registration
}

// GetPrivateKey returns the user's private key
func (u *ACMEUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

// Certificate represents an issued certificate
type Certificate struct {
	Domain      string    `json:"domain"`
	Certificate []byte    `json:"certificate"`
	PrivateKey  []byte    `json:"private_key"`
	IssuerCert  []byte    `json:"issuer_cert,omitempty"`
	CertURL     string    `json:"cert_url"`
	IssuedAt    time.Time `json:"issued_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	IsWildcard  bool      `json:"is_wildcard"`
}

// NewClient creates a new ACME client
func NewClient(email string, dnsProvider dns.Provider, staging bool) (*Client, error) {
	// Create a user with a new private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	user := &ACMEUser{
		Email: email,
		key:   privateKey,
	}

	// Create lego configuration
	config := lego.NewConfig(user)
	
	// Set ACME directory URL
	if staging {
		config.CADirURL = lego.LEDirectoryStaging
		log.Printf("Using Let's Encrypt staging environment")
	} else {
		config.CADirURL = lego.LEDirectoryProduction
		log.Printf("Using Let's Encrypt production environment")
	}
	
	config.Certificate.KeyType = certcrypto.RSA2048

	// Create the lego client
	client, err := lego.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create lego client: %w", err)
	}

	// Create DNS challenge provider wrapper
	dnsChallenge := &DNSProviderWrapper{
		provider: dnsProvider,
	}

	// Set the DNS challenge provider
	err = client.Challenge.SetDNS01Provider(dnsChallenge)
	if err != nil {
		return nil, fmt.Errorf("failed to set DNS provider: %w", err)
	}

	acmeClient := &Client{
		client:      client,
		dnsProvider: dnsProvider,
		email:       email,
		staging:     staging,
		user:        user,
	}

	// Register user
	if err := acmeClient.registerUser(); err != nil {
		return nil, fmt.Errorf("failed to register user: %w", err)
	}

	return acmeClient, nil
}

// registerUser registers the user with the ACME server
func (c *Client) registerUser() error {
	reg, err := c.client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return fmt.Errorf("failed to register: %w", err)
	}
	
	c.user.Registration = reg
	log.Printf("User registered with ACME server: %s", c.email)
	return nil
}

// IssueCertificate issues a certificate for the given domains
func (c *Client) IssueCertificate(ctx context.Context, domains []string) (*Certificate, error) {
	log.Printf("Issuing certificate for domains: %v", domains)

	// Create certificate request
	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}

	// Obtain certificate
	certificates, err := c.client.Certificate.Obtain(request)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain certificate: %w", err)
	}

	// Parse certificate to get expiration date
	block, _ := pem.Decode(certificates.Certificate)
	if block == nil {
		return nil, fmt.Errorf("failed to parse certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Determine if this is a wildcard certificate
	isWildcard := false
	for _, domain := range domains {
		if len(domain) > 0 && domain[0] == '*' {
			isWildcard = true
			break
		}
	}

	certificate := &Certificate{
		Domain:      domains[0], // Primary domain
		Certificate: certificates.Certificate,
		PrivateKey:  certificates.PrivateKey,
		IssuerCert:  certificates.IssuerCertificate,
		CertURL:     certificates.CertURL,
		IssuedAt:    time.Now(),
		ExpiresAt:   cert.NotAfter,
		IsWildcard:  isWildcard,
	}

	log.Printf("Certificate issued successfully for %s (expires: %s)", domains[0], cert.NotAfter.Format("2006-01-02"))
	return certificate, nil
}

// IssueWildcardCertificate issues a wildcard certificate for the given domain
func (c *Client) IssueWildcardCertificate(ctx context.Context, domain string) (*Certificate, error) {
	wildcardDomain := fmt.Sprintf("*.%s", domain)
	domains := []string{wildcardDomain, domain} // Include both wildcard and apex domain
	
	log.Printf("Issuing wildcard certificate for domain: %s", domain)
	return c.IssueCertificate(ctx, domains)
}

// RenewCertificate renews an existing certificate
func (c *Client) RenewCertificate(ctx context.Context, cert *Certificate) (*Certificate, error) {
	log.Printf("Renewing certificate for domain: %s", cert.Domain)

	// Parse the current certificate to get domains
	block, _ := pem.Decode(cert.Certificate)
	if block == nil {
		return nil, fmt.Errorf("failed to parse certificate PEM")
	}

	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Get all domains from the certificate
	domains := []string{parsedCert.Subject.CommonName}
	domains = append(domains, parsedCert.DNSNames...)

	// Remove duplicates
	uniqueDomains := make([]string, 0, len(domains))
	seen := make(map[string]bool)
	for _, domain := range domains {
		if domain != "" && !seen[domain] {
			uniqueDomains = append(uniqueDomains, domain)
			seen[domain] = true
		}
	}

	// Issue new certificate
	return c.IssueCertificate(ctx, uniqueDomains)
}

// NeedsRenewal checks if a certificate needs renewal (within 30 days of expiration)
func (c *Client) NeedsRenewal(cert *Certificate) bool {
	renewalThreshold := 30 * 24 * time.Hour // 30 days
	return time.Until(cert.ExpiresAt) < renewalThreshold
}

// ValidateCertificate validates that a certificate is valid and not expired
func (c *Client) ValidateCertificate(cert *Certificate) error {
	// Parse certificate
	block, _ := pem.Decode(cert.Certificate)
	if block == nil {
		return fmt.Errorf("failed to parse certificate PEM")
	}

	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Check expiration
	if time.Now().After(parsedCert.NotAfter) {
		return fmt.Errorf("certificate has expired on %s", parsedCert.NotAfter.Format("2006-01-02"))
	}

	// Check not valid before
	if time.Now().Before(parsedCert.NotBefore) {
		return fmt.Errorf("certificate is not yet valid (valid from %s)", parsedCert.NotBefore.Format("2006-01-02"))
	}

	return nil
}

// DNSProviderWrapper wraps our DNS provider to implement lego's DNS provider interface
type DNSProviderWrapper struct {
	provider dns.Provider
}

// Present creates a TXT record to fulfill the dns-01 challenge
func (d *DNSProviderWrapper) Present(domain, token, keyAuth string) error {
	fqdn, value := dns01.GetRecord(domain, keyAuth)
	
	// Extract hostname from FQDN
	hostname := fqdn
	if hostname[len(hostname)-1] == '.' {
		hostname = hostname[:len(hostname)-1] // Remove trailing dot
	}

	record := dns.Record{
		Hostname: hostname,
		Type:     dns.RecordTypeTXT,
		Value:    value,
		TTL:      120, // Short TTL for challenge records
	}

	log.Printf("Creating DNS TXT record for ACME challenge: %s = %s", hostname, value)
	
	ctx := context.Background()
	if err := d.provider.CreateRecord(ctx, record); err != nil {
		return fmt.Errorf("failed to create DNS challenge record: %w", err)
	}

	// Wait for DNS propagation
	log.Printf("Waiting for DNS propagation...")
	time.Sleep(30 * time.Second)

	return nil
}

// CleanUp removes the TXT record created for the dns-01 challenge
func (d *DNSProviderWrapper) CleanUp(domain, token, keyAuth string) error {
	fqdn, _ := dns01.GetRecord(domain, keyAuth)
	
	// Extract hostname from FQDN
	hostname := fqdn
	if hostname[len(hostname)-1] == '.' {
		hostname = hostname[:len(hostname)-1] // Remove trailing dot
	}

	log.Printf("Cleaning up DNS TXT record for ACME challenge: %s", hostname)
	
	ctx := context.Background()
	if err := d.provider.DeleteRecord(ctx, hostname, dns.RecordTypeTXT); err != nil {
		log.Printf("Warning: failed to clean up DNS challenge record: %v", err)
		// Don't return error for cleanup failures
	}

	return nil
}

// Timeout returns the timeout for DNS propagation
func (d *DNSProviderWrapper) Timeout() (timeout, interval time.Duration) {
	return 2 * time.Minute, 10 * time.Second
}