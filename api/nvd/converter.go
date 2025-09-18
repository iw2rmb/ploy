package nvd

// moved from nvd_converter.go

import (
	"fmt"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/security"
)

// NVDVulnerability represents a single vulnerability from NVD response
type NVDVulnerability struct {
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
					Version               string  `json:"version"`
					VectorString          string  `json:"vectorString"`
					AttackVector          string  `json:"attackVector"`
					AttackComplexity      string  `json:"attackComplexity"`
					PrivilegesRequired    string  `json:"privilegesRequired"`
					UserInteraction       string  `json:"userInteraction"`
					Scope                 string  `json:"scope"`
					ConfidentialityImpact string  `json:"confidentialityImpact"`
					IntegrityImpact       string  `json:"integrityImpact"`
					AvailabilityImpact    string  `json:"availabilityImpact"`
					BaseScore             float64 `json:"baseScore"`
					BaseSeverity          string  `json:"baseSeverity"`
					ExploitabilityScore   float64 `json:"exploitabilityScore"`
					ImpactScore           float64 `json:"impactScore"`
				} `json:"cvssData"`
				ExploitabilityScore float64 `json:"exploitabilityScore"`
				ImpactScore         float64 `json:"impactScore"`
			} `json:"cvssMetricV31"`
			CvssMetricV30 []struct {
				Source   string `json:"source"`
				Type     string `json:"type"`
				CvssData struct {
					Version               string  `json:"version"`
					VectorString          string  `json:"vectorString"`
					AttackVector          string  `json:"attackVector"`
					AttackComplexity      string  `json:"attackComplexity"`
					PrivilegesRequired    string  `json:"privilegesRequired"`
					UserInteraction       string  `json:"userInteraction"`
					Scope                 string  `json:"scope"`
					ConfidentialityImpact string  `json:"confidentialityImpact"`
					IntegrityImpact       string  `json:"integrityImpact"`
					AvailabilityImpact    string  `json:"availabilityImpact"`
					BaseScore             float64 `json:"baseScore"`
					BaseSeverity          string  `json:"baseSeverity"`
					ExploitabilityScore   float64 `json:"exploitabilityScore"`
					ImpactScore           float64 `json:"impactScore"`
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
				BaseSeverity            string  `json:"baseSeverity"`
				ExploitabilityScore     float64 `json:"exploitabilityScore"`
				ImpactScore             float64 `json:"impactScore"`
				AcInsufInfo             bool    `json:"acInsufInfo"`
				ObtainAllPrivilege      bool    `json:"obtainAllPrivilege"`
				ObtainUserPrivilege     bool    `json:"obtainUserPrivilege"`
				ObtainOtherPrivilege    bool    `json:"obtainOtherPrivilege"`
				UserInteractionRequired bool    `json:"userInteractionRequired"`
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
					Vulnerable            bool   `json:"vulnerable"`
					Criteria              string `json:"criteria"`
					VersionStartIncluding string `json:"versionStartIncluding,omitempty"`
					VersionStartExcluding string `json:"versionStartExcluding,omitempty"`
					VersionEndIncluding   string `json:"versionEndIncluding,omitempty"`
					VersionEndExcluding   string `json:"versionEndExcluding,omitempty"`
					MatchCriteriaId       string `json:"matchCriteriaId"`
				} `json:"cpeMatch"`
			} `json:"nodes"`
		} `json:"configurations"`
		VendorComments []struct {
			Organization string `json:"organization"`
			Comment      string `json:"comment"`
			LastModified string `json:"lastModified"`
		} `json:"vendorComments,omitempty"`
	} `json:"cve"`
}

// convertToCVEInfo converts NVD vulnerability data to CVEInfo
func (n *NVDDatabase) convertToCVEInfo(nvdVuln NVDVulnerability) (*security.CVEInfo, error) {
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
	refs := make([]security.CVEReference, len(cve.References))
	for i, ref := range cve.References {
		refs[i] = security.CVEReference{
			Type: "external",
			URL:  ref.URL,
			Tags: ref.Tags,
		}
	}

	// Parse published date
	publishedDate, _ := time.Parse("2006-01-02T15:04:05.000Z", cve.Published)

	// Determine CVSS score and version
	var cvssScore security.CVSSScore
	var severity string

	// Prefer CVSS v3.1, then v3.0, then v2
	if len(cve.Metrics.CvssMetricV31) > 0 {
		cvss := cve.Metrics.CvssMetricV31[0]
		cvssScore = security.CVSSScore{
			Version:        cvss.CvssData.Version,
			BaseScore:      cvss.CvssData.BaseScore,
			Vector:         cvss.CvssData.VectorString,
			Impact:         cvss.ImpactScore,
			Exploitability: cvss.ExploitabilityScore,
		}
		severity = cvss.CvssData.BaseSeverity
	} else if len(cve.Metrics.CvssMetricV30) > 0 {
		cvss := cve.Metrics.CvssMetricV30[0]
		cvssScore = security.CVSSScore{
			Version:        cvss.CvssData.Version,
			BaseScore:      cvss.CvssData.BaseScore,
			Vector:         cvss.CvssData.VectorString,
			Impact:         cvss.ImpactScore,
			Exploitability: cvss.ExploitabilityScore,
		}
		severity = cvss.CvssData.BaseSeverity
	} else if len(cve.Metrics.CvssMetricV2) > 0 {
		cvss := cve.Metrics.CvssMetricV2[0]
		cvssScore = security.CVSSScore{
			Version:        cvss.CvssData.Version,
			BaseScore:      cvss.CvssData.BaseScore,
			Vector:         cvss.CvssData.VectorString,
			Impact:         cvss.ImpactScore,
			Exploitability: cvss.ExploitabilityScore,
		}
		severity = cvss.BaseSeverity
	}

	// Parse affected packages from configurations
	var affectedPackages []security.AffectedPackage
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
	exploitability := security.ExploitabilityInfo{
		HasExploit:       hasExploit,
		ExploitMaturity:  "unknown",
		AttackVector:     "network",
		AttackComplexity: "low",
	}

	// Generate remediation guidance
	guidance := n.generateRemediationGuidance(NVDCVEInfo(cve), affectedPackages)

	cveInfo := &security.CVEInfo{
		ID:               cve.ID,
		Description:      description,
		CVSS:             cvssScore,
		AffectedPackages: affectedPackages,
		References:       refs,
		PublishedDate:    publishedDate,
		Severity:         severity,
		Remediation:      guidance,
		Exploitability:   exploitability,
		Metadata: map[string]interface{}{
			"source_identifier": cve.SourceIdentifier,
			"vuln_status":       cve.VulnStatus,
			"last_modified":     cve.LastModified,
		},
	}

	return cveInfo, nil
}

// parseCPEToPackage converts a CPE string to a package structure
func (n *NVDDatabase) parseCPEToPackage(cpe string) *security.AffectedPackage {
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

	return &security.AffectedPackage{
		Name:             fmt.Sprintf("%s/%s", vendor, product),
		Ecosystem:        "generic",
		AffectedVersions: []string{version},
		FixedVersions:    []string{}, // Would need additional data source
		PatchAvailable:   false,      // Would need additional analysis
	}
}
