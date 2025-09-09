package arf

import (
	"fmt"
	"strings"
)

// generateRiskAssessment generates a risk assessment based on the analysis
func (s *SyftSBOMAnalyzer) generateRiskAssessment(deps []Dependency, vulns []VulnerabilityMatch, metrics SBOMSecurityMetrics) RiskAssessment {
	// Count vulnerabilities by severity
	criticalCount := 0
	highCount := 0
	mediumCount := 0
	lowCount := 0

	for _, vuln := range vulns {
		switch strings.ToLower(vuln.Vulnerability.Severity) {
		case "critical":
			criticalCount++
		case "high":
			highCount++
		case "medium":
			mediumCount++
		case "low":
			lowCount++
		}
	}

	// Calculate overall risk score
	riskScore := float64(criticalCount*10+highCount*7+mediumCount*4+lowCount*1) / float64(len(deps))
	if riskScore > 10 {
		riskScore = 10
	}

	// Determine overall risk level
	overallRisk := "low"
	if riskScore >= 8 {
		overallRisk = "critical"
	} else if riskScore >= 6 {
		overallRisk = "high"
	} else if riskScore >= 3 {
		overallRisk = "medium"
	}

	// Generate recommendations
	recommendations := s.generateRiskRecommendations(criticalCount, highCount, mediumCount, lowCount, metrics)

	// Create timeline
	timeline := RemediationTimeline{
		Immediate: []string{},
		Short:     []string{},
		Medium:    []string{},
		Long:      []string{},
	}

	for _, vuln := range vulns {
		cveID := vuln.Vulnerability.CVE.ID
		switch strings.ToLower(vuln.Vulnerability.Severity) {
		case "critical":
			timeline.Immediate = append(timeline.Immediate, cveID)
		case "high":
			timeline.Short = append(timeline.Short, cveID)
		case "medium":
			timeline.Medium = append(timeline.Medium, cveID)
		default:
			timeline.Long = append(timeline.Long, cveID)
		}
	}

	return RiskAssessment{
		OverallRisk:      overallRisk,
		RiskScore:        riskScore,
		CriticalCount:    criticalCount,
		HighCount:        highCount,
		MediumCount:      mediumCount,
		LowCount:         lowCount,
		Recommendations:  recommendations,
		Timeline:         timeline,
		ComplianceStatus: s.assessSBOMCompliance(deps, vulns, metrics),
	}
}

// generateSecurityRecommendations generates actionable security recommendations
func (s *SyftSBOMAnalyzer) generateSecurityRecommendations(deps []Dependency, vulns []VulnerabilityMatch, assessment RiskAssessment) []SecurityRecommendation {
	var recommendations []SecurityRecommendation

	if assessment.CriticalCount > 0 {
		recommendations = append(recommendations, SecurityRecommendation{
			Priority:    1,
			Category:    "vulnerability_management",
			Title:       "Address Critical Vulnerabilities",
			Description: fmt.Sprintf("Found %d critical vulnerabilities requiring immediate remediation", assessment.CriticalCount),
			Action:      "Apply security patches and updates for critical vulnerabilities",
			Impact:      "High - Prevents potential system compromise",
			Effort: EstimatedEffort{
				Level:       "high",
				TimeMinutes: assessment.CriticalCount * 45,
				Complexity:  8,
				Risk:        "high",
				Resources:   []string{"security_team", "development_team"},
			},
			Urgency: "immediate",
		})
	}

	if assessment.HighCount > 0 {
		recommendations = append(recommendations, SecurityRecommendation{
			Priority:    2,
			Category:    "vulnerability_management",
			Title:       "Remediate High-Severity Vulnerabilities",
			Description: fmt.Sprintf("Address %d high-severity vulnerabilities", assessment.HighCount),
			Action:      "Update affected dependencies to patched versions",
			Impact:      "Medium-High - Reduces significant security risks",
			Effort: EstimatedEffort{
				Level:       "medium",
				TimeMinutes: assessment.HighCount * 30,
				Complexity:  6,
				Risk:        "medium",
				Resources:   []string{"development_team"},
			},
			Urgency: "short_term",
		})
	}

	// Recommendation for outdated dependencies
	outdatedCount := 0
	for _, dep := range deps {
		if outdated, _, _ := s.vulnerabilityDB.CheckOutdated(dep); outdated {
			outdatedCount++
		}
	}

	if outdatedCount > 0 {
		recommendations = append(recommendations, SecurityRecommendation{
			Priority:    3,
			Category:    "maintenance",
			Title:       "Update Outdated Dependencies",
			Description: fmt.Sprintf("Update %d outdated dependencies to latest stable versions", outdatedCount),
			Action:      "Review and update dependency versions",
			Impact:      "Medium - Improves overall security posture",
			Effort: EstimatedEffort{
				Level:       "medium",
				TimeMinutes: outdatedCount * 15,
				Complexity:  4,
				Risk:        "low",
				Resources:   []string{"development_team"},
			},
			Urgency: "medium_term",
		})
	}

	return recommendations
}

func (s *SyftSBOMAnalyzer) generateRiskRecommendations(critical, high, medium, low int, metrics SBOMSecurityMetrics) []SecurityRecommendation {
	var recommendations []SecurityRecommendation
	priority := 1

	if critical > 0 {
		recommendations = append(recommendations, SecurityRecommendation{
			Priority:    priority,
			Category:    "critical_vulnerabilities",
			Title:       "Immediate Action Required",
			Description: fmt.Sprintf("Address %d critical vulnerabilities", critical),
			Action:      "Apply emergency patches",
			Impact:      "Critical - System at immediate risk",
			Urgency:     "immediate",
		})
		priority++
	}

	if high > 0 {
		recommendations = append(recommendations, SecurityRecommendation{
			Priority:    priority,
			Category:    "high_vulnerabilities",
			Title:       "High Priority Security Updates",
			Description: fmt.Sprintf("Address %d high-severity vulnerabilities", high),
			Action:      "Schedule security updates within 48 hours",
			Impact:      "High - Significant security risk reduction",
			Urgency:     "short_term",
		})
		priority++
	}

	if metrics.OutdatedDependencies > 0 {
		recommendations = append(recommendations, SecurityRecommendation{
			Priority:    priority,
			Category:    "maintenance",
			Title:       "Dependency Maintenance",
			Description: fmt.Sprintf("Update %d outdated dependencies", metrics.OutdatedDependencies),
			Action:      "Regular dependency updates",
			Impact:      "Medium - Improved security posture",
			Urgency:     "medium_term",
		})
	}

	return recommendations
}
