package arf

import "fmt"

func (s *SyftSBOMAnalyzer) assessSBOMCompliance(deps []Dependency, vulns []VulnerabilityMatch, metrics SBOMSecurityMetrics) ComplianceStatus {
	frameworks := map[string]FrameworkCompliance{
		"NIST": {
			Name:    "NIST Secure Software Development Framework",
			Version: "1.1",
			Score:   s.calculateNISTComplianceScore(metrics),
			Status:  "partial_compliance",
		},
		"OWASP": {
			Name:    "OWASP Application Security Verification Standard",
			Version: "4.0",
			Score:   s.calculateOWASPComplianceScore(metrics),
			Status:  "partial_compliance",
		},
	}

	issues := []ComplianceIssue{}
	if metrics.VulnerableDependencies > 0 {
		issues = append(issues, ComplianceIssue{
			Framework:   "NIST",
			Requirement: "Vulnerability Management",
			Description: fmt.Sprintf("%d vulnerable dependencies found", metrics.VulnerableDependencies),
			Severity:    "high",
			Actions:     []string{"Update vulnerable dependencies", "Implement vulnerability scanning"},
		})
	}

	return ComplianceStatus{
		Frameworks: frameworks,
		Overall:    "requires_attention",
		Issues:     issues,
	}
}

func (s *SyftSBOMAnalyzer) calculateNISTComplianceScore(metrics SBOMSecurityMetrics) float64 {
	score := 10.0

	// Deduct points for security issues
	score -= float64(metrics.VulnerableDependencies) * 0.5
	score -= float64(metrics.LicenseIssues) * 0.2
	score -= float64(metrics.OutdatedDependencies) * 0.1

	if score < 0 {
		score = 0
	}

	return score
}

func (s *SyftSBOMAnalyzer) calculateOWASPComplianceScore(metrics SBOMSecurityMetrics) float64 {
	return s.calculateNISTComplianceScore(metrics) // Simplified
}
