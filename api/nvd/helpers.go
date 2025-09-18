package nvd

// moved from nvd_helpers.go

import (
	"github.com/iw2rmb/ploy/api/security"
)

// NVDCVEInfo represents CVE information structure for remediation guidance
type NVDCVEInfo struct {
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
}

// generateRemediationGuidance creates remediation guidance for a CVE
func (n *NVDDatabase) generateRemediationGuidance(cve NVDCVEInfo, affectedPackages []security.AffectedPackage) security.RemediationGuidance {
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
	effort := security.EstimatedEffort{
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

	return security.RemediationGuidance{
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
