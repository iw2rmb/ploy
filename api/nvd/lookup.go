package nvd

// moved from nvd_lookup.go

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/iw2rmb/ploy/api/arf"
)

// LookupCVE retrieves detailed information for a specific CVE
func (n *NVDDatabase) LookupCVE(cveID string) (*arf.CVEInfo, error) {
	// Check cache first
	if cached, exists := n.cache[cveID]; exists {
		return cached, nil
	}

	// Build request URL
	url := fmt.Sprintf("%s?cveId=%s", n.baseURL, cveID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add API key if available
	if n.apiKey != "" {
		req.Header.Set("apiKey", n.apiKey)
	}

	// Make request
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("NVD API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NVD API returned status %d", resp.StatusCode)
	}

	// Parse response
	var nvdResp NVDResponse
	if err := json.NewDecoder(resp.Body).Decode(&nvdResp); err != nil {
		return nil, fmt.Errorf("failed to decode NVD response: %w", err)
	}

	if len(nvdResp.Vulnerabilities) == 0 {
		return nil, fmt.Errorf("CVE %s not found", cveID)
	}

	// Convert to CVEInfo
	cveInfo, err := n.convertToCVEInfo(NVDVulnerability(nvdResp.Vulnerabilities[0]))
	if err != nil {
		return nil, fmt.Errorf("failed to convert CVE data: %w", err)
	}

	// Cache result
	n.cache[cveID] = cveInfo

	return cveInfo, nil
}
