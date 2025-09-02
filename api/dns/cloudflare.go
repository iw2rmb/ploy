package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// CloudflareProvider implements DNS provider for Cloudflare
type CloudflareProvider struct {
	apiToken string
	zoneID   string
	baseURL  string
	client   *http.Client
}

// CloudflareConfig holds Cloudflare-specific configuration
type CloudflareConfig struct {
	APIToken string `json:"api_token"`
	ZoneID   string `json:"zone_id"`
	Email    string `json:"email,omitempty"` // Optional, for legacy API key auth
	APIKey   string `json:"api_key,omitempty"` // Optional, for legacy auth
}

// CloudflareRecord represents a DNS record in Cloudflare's API format
type CloudflareRecord struct {
	ID         string    `json:"id,omitempty"`
	Type       string    `json:"type"`
	Name       string    `json:"name"`
	Content    string    `json:"content"`
	TTL        int       `json:"ttl"`
	Priority   int       `json:"priority,omitempty"`
	Proxied    bool      `json:"proxied"`
	CreatedOn  time.Time `json:"created_on,omitempty"`
	ModifiedOn time.Time `json:"modified_on,omitempty"`
}

// CloudflareResponse represents API response
type CloudflareResponse struct {
	Success  bool               `json:"success"`
	Errors   []CloudflareError  `json:"errors"`
	Messages []string           `json:"messages"`
	Result   json.RawMessage    `json:"result"`
}

// CloudflareError represents an API error
type CloudflareError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewCloudflareProvider creates a new Cloudflare DNS provider
func NewCloudflareProvider(config CloudflareConfig) (*CloudflareProvider, error) {
	if config.APIToken == "" && (config.Email == "" || config.APIKey == "") {
		return nil, fmt.Errorf("either api_token or email/api_key pair is required")
	}
	
	if config.ZoneID == "" {
		return nil, fmt.Errorf("zone_id is required")
	}
	
	return &CloudflareProvider{
		apiToken: config.APIToken,
		zoneID:   config.ZoneID,
		baseURL:  "https://api.cloudflare.com/client/v4",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// CreateRecord creates a DNS record in Cloudflare
func (cp *CloudflareProvider) CreateRecord(ctx context.Context, record Record) error {
	cfRecord := CloudflareRecord{
		Type:    record.Type,
		Name:    record.Hostname,
		Content: record.Value,
		TTL:     record.TTL,
		Proxied: false, // Don't proxy DNS-only records by default
	}
	
	if record.Priority > 0 {
		cfRecord.Priority = record.Priority
	}
	
	// Special handling for wildcard records
	if strings.HasPrefix(record.Hostname, "*.") {
		cfRecord.Name = record.Hostname
		// Cloudflare doesn't allow proxying wildcard records
		cfRecord.Proxied = false
	}
	
	body, err := json.Marshal(cfRecord)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}
	
	url := fmt.Sprintf("%s/zones/%s/dns_records", cp.baseURL, cp.zoneID)
	resp, err := cp.makeRequest(ctx, "POST", url, body)
	if err != nil {
		return fmt.Errorf("failed to create record: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create record, status %d: %s", resp.StatusCode, string(body))
	}
	
	log.Printf("Created Cloudflare DNS record: %s (%s)", record.Hostname, record.Type)
	return nil
}

// UpdateRecord updates an existing DNS record
func (cp *CloudflareProvider) UpdateRecord(ctx context.Context, record Record) error {
	// First, find the record ID
	existingRecord, err := cp.GetRecord(ctx, record.Hostname, record.Type)
	if err != nil {
		return fmt.Errorf("failed to find record for update: %w", err)
	}
	
	if existingRecord == nil {
		// Record doesn't exist, create it instead
		return cp.CreateRecord(ctx, record)
	}
	
	// Get the record ID from Cloudflare
	recordID, err := cp.getRecordID(ctx, record.Hostname, record.Type)
	if err != nil {
		return fmt.Errorf("failed to get record ID: %w", err)
	}
	
	cfRecord := CloudflareRecord{
		Type:    record.Type,
		Name:    record.Hostname,
		Content: record.Value,
		TTL:     record.TTL,
		Proxied: false,
	}
	
	body, err := json.Marshal(cfRecord)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}
	
	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", cp.baseURL, cp.zoneID, recordID)
	resp, err := cp.makeRequest(ctx, "PUT", url, body)
	if err != nil {
		return fmt.Errorf("failed to update record: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update record, status %d: %s", resp.StatusCode, string(body))
	}
	
	log.Printf("Updated Cloudflare DNS record: %s (%s)", record.Hostname, record.Type)
	return nil
}

