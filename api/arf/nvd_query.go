package arf

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// QueryVulnerabilities searches for vulnerabilities based on criteria
func (n *NVDDatabase) QueryVulnerabilities(criteria VulnerabilityQuery) ([]VulnerabilityInfo, error) {
	// Build query parameters
	params := make(map[string]string)

	if criteria.PackageName != "" {
		params["keywordSearch"] = criteria.PackageName
	}

	if !criteria.DateRange.From.IsZero() {
		params["pubStartDate"] = criteria.DateRange.From.Format("2006-01-02T15:04:05.000Z")
	}

	if !criteria.DateRange.To.IsZero() {
		params["pubEndDate"] = criteria.DateRange.To.Format("2006-01-02T15:04:05.000Z")
	}

	if criteria.CVSS.Min > 0 {
		params["cvssV3Severity"] = n.mapCVSSToSeverity(criteria.CVSS.Min)
	}

	// Build URL with parameters
	url := n.baseURL
	if len(params) > 0 {
		url += "?"
		var paramPairs []string
		for key, value := range params {
			paramPairs = append(paramPairs, fmt.Sprintf("%s=%s", key, value))
		}
		url += strings.Join(paramPairs, "&")
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if n.apiKey != "" {
		req.Header.Set("apiKey", n.apiKey)
	}

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("NVD API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NVD API returned status %d", resp.StatusCode)
	}

	var nvdResp NVDResponse
	if err := json.NewDecoder(resp.Body).Decode(&nvdResp); err != nil {
		return nil, fmt.Errorf("failed to decode NVD response: %w", err)
	}

	// Convert to VulnerabilityInfo
	vulns := make([]VulnerabilityInfo, 0, len(nvdResp.Vulnerabilities))
	for _, nvdVuln := range nvdResp.Vulnerabilities {
		cveInfo, err := n.convertToCVEInfo(nvdVuln)
		if err != nil {
			continue // Skip problematic entries
		}

		vuln := VulnerabilityInfo{
			CVE:      *cveInfo,
			Severity: cveInfo.Severity,
			CVSS:     cveInfo.CVSS.BaseScore,
			Discovery: VulnerabilityDiscovery{
				Method:     "database_query",
				Tool:       "nvd",
				Timestamp:  time.Now(),
				Confidence: 1.0,
				Source:     "NVD",
			},
		}

		vulns = append(vulns, vuln)
	}

	return vulns, nil
}
