package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// NVDDatabase implements CVEDatabase using the National Vulnerability Database
type NVDDatabase struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	cache      map[string]*CVEInfo
}

// NVDResponse represents the NVD API response structure
type NVDResponse struct {
	ResultsPerPage  int `json:"resultsPerPage"`
	StartIndex      int `json:"startIndex"`
	TotalResults    int `json:"totalResults"`
	Format          string `json:"format"`
	Version         string `json:"version"`
	Timestamp       string `json:"timestamp"`
	Vulnerabilities []struct {
		CVE struct {
			ID                 string `json:"id"`
			SourceIdentifier   string `json:"sourceIdentifier"`
			VulnStatus         string `json:"vulnStatus"`
			Published          string `json:"published"`
			LastModified       string `json:"lastModified"`
			EvaluatorComment   string `json:"evaluatorComment,omitempty"`
			EvaluatorSolution  string `json:"evaluatorSolution,omitempty"`
			EvaluatorImpact    string `json:"evaluatorImpact,omitempty"`
			CISAExploitAdd     string `json:"cisaExploitAdd,omitempty"`
			CISAActionDue      string `json:"cisaActionDue,omitempty"`
			CISARequiredAction string `json:"cisaRequiredAction,omitempty"`
			CISAVulnName       string `json:"cisaVulnName,omitempty"`
			Descriptions       []struct {
				Lang  string `json:"lang"`
				Value string `json:"value"`
			} `json:"descriptions"`
			References []struct {
				URL    string   `json:"url"`
				Source string   `json:"source"`
				Tags   []string `json:"tags,omitempty"`
			} `json:"references"`
			Metrics struct {
				CvssMetricV31 []struct {
					Source   string `json:"source"`
					Type     string `json:"type"`
					CvssData struct {
						Version                    string  `json:"version"`
						VectorString               string  `json:"vectorString"`
						AttackVector               string  `json:"attackVector"`
						AttackComplexity           string  `json:"attackComplexity"`
						PrivilegesRequired         string  `json:"privilegesRequired"`
						UserInteraction            string  `json:"userInteraction"`
						Scope                      string  `json:"scope"`
						ConfidentialityImpact      string  `json:"confidentialityImpact"`
						IntegrityImpact            string  `json:"integrityImpact"`
						AvailabilityImpact         string  `json:"availabilityImpact"`
						BaseScore                  float64 `json:"baseScore"`
						BaseSeverity               string  `json:"baseSeverity"`
						ExploitabilityScore        float64 `json:"exploitabilityScore"`
						ImpactScore                float64 `json:"impactScore"`
					} `json:"cvssData"`
					ExploitabilityScore float64 `json:"exploitabilityScore"`
					ImpactScore         float64 `json:"impactScore"`
				} `json:"cvssMetricV31"`
				CvssMetricV30 []struct {
					Source   string `json:"source"`
					Type     string `json:"type"`
					CvssData struct {
						Version                    string  `json:"version"`
						VectorString               string  `json:"vectorString"`
						AttackVector               string  `json:"attackVector"`
						AttackComplexity           string  `json:"attackComplexity"`
						PrivilegesRequired         string  `json:"privilegesRequired"`
						UserInteraction            string  `json:"userInteraction"`
						Scope                      string  `json:"scope"`
						ConfidentialityImpact      string  `json:"confidentialityImpact"`
						IntegrityImpact            string  `json:"integrityImpact"`
						AvailabilityImpact         string  `json:"availabilityImpact"`
						BaseScore                  float64 `json:"baseScore"`
						BaseSeverity               string  `json:"baseSeverity"`
						ExploitabilityScore        float64 `json:"exploitabilityScore"`
						ImpactScore                float64 `json:"impactScore"`
					} `json:"cvssData"`
					ExploitabilityScore float64 `json:"exploitabilityScore"`
					ImpactScore         float64 `json:"impactScore"`
				} `json:"cvssMetricV30"`
				CvssMetricV2 []struct {
					Source   string `json:"source"`
					Type     string `json:"type"`
					CvssData struct {
						Version               string  `json:"version"`
						VectorString          string  `json:"vectorString"`
						AccessVector          string  `json:"accessVector"`
						AccessComplexity      string  `json:"accessComplexity"`
						Authentication        string  `json:"authentication"`
						ConfidentialityImpact string  `json:"confidentialityImpact"`
						IntegrityImpact       string  `json:"integrityImpact"`
						AvailabilityImpact    string  `json:"availabilityImpact"`
						BaseScore             float64 `json:"baseScore"`
					} `json:"cvssData"`
					BaseSeverity        string  `json:"baseSeverity"`
					ExploitabilityScore float64 `json:"exploitabilityScore"`
					ImpactScore         float64 `json:"impactScore"`
					AcInsufInfo         bool    `json:"acInsufInfo"`
					ObtainAllPrivilege  bool    `json:"obtainAllPrivilege"`
					ObtainUserPrivilege bool    `json:"obtainUserPrivilege"`
					ObtainOtherPrivilege bool   `json:"obtainOtherPrivilege"`
					UserInteractionRequired bool `json:"userInteractionRequired"`
				} `json:"cvssMetricV2"`
			} `json:"metrics"`
			Weaknesses []struct {
				Source      string `json:"source"`
				Type        string `json:"type"`
				Description []struct {
					Lang  string `json:"lang"`
					Value string `json:"value"`
				} `json:"description"`
			} `json:"weaknesses"`
			Configurations []struct {
				Nodes []struct {
					Operator string `json:"operator"`
					Negate   bool   `json:"negate"`
					CpeMatch []struct {
						Vulnerable                bool   `json:"vulnerable"`
						Criteria                  string `json:"criteria"`
						VersionStartIncluding     string `json:"versionStartIncluding,omitempty"`
						VersionStartExcluding     string `json:"versionStartExcluding,omitempty"`
						VersionEndIncluding       string `json:"versionEndIncluding,omitempty"`
						VersionEndExcluding       string `json:"versionEndExcluding,omitempty"`
						MatchCriteriaId           string `json:"matchCriteriaId"`
					} `json:"cpeMatch"`
				} `json:"nodes"`
			} `json:"configurations"`
			VendorComments []struct {
				Organization string `json:"organization"`
				Comment      string `json:"comment"`
				LastModified string `json:"lastModified"`
			} `json:"vendorComments,omitempty"`
		} `json:"cve"`
	} `json:"vulnerabilities"`
}

