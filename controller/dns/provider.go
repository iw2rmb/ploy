package dns

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

// Provider defines the interface for DNS providers
type Provider interface {
	// CreateRecord creates a DNS record
	CreateRecord(ctx context.Context, record Record) error
	
	// UpdateRecord updates an existing DNS record
	UpdateRecord(ctx context.Context, record Record) error
	
	// DeleteRecord deletes a DNS record
	DeleteRecord(ctx context.Context, hostname string, recordType string) error
	
	// GetRecord retrieves a specific DNS record
	GetRecord(ctx context.Context, hostname string, recordType string) (*Record, error)
	
	// ListRecords lists all DNS records for a domain
	ListRecords(ctx context.Context, domain string) ([]Record, error)
	
	// CreateWildcardRecord creates a wildcard DNS record
	CreateWildcardRecord(ctx context.Context, domain string, target string) error
	
	// ValidateConfiguration validates the provider configuration
	ValidateConfiguration() error
}

// Record represents a DNS record
type Record struct {
	Hostname   string    `json:"hostname"`
	Type       string    `json:"type"`       // A, AAAA, CNAME, TXT, MX, etc.
	Value      string    `json:"value"`
	TTL        int       `json:"ttl"`
	Priority   int       `json:"priority,omitempty"` // For MX records
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// RecordType constants
const (
	RecordTypeA     = "A"
	RecordTypeAAAA  = "AAAA"
	RecordTypeCNAME = "CNAME"
	RecordTypeTXT   = "TXT"
	RecordTypeMX    = "MX"
	RecordTypeNS    = "NS"
	RecordTypeSRV   = "SRV"
)

// WildcardConfig represents wildcard DNS configuration
type WildcardConfig struct {
	Domain        string   `json:"domain"`         // Base domain (e.g., "ployd.app")
	WildcardHost  string   `json:"wildcard_host"`  // Wildcard hostname (e.g., "*")
	TargetIP      string   `json:"target_ip"`      // Target IP address
	TargetCNAME   string   `json:"target_cname"`   // Alternative: target CNAME
	TTL           int      `json:"ttl"`
	EnableIPv6    bool     `json:"enable_ipv6"`
	IPv6Target    string   `json:"ipv6_target,omitempty"`
	LoadBalancer  []string `json:"load_balancer,omitempty"` // Multiple IPs for load balancing
}

// Manager handles DNS operations
type Manager struct {
	provider Provider
	config   *WildcardConfig
}

// NewManager creates a new DNS manager
func NewManager(provider Provider, config *WildcardConfig) *Manager {
	if config.TTL == 0 {
		config.TTL = 300 // Default 5 minutes TTL
	}
	
	return &Manager{
		provider: provider,
		config:   config,
	}
}

// SetupWildcardDNS sets up wildcard DNS for the configured domain
func (m *Manager) SetupWildcardDNS(ctx context.Context) error {
	log.Printf("Setting up wildcard DNS for *.%s", m.config.Domain)
	
	// Validate configuration
	if err := m.validateWildcardConfig(); err != nil {
		return fmt.Errorf("invalid wildcard configuration: %w", err)
	}
	
	// Create wildcard A record
	if m.config.TargetIP != "" {
		wildcardRecord := Record{
			Hostname: fmt.Sprintf("*.%s", m.config.Domain),
			Type:     RecordTypeA,
			Value:    m.config.TargetIP,
			TTL:      m.config.TTL,
		}
		
		if err := m.provider.CreateRecord(ctx, wildcardRecord); err != nil {
			// Try to update if create fails (record might exist)
			if updateErr := m.provider.UpdateRecord(ctx, wildcardRecord); updateErr != nil {
				return fmt.Errorf("failed to create/update wildcard A record: create=%v, update=%v", err, updateErr)
			}
		}
		log.Printf("Created/updated wildcard A record: *.%s -> %s", m.config.Domain, m.config.TargetIP)
	}
	
	// Create wildcard CNAME record if specified
	if m.config.TargetCNAME != "" {
		wildcardRecord := Record{
			Hostname: fmt.Sprintf("*.%s", m.config.Domain),
			Type:     RecordTypeCNAME,
			Value:    m.config.TargetCNAME,
			TTL:      m.config.TTL,
		}
		
		if err := m.provider.CreateRecord(ctx, wildcardRecord); err != nil {
			if updateErr := m.provider.UpdateRecord(ctx, wildcardRecord); updateErr != nil {
				return fmt.Errorf("failed to create/update wildcard CNAME record: create=%v, update=%v", err, updateErr)
			}
		}
		log.Printf("Created/updated wildcard CNAME record: *.%s -> %s", m.config.Domain, m.config.TargetCNAME)
	}
	
	// Create IPv6 AAAA record if enabled
	if m.config.EnableIPv6 && m.config.IPv6Target != "" {
		ipv6Record := Record{
			Hostname: fmt.Sprintf("*.%s", m.config.Domain),
			Type:     RecordTypeAAAA,
			Value:    m.config.IPv6Target,
			TTL:      m.config.TTL,
		}
		
		if err := m.provider.CreateRecord(ctx, ipv6Record); err != nil {
			if updateErr := m.provider.UpdateRecord(ctx, ipv6Record); updateErr != nil {
				log.Printf("Warning: failed to create/update wildcard IPv6 record: %v", updateErr)
			} else {
				log.Printf("Created/updated wildcard AAAA record: *.%s -> %s", m.config.Domain, m.config.IPv6Target)
			}
		}
	}
	
	// Setup load balancer records if multiple IPs provided
	if len(m.config.LoadBalancer) > 0 {
		for i, ip := range m.config.LoadBalancer {
			lbRecord := Record{
				Hostname: fmt.Sprintf("*.%s", m.config.Domain),
				Type:     RecordTypeA,
				Value:    ip,
				TTL:      m.config.TTL,
			}
			
			if err := m.provider.CreateRecord(ctx, lbRecord); err != nil {
				log.Printf("Warning: failed to create load balancer record %d: %v", i, err)
			} else {
				log.Printf("Created load balancer record %d: *.%s -> %s", i, m.config.Domain, ip)
			}
		}
	}
	
	log.Printf("Wildcard DNS setup completed for *.%s", m.config.Domain)
	return nil
}

// CreateAppSubdomain creates a specific subdomain for an app
func (m *Manager) CreateAppSubdomain(ctx context.Context, appName string, target string) error {
	// Create app-specific subdomain
	record := Record{
		Hostname: fmt.Sprintf("%s.%s", appName, m.config.Domain),
		Type:     RecordTypeA,
		Value:    target,
		TTL:      m.config.TTL,
	}
	
	if err := m.provider.CreateRecord(ctx, record); err != nil {
		// Try to update if create fails
		if updateErr := m.provider.UpdateRecord(ctx, record); updateErr != nil {
			return fmt.Errorf("failed to create/update app subdomain: %w", updateErr)
		}
	}
	
	log.Printf("Created subdomain: %s.%s -> %s", appName, m.config.Domain, target)
	return nil
}

// CreatePreviewSubdomain creates a preview subdomain for an app
func (m *Manager) CreatePreviewSubdomain(ctx context.Context, sha string, appName string) error {
	// Preview subdomains follow pattern: <sha>.<app>.ployd.app
	hostname := fmt.Sprintf("%s.%s.%s", sha, appName, m.config.Domain)
	
	// Use wildcard target or specific IP
	target := m.config.TargetIP
	if target == "" && m.config.TargetCNAME != "" {
		// Create CNAME instead
		record := Record{
			Hostname: hostname,
			Type:     RecordTypeCNAME,
			Value:    m.config.TargetCNAME,
			TTL:      m.config.TTL,
		}
		return m.provider.CreateRecord(ctx, record)
	}
	
	record := Record{
		Hostname: hostname,
		Type:     RecordTypeA,
		Value:    target,
		TTL:      m.config.TTL,
	}
	
	if err := m.provider.CreateRecord(ctx, record); err != nil {
		return fmt.Errorf("failed to create preview subdomain: %w", err)
	}
	
	log.Printf("Created preview subdomain: %s -> %s", hostname, target)
	return nil
}

// ValidateWildcardDNS validates that wildcard DNS is properly configured
func (m *Manager) ValidateWildcardDNS(ctx context.Context) error {
	// Generate a random test subdomain
	testHost := fmt.Sprintf("test-%d.%s", time.Now().Unix(), m.config.Domain)
	
	// Perform DNS lookup
	ips, err := net.LookupIP(testHost)
	if err != nil {
		return fmt.Errorf("wildcard DNS validation failed for %s: %w", testHost, err)
	}
	
	if len(ips) == 0 {
		return fmt.Errorf("no IP addresses found for wildcard test host %s", testHost)
	}
	
	// Check if resolved IP matches configured target
	expectedIP := m.config.TargetIP
	if expectedIP != "" {
		found := false
		for _, ip := range ips {
			if ip.String() == expectedIP {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("wildcard DNS resolves to %v, expected %s", ips, expectedIP)
		}
	}
	
	log.Printf("Wildcard DNS validation successful: %s resolves to %v", testHost, ips)
	return nil
}

// validateWildcardConfig validates the wildcard configuration
func (m *Manager) validateWildcardConfig() error {
	if m.config.Domain == "" {
		return fmt.Errorf("domain is required")
	}
	
	// Must have either target IP or CNAME
	if m.config.TargetIP == "" && m.config.TargetCNAME == "" {
		return fmt.Errorf("either target_ip or target_cname is required")
	}
	
	// Validate IP addresses
	if m.config.TargetIP != "" {
		if net.ParseIP(m.config.TargetIP) == nil {
			return fmt.Errorf("invalid target IP address: %s", m.config.TargetIP)
		}
	}
	
	if m.config.EnableIPv6 && m.config.IPv6Target != "" {
		if net.ParseIP(m.config.IPv6Target) == nil {
			return fmt.Errorf("invalid IPv6 target address: %s", m.config.IPv6Target)
		}
	}
	
	// Validate load balancer IPs
	for _, ip := range m.config.LoadBalancer {
		if net.ParseIP(ip) == nil {
			return fmt.Errorf("invalid load balancer IP: %s", ip)
		}
	}
	
	// Validate domain format
	if strings.Contains(m.config.Domain, "*") {
		return fmt.Errorf("domain should not contain wildcard character")
	}
	
	return nil
}

// RemoveWildcardDNS removes wildcard DNS configuration
func (m *Manager) RemoveWildcardDNS(ctx context.Context) error {
	wildcardHost := fmt.Sprintf("*.%s", m.config.Domain)
	
	// Remove A record
	if err := m.provider.DeleteRecord(ctx, wildcardHost, RecordTypeA); err != nil {
		log.Printf("Warning: failed to delete wildcard A record: %v", err)
	}
	
	// Remove CNAME record
	if err := m.provider.DeleteRecord(ctx, wildcardHost, RecordTypeCNAME); err != nil {
		log.Printf("Warning: failed to delete wildcard CNAME record: %v", err)
	}
	
	// Remove AAAA record
	if err := m.provider.DeleteRecord(ctx, wildcardHost, RecordTypeAAAA); err != nil {
		log.Printf("Warning: failed to delete wildcard AAAA record: %v", err)
	}
	
	log.Printf("Removed wildcard DNS configuration for *.%s", m.config.Domain)
	return nil
}