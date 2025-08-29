package arf

import "context"

// CVEDatabase manages CVE data and queries
type CVEDatabase interface {
	LookupCVE(cveID string) (*CVEInfo, error)
	QueryVulnerabilities(criteria VulnerabilityQuery) ([]VulnerabilityInfo, error)
	UpdateDatabase(ctx context.Context) error
}

// VulnerabilityRemediator generates remediation recipes for security issues
type VulnerabilityRemediator interface {
	GenerateRemediation(vuln VulnerabilityInfo, codebase Codebase) (*RemediationRecipe, error)
	ValidateRemediation(recipe *RemediationRecipe) error
	ApplyRemediation(ctx context.Context, recipe *RemediationRecipe, sandbox string) (*RemediationResult, error)
}

// RiskAnalyzer assesses security risk levels
type RiskAnalyzer interface {
	AnalyzeRisk(vulns []VulnerabilityInfo, context SecurityContext) RiskAssessment
	PrioritizeVulnerabilities(vulns []VulnerabilityInfo) []VulnerabilityPriority
	GenerateRiskReport(assessment RiskAssessment) SecurityReport
}

// SBOMSecurityAnalyzer analyzes SBOM files for security issues
type SBOMSecurityAnalyzer interface {
	AnalyzeSBOM(sbomPath string) (*SBOMSecurityAnalysis, error)
	ExtractDependencies(sbomData map[string]interface{}) ([]Dependency, error)
	CorrelateVulnerabilities(deps []Dependency) ([]VulnerabilityMatch, error)
}
