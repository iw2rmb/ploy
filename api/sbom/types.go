package sbom

type Dependency struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Ecosystem string `json:"ecosystem"`
}

type VulnerabilityMatch struct {
	Package    Dependency `json:"package"`
	CVE        string     `json:"cve"`
	Severity   string     `json:"severity"`
	CVSS       float64    `json:"cvss"`
	FixVersion string     `json:"fix_version,omitempty"`
}

type SBOMSecurityMetrics struct {
	TotalDependencies int     `json:"total_dependencies"`
	KnownVulns        int     `json:"known_vulnerabilities"`
	High              int     `json:"high"`
	Medium            int     `json:"medium"`
	Low               int     `json:"low"`
	LicenseIssues     int     `json:"license_issues"`
	Score             float64 `json:"score"`
}

type RiskAssessment struct {
	OverallRisk string  `json:"overall_risk"`
	RiskScore   float64 `json:"risk_score"`
}

type SBOMSecurityAnalysis struct {
	Dependencies    []Dependency         `json:"dependencies"`
	Vulnerabilities []VulnerabilityMatch `json:"vulnerabilities"`
	SecurityMetrics SBOMSecurityMetrics  `json:"security_metrics"`
	RiskAssessment  RiskAssessment       `json:"risk_assessment"`
}

func (s *SyftSBOMAnalyzer) ExtractDependencies(sbomData map[string]interface{}) ([]Dependency, error) {
	var deps []Dependency
	// Syft JSON: artifacts: [{name, version,...}]
	if artsRaw, ok := sbomData["artifacts"].([]interface{}); ok {
		for _, a := range artsRaw {
			if m, ok := a.(map[string]interface{}); ok {
				name, _ := m["name"].(string)
				version, _ := m["version"].(string)
				if name != "" {
					deps = append(deps, Dependency{Name: name, Version: version})
				}
			}
		}
	}
	// CycloneDX JSON: components: [{name, version, purl,...}]
	if compsRaw, ok := sbomData["components"].([]interface{}); ok {
		for _, c := range compsRaw {
			if m, ok := c.(map[string]interface{}); ok {
				name, _ := m["name"].(string)
				version, _ := m["version"].(string)
				if name != "" {
					deps = append(deps, Dependency{Name: name, Version: version})
				}
			}
		}
	}
	// SPDX JSON: packages: [{name, versionInfo,...}]
	if pkgsRaw, ok := sbomData["packages"].([]interface{}); ok {
		for _, p := range pkgsRaw {
			if m, ok := p.(map[string]interface{}); ok {
				name, _ := m["name"].(string)
				// versionInfo may be string or missing
				version, _ := m["versionInfo"].(string)
				if name != "" {
					deps = append(deps, Dependency{Name: name, Version: version})
				}
			}
		}
	}
	return deps, nil
}

func (s *SyftSBOMAnalyzer) CorrelateVulnerabilities(deps []Dependency) ([]VulnerabilityMatch, error) {
	// Minimal stub returning empty correlations; real impl would query vuln DB
	return []VulnerabilityMatch{}, nil
}

func (s *SyftSBOMAnalyzer) CalculateSecurityMetrics(deps []Dependency, vulns []VulnerabilityMatch) SBOMSecurityMetrics {
	metrics := SBOMSecurityMetrics{TotalDependencies: len(deps)}
	for _, v := range vulns {
		metrics.KnownVulns++
		switch v.Severity {
		case "HIGH":
			metrics.High++
		case "MEDIUM":
			metrics.Medium++
		case "LOW":
			metrics.Low++
		}
	}
	return metrics
}

func (s *SyftSBOMAnalyzer) AssessRisk(analysis *SBOMSecurityAnalysis) RiskAssessment {
	// Simple derived score: more vulns => higher risk
	score := 0.0
	score += float64(analysis.SecurityMetrics.High)*2 + float64(analysis.SecurityMetrics.Medium)*1
	overall := "low"
	if score >= 5 {
		overall = "high"
	} else if score >= 2 {
		overall = "medium"
	}
	return RiskAssessment{OverallRisk: overall, RiskScore: score}
}

// EvaluateCompliance provides a coarse compliance status based on metrics
func EvaluateCompliance(metrics SBOMSecurityMetrics) string {
	// Very simple policy: any HIGH => partial, if known vulns > 0 => partial, else compliant
	if metrics.High > 0 {
		return "partial_compliance"
	}
	if metrics.KnownVulns > 0 {
		return "partial_compliance"
	}
	return "compliant"
}