// NewNVDDatabase creates a new NVD database client
func NewNVDDatabase() *NVDDatabase {
	return &NVDDatabase{
		baseURL: "https://services.nvd.nist.gov/rest/json/cves/2.0",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: make(map[string]*CVEInfo),
	}
}

// SetAPIKey sets the NVD API key for enhanced rate limits
func (n *NVDDatabase) SetAPIKey(apiKey string) {
	n.apiKey = apiKey
}

// LookupCVE retrieves detailed information for a specific CVE
func (n *NVDDatabase) LookupCVE(cveID string) (*CVEInfo, error) {
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
	defer resp.Body.Close()
	
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
	cveInfo, err := n.convertToCVEInfo(nvdResp.Vulnerabilities[0])
	if err != nil {
		return nil, fmt.Errorf("failed to convert CVE data: %w", err)
	}
	
	// Cache result
	n.cache[cveID] = cveInfo
	
	return cveInfo, nil
}

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

// UpdateDatabase updates the local CVE database
func (n *NVDDatabase) UpdateDatabase(ctx context.Context) error {
	// For NVD, we don't maintain a local database - we query in real time
	// This method could be used to update cached data or local mirrors
	return nil
}

// convertToCVEInfo converts NVD vulnerability data to CVEInfo
func (n *NVDDatabase) convertToCVEInfo(nvdVuln struct {
	CVE struct {
		ID                 string `json:"id"`
		SourceIdentifier   string `json:"sourceIdentifier"`
		VulnStatus         string `json:"vulnStatus"`
		Published          string `json:"published"`
		LastModified       string `json:"lastModified"`
		EvaluatorComment   string `json:"evaluatorComment,omitempty"`
		EvaluatorSolution  string `json:"evaluatorSolution,omitempty"`
		EvaluatorImpact    string `json:"evaluatorImpact,omitempty"`
		CISAExploitAdd     string `json:"cisaExploitAdd,omitempty"`
		CISAActionDue      string `json:"cisaActionDue,omitempty"`
		CISARequiredAction string `json:"cisaRequiredAction,omitempty"`
		CISAVulnName       string `json:"cisaVulnName,omitempty"`
		Descriptions       []struct {
			Lang  string `json:"lang"`
			Value string `json:"value"`
		} `json:"descriptions"`
		References []struct {
			URL    string   `json:"url"`
			Source string   `json:"source"`
			Tags   []string `json:"tags,omitempty"`
		} `json:"references"`
		Metrics struct {
			CvssMetricV31 []struct {
				Source   string `json:"source"`
				Type     string `json:"type"`
				CvssData struct {
					Version                    string  `json:"version"`
					VectorString               string  `json:"vectorString"`
					AttackVector               string  `json:"attackVector"`
					AttackComplexity           string  `json:"attackComplexity"`
					PrivilegesRequired         string  `json:"privilegesRequired"`
					UserInteraction            string  `json:"userInteraction"`
					Scope                      string  `json:"scope"`
					ConfidentialityImpact      string  `json:"confidentialityImpact"`
					IntegrityImpact            string  `json:"integrityImpact"`
					AvailabilityImpact         string  `json:"availabilityImpact"`
					BaseScore                  float64 `json:"baseScore"`
					BaseSeverity               string  `json:"baseSeverity"`
					ExploitabilityScore        float64 `json:"exploitabilityScore"`
					ImpactScore                float64 `json:"impactScore"`
				} `json:"cvssData"`
				ExploitabilityScore float64 `json:"exploitabilityScore"`
				ImpactScore         float64 `json:"impactScore"`
			} `json:"cvssMetricV31"`
			CvssMetricV30 []struct {
				Source   string `json:"source"`
				Type     string `json:"type"`
				CvssData struct {
					Version                    string  `json:"version"`
					VectorString               string  `json:"vectorString"`
					AttackVector               string  `json:"attackVector"`
					AttackComplexity           string  `json:"attackComplexity"`
					PrivilegesRequired         string  `json:"privilegesRequired"`
					UserInteraction            string  `json:"userInteraction"`
					Scope                      string  `json:"scope"`
					ConfidentialityImpact      string  `json:"confidentialityImpact"`
					IntegrityImpact            string  `json:"integrityImpact"`
					AvailabilityImpact         string  `json:"availabilityImpact"`
					BaseScore                  float64 `json:"baseScore"`
					BaseSeverity               string  `json:"baseSeverity"`
					ExploitabilityScore        float64 `json:"exploitabilityScore"`
					ImpactScore                float64 `json:"impactScore"`
				} `json:"cvssData"`
				ExploitabilityScore float64 `json:"exploitabilityScore"`
				ImpactScore         float64 `json:"impactScore"`
			} `json:"cvssMetricV30"`
			CvssMetricV2 []struct {
				Source   string `json:"source"`
				Type     string `json:"type"`
				CvssData struct {
					Version               string  `json:"version"`
					VectorString          string  `json:"vectorString"`
					AccessVector          string  `json:"accessVector"`
					AccessComplexity      string  `json:"accessComplexity"`
					Authentication        string  `json:"authentication"`
					ConfidentialityImpact string  `json:"confidentialityImpact"`
					IntegrityImpact       string  `json:"integrityImpact"`
					AvailabilityImpact    string  `json:"availabilityImpact"`
					BaseScore             float64 `json:"baseScore"`
				} `json:"cvssData"`
				BaseSeverity        string  `json:"baseSeverity"`
				ExploitabilityScore float64 `json:"exploitabilityScore"`
				ImpactScore         float64 `json:"impactScore"`
				AcInsufInfo         bool    `json:"acInsufInfo"`
				ObtainAllPrivilege  bool    `json:"obtainAllPrivilege"`
				ObtainUserPrivilege bool    `json:"obtainUserPrivilege"`
				ObtainOtherPrivilege bool   `json:"obtainOtherPrivilege"`
				UserInteractionRequired bool `json:"userInteractionRequired"`
			} `json:"cvssMetricV2"`
		} `json:"metrics"`
		Weaknesses []struct {
			Source      string `json:"source"`
			Type        string `json:"type"`
			Description []struct {
				Lang  string `json:"lang"`
				Value string `json:"value"`
			} `json:"description"`
		} `json:"weaknesses"`
		Configurations []struct {
			Nodes []struct {
				Operator string `json:"operator"`
				Negate   bool   `json:"negate"`
				CpeMatch []struct {
					Vulnerable                bool   `json:"vulnerable"`
					Criteria                  string `json:"criteria"`
					VersionStartIncluding     string `json:"versionStartIncluding,omitempty"`
					VersionStartExcluding     string `json:"versionStartExcluding,omitempty"`
					VersionEndIncluding       string `json:"versionEndIncluding,omitempty"`
					VersionEndExcluding       string `json:"versionEndExcluding,omitempty"`
					MatchCriteriaId           string `json:"matchCriteriaId"`
				} `json:"cpeMatch"`
			} `json:"nodes"`
		} `json:"configurations"`
		VendorComments []struct {
			Organization string `json:"organization"`
			Comment      string `json:"comment"`
			LastModified string `json:"lastModified"`
		} `json:"vendorComments,omitempty"`
	} `json:"cve"`
}) (*CVEInfo, error) {
	cve := nvdVuln.CVE
	
	// Parse description
	var description string
	for _, desc := range cve.Descriptions {
		if desc.Lang == "en" {
			description = desc.Value
			break
		}
	}
	if description == "" && len(cve.Descriptions) > 0 {
		description = cve.Descriptions[0].Value
	}
	
	// Parse references
	refs := make([]CVEReference, len(cve.References))
	for i, ref := range cve.References {
		refs[i] = CVEReference{
			Type: "external",
			URL:  ref.URL,
			Tags: ref.Tags,
		}
	}
	
	// Parse published date
	publishedDate, _ := time.Parse("2006-01-02T15:04:05.000Z", cve.Published)
	
	// Determine CVSS score and version
	var cvssScore CVSSScore
	var severity string
	
	// Prefer CVSS v3.1, then v3.0, then v2
	if len(cve.Metrics.CvssMetricV31) > 0 {
		cvss := cve.Metrics.CvssMetricV31[0]
		cvssScore = CVSSScore{
			Version:        cvss.CvssData.Version,
			BaseScore:      cvss.CvssData.BaseScore,
			Vector:         cvss.CvssData.VectorString,
			Impact:         cvss.ImpactScore,
			Exploitability: cvss.ExploitabilityScore,
		}
		severity = cvss.CvssData.BaseSeverity
	} else if len(cve.Metrics.CvssMetricV30) > 0 {
		cvss := cve.Metrics.CvssMetricV30[0]
		cvssScore = CVSSScore{
			Version:        cvss.CvssData.Version,
			BaseScore:      cvss.CvssData.BaseScore,
			Vector:         cvss.CvssData.VectorString,
			Impact:         cvss.ImpactScore,
			Exploitability: cvss.ExploitabilityScore,
		}
		severity = cvss.CvssData.BaseSeverity
	} else if len(cve.Metrics.CvssMetricV2) > 0 {
		cvss := cve.Metrics.CvssMetricV2[0]
		cvssScore = CVSSScore{
			Version:        cvss.CvssData.Version,
			BaseScore:      cvss.CvssData.BaseScore,
			Vector:         cvss.CvssData.VectorString,
			Impact:         cvss.ImpactScore,
			Exploitability: cvss.ExploitabilityScore,
		}
		severity = cvss.BaseSeverity
	}
	
	// Parse affected packages from configurations
	var affectedPackages []AffectedPackage
	for _, config := range cve.Configurations {
		for _, node := range config.Nodes {
			for _, cpeMatch := range node.CpeMatch {
				if cpeMatch.Vulnerable {
					pkg := n.parseCPEToPackage(cpeMatch.Criteria)
					if pkg != nil {
						affectedPackages = append(affectedPackages, *pkg)
					}
				}
			}
		}
	}
	
	// Determine exploitability
	hasExploit := cve.CISAExploitAdd != ""
	exploitability := ExploitabilityInfo{
		HasExploit:      hasExploit,
		ExploitMaturity: "unknown",
		AttackVector:    "network",
		AttackComplexity: "low",
	}
	
	// Generate remediation guidance
	remediation := n.generateRemediationGuidance(cve, affectedPackages)
	
	cveInfo := &CVEInfo{
		ID:               cve.ID,
		Description:      description,
		CVSS:            cvssScore,
		AffectedPackages: affectedPackages,
		References:      refs,
		PublishedDate:   publishedDate,
		Severity:        severity,
		Remediation:     remediation,
		Exploitability:  exploitability,
		Metadata: map[string]interface{}{
			"source_identifier": cve.SourceIdentifier,
			"vuln_status":       cve.VulnStatus,
			"last_modified":     cve.LastModified,
		},
	}
	
	return cveInfo, nil
}

