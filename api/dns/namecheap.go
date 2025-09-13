package dns

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NamecheapProvider implements DNS provider for Namecheap
type NamecheapProvider struct {
	apiUser  string
	apiKey   string
	username string
	clientIP string
	baseURL  string
	client   *http.Client
}

// NamecheapConfig holds Namecheap-specific configuration
type NamecheapConfig struct {
	APIUser  string `json:"api_user"`  // API user (usually same as username)
	APIKey   string `json:"api_key"`   // API key from Namecheap
	Username string `json:"username"`  // Namecheap username
	ClientIP string `json:"client_ip"` // Client IP address (required by Namecheap)
	Sandbox  bool   `json:"sandbox"`   // Use sandbox environment
}

// NamecheapResponse represents the standard Namecheap API response structure
type NamecheapResponse struct {
	XMLName xml.Name `xml:"http://api.namecheap.com/xml.response ApiResponse"`
	Status  string   `xml:"Status,attr"`
	Errors  struct {
		Error []NamecheapError `xml:"Error"`
	} `xml:"Errors"`
	CommandResponse NamecheapCommandResponse `xml:"CommandResponse"`
}

// NamecheapCommandResponse wraps command-specific responses
type NamecheapCommandResponse struct {
	DomainDNSGetHostsResult *NamecheapDNSGetHostsResult `xml:"DomainDNSGetHostsResult"`
	DomainDNSSetHostsResult *NamecheapDNSSetHostsResult `xml:"DomainDNSSetHostsResult"`
	// For other responses we don't have specific structs for
	Raw string `xml:",innerxml"`
}

// NamecheapError represents an API error from Namecheap
type NamecheapError struct {
	Number      string `xml:"Number,attr"`
	Description string `xml:",chardata"`
}

// NamecheapDNSGetHostsResult represents the result from DNS getHosts API
type NamecheapDNSGetHostsResult struct {
	Domain        string                `xml:"Domain,attr"`
	EmailType     string                `xml:"EmailType,attr"`
	IsUsingOurDNS bool                  `xml:"IsUsingOurDNS,attr"`
	Host          []NamecheapHostRecord `xml:"host"`
}

// NamecheapHostRecord represents a DNS host record
type NamecheapHostRecord struct {
	HostId             int    `xml:"HostId,attr"`
	Name               string `xml:"Name,attr"`
	Type               string `xml:"Type,attr"`
	Address            string `xml:"Address,attr"`
	MXPref             string `xml:"MXPref,attr"`
	TTL                int    `xml:"TTL,attr"`
	AssociatedAppTitle string `xml:"AssociatedAppTitle,attr"`
	FriendlyName       string `xml:"FriendlyName,attr"`
	IsActive           bool   `xml:"IsActive,attr"`
	IsDDNSEnabled      bool   `xml:"IsDDNSEnabled,attr"`
}

// NamecheapDNSSetHostsResult represents the result from DNS setHosts API
type NamecheapDNSSetHostsResult struct {
	Domain    string `xml:"Domain,attr"`
	IsSuccess bool   `xml:"IsSuccess,attr"`
}

