package security

import "context"

// CVEDatabase manages CVE data and queries
type CVEDatabase interface {
	LookupCVE(cveID string) (*CVEInfo, error)
	QueryVulnerabilities(criteria VulnerabilityQuery) ([]VulnerabilityInfo, error)
	UpdateDatabase(ctx context.Context) error
}

// VulnerabilityModPlanner plans code modifications for security issues
type VulnerabilityModPlanner interface {
	GenerateModification(vuln VulnerabilityInfo, codebase Codebase) (*ModRecipe, error)
	ValidateModification(recipe *ModRecipe) error
	ApplyModification(ctx context.Context, recipe *ModRecipe, sandbox string) (*ModResult, error)
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