// parseCPEToPackage converts a CPE string to a package structure
func (n *NVDDatabase) parseCPEToPackage(cpe string) *AffectedPackage {
	// Simple CPE parsing - in practice, this would be more comprehensive
	// CPE format: cpe:2.3:a:vendor:product:version:update:edition:language:sw_edition:target_sw:target_hw:other
	parts := strings.Split(cpe, ":")
	if len(parts) < 5 {
		return nil
	}
	
	vendor := parts[3]
	product := parts[4]
	version := "*"
	if len(parts) > 5 && parts[5] != "*" {
		version = parts[5]
	}
	
	return &AffectedPackage{
		Name:             fmt.Sprintf("%s/%s", vendor, product),
		Ecosystem:        "generic",
		AffectedVersions: []string{version},
		FixedVersions:    []string{}, // Would need additional data source
		PatchAvailable:   false,      // Would need additional analysis
	}
}

// generateRemediationGuidance creates remediation guidance for a CVE
func (n *NVDDatabase) generateRemediationGuidance(cve struct {
	ID                 string `json:"id"`
	SourceIdentifier   string `json:"sourceIdentifier"`
	VulnStatus         string `json:"vulnStatus"`
	Published          string `json:"published"`
	LastModified       string `json:"lastModified"`
	EvaluatorComment   string `json:"evaluatorComment,omitempty"`
	EvaluatorSolution  string `json:"evaluatorSolution,omitempty"`
	EvaluatorImpact    string `json:"evaluatorImpact,omitempty"`
	CISAExploitAdd     string `json:"cisaExploitAdd,omitempty"`
	CISAActionDue      string `json:"cisaActionDue,omitempty"`
	CISARequiredAction string `json:"cisaRequiredAction,omitempty"`
	CISAVulnName       string `json:"cisaVulnName,omitempty"`
	Descriptions       []struct {
		Lang  string `json:"lang"`
		Value string `json:"value"`
	} `json:"descriptions"`
	References []struct {
		URL    string   `json:"url"`
		Source string   `json:"source"`
		Tags   []string `json:"tags,omitempty"`
	} `json:"references"`
	Metrics struct {
		CvssMetricV31 []struct {
			Source   string `json:"source"`
			Type     string `json:"type"`
			CvssData struct {
				Version                    string  `json:"version"`
				VectorString               string  `json:"vectorString"`
				AttackVector               string  `json:"attackVector"`
				AttackComplexity           string  `json:"attackComplexity"`
				PrivilegesRequired         string  `json:"privilegesRequired"`
				UserInteraction            string  `json:"userInteraction"`
				Scope                      string  `json:"scope"`
				ConfidentialityImpact      string  `json:"confidentialityImpact"`
				IntegrityImpact            string  `json:"integrityImpact"`
				AvailabilityImpact         string  `json:"availabilityImpact"`
				BaseScore                  float64 `json:"baseScore"`
				BaseSeverity               string  `json:"baseSeverity"`
				ExploitabilityScore        float64 `json:"exploitabilityScore"`
				ImpactScore                float64 `json:"impactScore"`
			} `json:"cvssData"`
			ExploitabilityScore float64 `json:"exploitabilityScore"`
			ImpactScore         float64 `json:"impactScore"`
		} `json:"cvssMetricV31"`
		CvssMetricV30 []struct {
			Source   string `json:"source"`
			Type     string `json:"type"`
			CvssData struct {
				Version                    string  `json:"version"`
				VectorString               string  `json:"vectorString"`
				AttackVector               string  `json:"attackVector"`
				AttackComplexity           string  `json:"attackComplexity"`
				PrivilegesRequired         string  `json:"privilegesRequired"`
				UserInteraction            string  `json:"userInteraction"`
				Scope                      string  `json:"scope"`
				ConfidentialityImpact      string  `json:"confidentialityImpact"`
				IntegrityImpact            string  `json:"integrityImpact"`
				AvailabilityImpact         string  `json:"availabilityImpact"`
				BaseScore                  float64 `json:"baseScore"`
				BaseSeverity               string  `json:"baseSeverity"`
				ExploitabilityScore        float64 `json:"exploitabilityScore"`
				ImpactScore                float64 `json:"impactScore"`
			} `json:"cvssData"`
			ExploitabilityScore float64 `json:"exploitabilityScore"`
			ImpactScore         float64 `json:"impactScore"`
		} `json:"cvssMetricV30"`
		CvssMetricV2 []struct {
			Source   string `json:"source"`
			Type     string `json:"type"`
			CvssData struct {
				Version               string  `json:"version"`
				VectorString          string  `json:"vectorString"`
				AccessVector          string  `json:"accessVector"`
				AccessComplexity      string  `json:"accessComplexity"`
				Authentication        string  `json:"authentication"`
				ConfidentialityImpact string  `json:"confidentialityImpact"`
				IntegrityImpact       string  `json:"integrityImpact"`
				AvailabilityImpact    string  `json:"availabilityImpact"`
				BaseScore             float64 `json:"baseScore"`
			} `json:"cvssData"`
			BaseSeverity        string  `json:"baseSeverity"`
			ExploitabilityScore float64 `json:"exploitabilityScore"`
			ImpactScore         float64 `json:"impactScore"`
			AcInsufInfo         bool    `json:"acInsufInfo"`
			ObtainAllPrivilege  bool    `json:"obtainAllPrivilege"`
			ObtainUserPrivilege bool    `json:"obtainUserPrivilege"`
			ObtainOtherPrivilege bool   `json:"obtainOtherPrivilege"`
			UserInteractionRequired bool `json:"userInteractionRequired"`
		} `json:"cvssMetricV2"`
	} `json:"metrics"`
	Weaknesses []struct {
		Source      string `json:"source"`
		Type        string `json:"type"`
		Description []struct {
			Lang  string `json:"lang"`
			Value string `json:"value"`
		} `json:"description"`
	} `json:"weaknesses"`
	Configurations []struct {
		Nodes []struct {
			Operator string `json:"operator"`
			Negate   bool   `json:"negate"`
			CpeMatch []struct {
				Vulnerable                bool   `json:"vulnerable"`
				Criteria                  string `json:"criteria"`
				VersionStartIncluding     string `json:"versionStartIncluding,omitempty"`
				VersionStartExcluding     string `json:"versionStartExcluding,omitempty"`
				VersionEndIncluding       string `json:"versionEndIncluding,omitempty"`
				VersionEndExcluding       string `json:"versionEndExcluding,omitempty"`
				MatchCriteriaId           string `json:"matchCriteriaId"`
			} `json:"cpeMatch"`
		} `json:"nodes"`
	} `json:"configurations"`
	VendorComments []struct {
		Organization string `json:"organization"`
		Comment      string `json:"comment"`
		LastModified string `json:"lastModified"`
	} `json:"vendorComments,omitempty"`
}, affectedPackages []AffectedPackage) RemediationGuidance {
	remediationType := "upgrade"
	instructions := "Update affected components to latest secure versions"
	autoApplicable := true
	confidence := 0.7
	
	// Use evaluator solution if available
	if cve.EvaluatorSolution != "" {
		instructions = cve.EvaluatorSolution
		confidence = 0.9
	}
	
	// Use CISA required action if available
	if cve.CISARequiredAction != "" {
		instructions = cve.CISARequiredAction
		confidence = 1.0
	}
	
	// Determine if auto-applicable based on references and tags
	for _, ref := range cve.References {
		for _, tag := range ref.Tags {
			if tag == "Patch" || tag == "Vendor Advisory" {
				autoApplicable = true
				confidence = 0.8
				break
			}
		}
	}
	
	// Estimate effort based on CVSS score and complexity
	effort := EstimatedEffort{
		Level:       "medium",
		TimeMinutes: 60,
		Complexity:  5,
		Risk:        "medium",
		Resources:   []string{"development", "testing"},
	}
	
	if len(cve.Metrics.CvssMetricV31) > 0 && cve.Metrics.CvssMetricV31[0].CvssData.BaseScore >= 7.0 {
		effort.Level = "high"
		effort.TimeMinutes = 120
		effort.Complexity = 7
		effort.Risk = "high"
	}
	
	return RemediationGuidance{
		Type:           remediationType,
		Instructions:   instructions,
		AutoApplicable: autoApplicable,
		Confidence:     confidence,
		Effort:         effort,
	}
}

// mapCVSSToSeverity maps CVSS scores to severity levels for NVD API
func (n *NVDDatabase) mapCVSSToSeverity(score float64) string {
	switch {
	case score >= 9.0:
		return "CRITICAL"
	case score >= 7.0:
		return "HIGH"
	case score >= 4.0:
		return "MEDIUM"
	default:
		return "LOW"
	}
}