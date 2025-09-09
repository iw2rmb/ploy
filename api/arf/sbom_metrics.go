package arf

// calculateSecurityMetrics calculates security metrics for the SBOM
func (s *SyftSBOMAnalyzer) calculateSecurityMetrics(deps []Dependency, vulns []VulnerabilityMatch) SBOMSecurityMetrics {
	vulnerableDeps := make(map[string]bool)
	licenseIssues := 0
	outdatedCount := 0

	for _, vuln := range vulns {
		vulnerableDeps[vuln.Dependency.Name] = true
	}

	// Count license issues and outdated dependencies
	for _, dep := range deps {
		if licenses, ok := dep.Metadata["licenses"].([]string); ok {
			for _, license := range licenses {
				if s.isProblematicLicense(license) {
					licenseIssues++
					break
				}
			}
		}

		if outdated, _, err := s.vulnerabilityDB.CheckOutdated(dep); err == nil && outdated {
			outdatedCount++
		}
	}

	securityScore := s.riskCalculator.CalculateSecurityScore(deps, vulns)

	return SBOMSecurityMetrics{
		TotalDependencies:      len(deps),
		VulnerableDependencies: len(vulnerableDeps),
		SecurityScore:          securityScore,
		LicenseIssues:          licenseIssues,
		OutdatedDependencies:   outdatedCount,
	}
}