// DeleteRecord deletes a DNS record
func (cp *CloudflareProvider) DeleteRecord(ctx context.Context, hostname string, recordType string) error {
	recordID, err := cp.getRecordID(ctx, hostname, recordType)
	if err != nil {
		// Record might not exist, which is fine for delete
		log.Printf("Record not found for deletion: %s (%s)", hostname, recordType)
		return nil
	}
	
	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", cp.baseURL, cp.zoneID, recordID)
	resp, err := cp.makeRequest(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to delete record: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete record, status %d: %s", resp.StatusCode, string(body))
	}
	
	log.Printf("Deleted Cloudflare DNS record: %s (%s)", hostname, recordType)
	return nil
}

// GetRecord retrieves a specific DNS record
func (cp *CloudflareProvider) GetRecord(ctx context.Context, hostname string, recordType string) (*Record, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records?name=%s&type=%s", cp.baseURL, cp.zoneID, hostname, recordType)
	
	resp, err := cp.makeRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get record: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	var cfResp CloudflareResponse
	if err := json.Unmarshal(body, &cfResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	if !cfResp.Success {
		return nil, fmt.Errorf("API error: %v", cfResp.Errors)
	}
	
	var records []CloudflareRecord
	if err := json.Unmarshal(cfResp.Result, &records); err != nil {
		return nil, fmt.Errorf("failed to parse records: %w", err)
	}
	
	if len(records) == 0 {
		return nil, nil
	}
	
	// Convert to our Record type
	cfRecord := records[0]
	return &Record{
		Hostname:  cfRecord.Name,
		Type:      cfRecord.Type,
		Value:     cfRecord.Content,
		TTL:       cfRecord.TTL,
		Priority:  cfRecord.Priority,
		CreatedAt: cfRecord.CreatedOn,
		UpdatedAt: cfRecord.ModifiedOn,
	}, nil
}

// ListRecords lists all DNS records for a domain
func (cp *CloudflareProvider) ListRecords(ctx context.Context, domain string) ([]Record, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records", cp.baseURL, cp.zoneID)
	if domain != "" {
		url += fmt.Sprintf("?name=%s", domain)
	}
	
	resp, err := cp.makeRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list records: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	var cfResp CloudflareResponse
	if err := json.Unmarshal(body, &cfResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	if !cfResp.Success {
		return nil, fmt.Errorf("API error: %v", cfResp.Errors)
	}
	
	var cfRecords []CloudflareRecord
	if err := json.Unmarshal(cfResp.Result, &cfRecords); err != nil {
		return nil, fmt.Errorf("failed to parse records: %w", err)
	}
	
	// Convert to our Record type
	records := make([]Record, len(cfRecords))
	for i, cfRecord := range cfRecords {
		records[i] = Record{
			Hostname:  cfRecord.Name,
			Type:      cfRecord.Type,
			Value:     cfRecord.Content,
			TTL:       cfRecord.TTL,
			Priority:  cfRecord.Priority,
			CreatedAt: cfRecord.CreatedOn,
			UpdatedAt: cfRecord.ModifiedOn,
		}
	}
	
	return records, nil
}

// CreateWildcardRecord creates a wildcard DNS record
func (cp *CloudflareProvider) CreateWildcardRecord(ctx context.Context, domain string, target string) error {
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
	
	return cp.CreateRecord(ctx, record)
}

// ValidateConfiguration validates the Cloudflare provider configuration
func (cp *CloudflareProvider) ValidateConfiguration() error {
	ctx := context.Background()
	
	// Test API credentials by fetching zone details
	url := fmt.Sprintf("%s/zones/%s", cp.baseURL, cp.zoneID)
	resp, err := cp.makeRequest(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to validate credentials: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("invalid credentials or zone ID, status %d: %s", resp.StatusCode, string(body))
	}
	
	log.Printf("Cloudflare provider configuration validated successfully")
	return nil
}

// getRecordID gets the Cloudflare record ID for a given hostname and type
func (cp *CloudflareProvider) getRecordID(ctx context.Context, hostname string, recordType string) (string, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records?name=%s&type=%s", cp.baseURL, cp.zoneID, hostname, recordType)
	
	resp, err := cp.makeRequest(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get record ID: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	
	var cfResp CloudflareResponse
	if err := json.Unmarshal(body, &cfResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	
	if !cfResp.Success {
		return "", fmt.Errorf("API error: %v", cfResp.Errors)
	}
	
	var records []CloudflareRecord
	if err := json.Unmarshal(cfResp.Result, &records); err != nil {
		return "", fmt.Errorf("failed to parse records: %w", err)
	}
	
	if len(records) == 0 {
		return "", fmt.Errorf("record not found: %s (%s)", hostname, recordType)
	}
	
	return records[0].ID, nil
}

// makeRequest makes an HTTP request to Cloudflare API
func (cp *CloudflareProvider) makeRequest(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Content-Type", "application/json")
	if cp.apiToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cp.apiToken))
	}
	
	return cp.client.Do(req)
}

// isIPAddress checks if a string is an IP address
func isIPAddress(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	
	for _, part := range parts {
		if len(part) == 0 || len(part) > 3 {
			return false
		}
		
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}
	
	return true
}