// NewNamecheapProvider creates a new Namecheap DNS provider
func NewNamecheapProvider(config NamecheapConfig) (*NamecheapProvider, error) {
	if config.APIUser == "" || config.APIKey == "" || config.Username == "" {
		return nil, fmt.Errorf("api_user, api_key, and username are required")
	}

	if config.ClientIP == "" {
		return nil, fmt.Errorf("client_ip is required for Namecheap API")
	}

	baseURL := "https://api.namecheap.com/xml.response"
	if config.Sandbox {
		baseURL = "https://api.sandbox.namecheap.com/xml.response"
	}

	return &NamecheapProvider{
		apiUser:  config.APIUser,
		apiKey:   config.APIKey,
		username: config.Username,
		clientIP: config.ClientIP,
		baseURL:  baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// CreateRecord creates a DNS record in Namecheap
func (np *NamecheapProvider) CreateRecord(ctx context.Context, record Record) error {
	domain := extractDomain(record.Hostname)
	subdomain := extractSubdomain(record.Hostname, domain)

	// Get existing records
	existingRecords, err := np.getHostRecords(ctx, domain)
	if err != nil {
		return fmt.Errorf("failed to get existing records: %w", err)
	}

	// Check if record already exists
	for _, existing := range existingRecords {
		if existing.Name == subdomain && existing.Type == record.Type {
			return fmt.Errorf("record already exists: %s (%s)", record.Hostname, record.Type)
		}
	}

	// Add new record to existing records
	newRecord := NamecheapHostRecord{
		Name:    subdomain,
		Type:    record.Type,
		Address: record.Value,
		TTL:     record.TTL,
	}

	if record.Priority > 0 && record.Type == "MX" {
		newRecord.MXPref = fmt.Sprintf("%d", record.Priority)
	}

	existingRecords = append(existingRecords, newRecord)

	// Update all records (Namecheap requires setting all records at once)
	if err := np.setHostRecords(ctx, domain, existingRecords); err != nil {
		return fmt.Errorf("failed to create record: %w", err)
	}

	log.Printf("Created Namecheap DNS record: %s (%s)", record.Hostname, record.Type)
	return nil
}

// UpdateRecord updates an existing DNS record
func (np *NamecheapProvider) UpdateRecord(ctx context.Context, record Record) error {
	domain := extractDomain(record.Hostname)
	subdomain := extractSubdomain(record.Hostname, domain)

	// Get existing records
	existingRecords, err := np.getHostRecords(ctx, domain)
	if err != nil {
		return fmt.Errorf("failed to get existing records: %w", err)
	}

	// Find and update the record
	found := false
	for i, existing := range existingRecords {
		if existing.Name == subdomain && existing.Type == record.Type {
			existingRecords[i].Address = record.Value
			existingRecords[i].TTL = record.TTL
			if record.Priority > 0 && record.Type == "MX" {
				existingRecords[i].MXPref = fmt.Sprintf("%d", record.Priority)
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("record not found for update: %s (%s)", record.Hostname, record.Type)
	}

	// Update all records
	if err := np.setHostRecords(ctx, domain, existingRecords); err != nil {
		return fmt.Errorf("failed to update record: %w", err)
	}

	log.Printf("Updated Namecheap DNS record: %s (%s)", record.Hostname, record.Type)
	return nil
}

// DeleteRecord deletes a DNS record
func (np *NamecheapProvider) DeleteRecord(ctx context.Context, hostname string, recordType string) error {
	domain := extractDomain(hostname)
	subdomain := extractSubdomain(hostname, domain)

	// Get existing records
	existingRecords, err := np.getHostRecords(ctx, domain)
	if err != nil {
		return fmt.Errorf("failed to get existing records: %w", err)
	}

	// Filter out the record to delete
	var filteredRecords []NamecheapHostRecord
	found := false
	for _, existing := range existingRecords {
		if existing.Name == subdomain && existing.Type == recordType {
			found = true
			continue // Skip this record (delete it)
		}
		filteredRecords = append(filteredRecords, existing)
	}

	if !found {
		log.Printf("Record not found for deletion: %s (%s)", hostname, recordType)
		return nil // Not an error if record doesn't exist
	}

	// Update with filtered records
	if err := np.setHostRecords(ctx, domain, filteredRecords); err != nil {
		return fmt.Errorf("failed to delete record: %w", err)
	}

	log.Printf("Deleted Namecheap DNS record: %s (%s)", hostname, recordType)
	return nil
}

// GetRecord retrieves a specific DNS record
func (np *NamecheapProvider) GetRecord(ctx context.Context, hostname string, recordType string) (*Record, error) {
	domain := extractDomain(hostname)
	subdomain := extractSubdomain(hostname, domain)

	// Get existing records
	existingRecords, err := np.getHostRecords(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing records: %w", err)
	}

	// Find the record
	for _, existing := range existingRecords {
		if existing.Name == subdomain && existing.Type == recordType {
			record := &Record{
				Hostname: hostname,
				Type:     existing.Type,
				Value:    existing.Address,
				TTL:      existing.TTL,
			}

			if existing.MXPref != "" && recordType == "MX" {
				// Parse MX priority
				var priority int
				if _, err := fmt.Sscanf(existing.MXPref, "%d", &priority); err == nil {
					record.Priority = priority
				}
			}

			return record, nil
		}
	}

	return nil, nil // Record not found
}

// ListRecords lists all DNS records for a domain
func (np *NamecheapProvider) ListRecords(ctx context.Context, domain string) ([]Record, error) {
	hostRecords, err := np.getHostRecords(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to get host records: %w", err)
	}

	var records []Record
	for _, hostRecord := range hostRecords {
		hostname := hostRecord.Name
		if hostname != "@" && hostname != "" {
			hostname = fmt.Sprintf("%s.%s", hostRecord.Name, domain)
		} else {
			hostname = domain
		}

		record := Record{
			Hostname: hostname,
			Type:     hostRecord.Type,
			Value:    hostRecord.Address,
			TTL:      hostRecord.TTL,
		}

		if hostRecord.MXPref != "" && hostRecord.Type == "MX" {
			var priority int
			if _, err := fmt.Sscanf(hostRecord.MXPref, "%d", &priority); err == nil {
				record.Priority = priority
			}
		}

		records = append(records, record)
	}

	return records, nil
}

// CreateWildcardRecord creates a wildcard DNS record
func (np *NamecheapProvider) CreateWildcardRecord(ctx context.Context, domain string, target string) error {
	wildcardHost := fmt.Sprintf("*.%s", domain)

	record := Record{
		Hostname: wildcardHost,
		Type:     RecordTypeA,
		Value:    target,
		TTL:      300, // 5 minutes default
	}

	// Check if target is an IP or hostname
	if strings.Contains(target, ".") && !isIPAddress(target) {
		// It's a hostname, create CNAME instead
		record.Type = RecordTypeCNAME
	}

	return np.CreateRecord(ctx, record)
}

// ValidateConfiguration validates the Namecheap provider configuration
func (np *NamecheapProvider) ValidateConfiguration() error {
	ctx := context.Background()

	// Test API credentials by making a simple API call
	params := url.Values{}
	params.Set("ApiUser", np.apiUser)
	params.Set("ApiKey", np.apiKey)
	params.Set("UserName", np.username)
	params.Set("Command", "namecheap.users.getBalances")
	params.Set("ClientIp", np.clientIP)

	resp, err := np.makeRequest(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to validate credentials: %w", err)
	}

	if resp.Status != "OK" {
		if len(resp.Errors.Error) > 0 {
			return fmt.Errorf("invalid credentials: %s", resp.Errors.Error[0].Description)
		}
		return fmt.Errorf("invalid credentials")
	}

	log.Printf("Namecheap provider configuration validated successfully")
	return nil
}

// getHostRecords retrieves all host records for a domain
func (np *NamecheapProvider) getHostRecords(ctx context.Context, domain string) ([]NamecheapHostRecord, error) {
	sld, tld := splitDomain(domain)

	params := url.Values{}
	params.Set("ApiUser", np.apiUser)
	params.Set("ApiKey", np.apiKey)
	params.Set("UserName", np.username)
	params.Set("Command", "namecheap.domains.dns.getHosts")
	params.Set("ClientIp", np.clientIP)
	params.Set("SLD", sld)
	params.Set("TLD", tld)

	resp, err := np.makeRequest(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get host records: %w", err)
	}

	if resp.Status != "OK" {
		if len(resp.Errors.Error) > 0 {
			return nil, fmt.Errorf("API error: %s", resp.Errors.Error[0].Description)
		}
		return nil, fmt.Errorf("API error")
	}

	// Check if we have the expected response
	if resp.CommandResponse.DomainDNSGetHostsResult == nil {
		// Log the raw response for debugging
		log.Printf("Unexpected response structure. Raw CommandResponse: %s", resp.CommandResponse.Raw)
		return nil, fmt.Errorf("unexpected response structure: no DomainDNSGetHostsResult")
	}

	return resp.CommandResponse.DomainDNSGetHostsResult.Host, nil
}

// setHostRecords sets all host records for a domain
func (np *NamecheapProvider) setHostRecords(ctx context.Context, domain string, records []NamecheapHostRecord) error {
	sld, tld := splitDomain(domain)

	params := url.Values{}
	params.Set("ApiUser", np.apiUser)
	params.Set("ApiKey", np.apiKey)
	params.Set("UserName", np.username)
	params.Set("Command", "namecheap.domains.dns.setHosts")
	params.Set("ClientIp", np.clientIP)
	params.Set("SLD", sld)
	params.Set("TLD", tld)

	// Add host records as parameters
	for i, record := range records {
		prefix := fmt.Sprintf("HostName%d", i+1)
		params.Set(prefix, record.Name)
		params.Set(fmt.Sprintf("RecordType%d", i+1), record.Type)
		params.Set(fmt.Sprintf("Address%d", i+1), record.Address)
		if record.TTL > 0 {
			params.Set(fmt.Sprintf("TTL%d", i+1), fmt.Sprintf("%d", record.TTL))
		}
		if record.MXPref != "" && record.Type == "MX" {
			params.Set(fmt.Sprintf("MXPref%d", i+1), record.MXPref)
		}
	}

	resp, err := np.makeRequest(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to set host records: %w", err)
	}

	if resp.Status != "OK" {
		if len(resp.Errors.Error) > 0 {
			return fmt.Errorf("API error: %s", resp.Errors.Error[0].Description)
		}
		return fmt.Errorf("API error")
	}

	return nil
}

// makeRequest makes an HTTP request to Namecheap API
func (np *NamecheapProvider) makeRequest(ctx context.Context, params url.Values) (*NamecheapResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", np.baseURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := np.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Debug: Log raw response
	log.Printf("Namecheap API raw response: %s", string(body))

	var ncResp NamecheapResponse
	if err := xml.Unmarshal(body, &ncResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &ncResp, nil
}

// Helper functions

// extractDomain extracts the domain from a hostname (e.g., "app.example.com" -> "example.com")
func extractDomain(hostname string) string {
	parts := strings.Split(hostname, ".")
	if len(parts) < 2 {
		return hostname
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// extractSubdomain extracts the subdomain from a hostname (e.g., "app.example.com" -> "app")
func extractSubdomain(hostname, domain string) string {
	if hostname == domain {
		return "@"
	}
	subdomain := strings.TrimSuffix(hostname, "."+domain)
	if subdomain == "" {
		return "@"
	}
	return subdomain
}

// splitDomain splits a domain into SLD and TLD (e.g., "example.com" -> "example", "com")
func splitDomain(domain string) (string, string) {
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return domain, ""
	}
	return strings.Join(parts[:len(parts)-1], "."), parts[len(parts)-1]
}
