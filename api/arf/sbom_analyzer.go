package arf

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"
)

// SyftSBOMAnalyzer implements SBOMSecurityAnalyzer using syft-generated SBOMs
type SyftSBOMAnalyzer struct {
	cveDatabase      CVEDatabase
	vulnerabilityDB  VulnerabilityDatabase
	licenseAnalyzer  LicenseAnalyzer
	riskCalculator   RiskCalculator
}

// VulnerabilityDatabase provides vulnerability lookup capabilities
type VulnerabilityDatabase interface {
	FindVulnerabilities(dep Dependency) ([]VulnerabilityInfo, error)
	CheckOutdated(dep Dependency) (bool, string, error)
}

// LicenseAnalyzer analyzes software licenses for compliance issues
type LicenseAnalyzer interface {
	AnalyzeLicense(license string) LicenseAnalysis
	CheckCompliance(licenses []string, policy LicensePolicy) ComplianceResult
}

// RiskCalculator calculates security risk scores
type RiskCalculator interface {
	CalculateSecurityScore(deps []Dependency, vulns []VulnerabilityMatch) float64
	AssessRisk(analysis *SBOMSecurityAnalysis) RiskAssessment
}

// LicenseAnalysis represents license analysis results
type LicenseAnalysis struct {
	License     string   `json:"license"`
	Type        string   `json:"type"` // permissive, copyleft, proprietary, unknown
	Restrictions []string `json:"restrictions"`
	Risks       []string `json:"risks"`
	Compliance  string   `json:"compliance"` // compliant, non_compliant, review_required
}

// LicensePolicy defines license compliance policy
type LicensePolicy struct {
	AllowedLicenses   []string `json:"allowed_licenses"`
	ForbiddenLicenses []string `json:"forbidden_licenses"`
	RequireReview     []string `json:"require_review"`
	CopyleftPolicy    string   `json:"copyleft_policy"` // allow, forbid, review
}

// ComplianceResult represents license compliance check results
type ComplianceResult struct {
	Compliant    bool              `json:"compliant"`
	Violations   []LicenseViolation `json:"violations"`
	ReviewNeeded []string          `json:"review_needed"`
	Summary      string            `json:"summary"`
}

// LicenseViolation represents a license compliance violation
type LicenseViolation struct {
	Dependency string `json:"dependency"`
	License    string `json:"license"`
	Violation  string `json:"violation"`
	Severity   string `json:"severity"`
}

// SyftSBOM represents the structure of syft-generated SBOMs
type SyftSBOM struct {
	Artifacts []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Version   string `json:"version"`
		Type      string `json:"type"`
		FoundBy   string `json:"foundBy"`
		Locations []struct {
			Path               string `json:"path"`
			LayerID            string `json:"layerID,omitempty"`
			AccessPath         string `json:"accessPath,omitempty"`
			AnnotationsPresent bool   `json:"annotationsPresent,omitempty"`
		} `json:"locations"`
		Licenses []string `json:"licenses"`
		Language string   `json:"language"`
		Metadata struct {
			VirtualPath   string `json:"virtualPath,omitempty"`
			Architecture  string `json:"architecture,omitempty"`
			OS            string `json:"os,omitempty"`
			Size          int64  `json:"size,omitempty"`
			ContentDigest string `json:"contentDigest,omitempty"`
		} `json:"metadata"`
	} `json:"artifacts"`
	ArtifactRelationships []struct {
		Parent string `json:"parent"`
		Child  string `json:"child"`
		Type   string `json:"type"`
	} `json:"artifactRelationships"`
	Files []struct {
		ID       string `json:"id"`
		Location struct {
			Path               string `json:"path"`
			LayerID            string `json:"layerID,omitempty"`
			AccessPath         string `json:"accessPath,omitempty"`
			AnnotationsPresent bool   `json:"annotationsPresent,omitempty"`
		} `json:"location"`
		Metadata struct {
			Mode        int    `json:"mode"`
			Type        string `json:"type"`
			LinkDestination string `json:"linkDestination,omitempty"`
			UserID      int    `json:"userID"`
			GroupID     int    `json:"groupID"`
			Size        int64  `json:"size"`
			MIMEType    string `json:"mimeType"`
			Digests     []struct {
				Algorithm string `json:"algorithm"`
				Value     string `json:"value"`
			} `json:"digests"`
		} `json:"metadata"`
		Contents string `json:"contents,omitempty"`
	} `json:"files,omitempty"`
	Distro struct {
		Name            string   `json:"name"`
		Version         string   `json:"version"`
		IDLike          []string `json:"idLike,omitempty"`
		VersionCodename string   `json:"versionCodename,omitempty"`
		VersionID       string   `json:"versionID,omitempty"`
		HomeURL         string   `json:"homeURL,omitempty"`
		SupportURL      string   `json:"supportURL,omitempty"`
		BugReportURL    string   `json:"bugReportURL,omitempty"`
		PrivacyPolicyURL string  `json:"privacyPolicyURL,omitempty"`
	} `json:"distro,omitempty"`
	Descriptor struct {
		Name          string `json:"name"`
		Version       string `json:"version"`
		Configuration struct {
			ConfigPath      string   `json:"configPath"`
			VerboseOutput   bool     `json:"verboseOutput"`
			QuietOutput     bool     `json:"quietOutput"`
			CheckForAppUpdate bool   `json:"checkForAppUpdate"`
			OnlyFixed       bool     `json:"onlyFixed"`
			OnlyNotFixed    bool     `json:"onlyNotFixed"`
			OutputFormat    []string `json:"outputFormat"`
			OutputFile      string   `json:"outputFile"`
			FileMetadata    struct {
				Cataloger struct {
					Enabled bool `json:"enabled"`
				} `json:"cataloger"`
			} `json:"file-metadata"`
		} `json:"configuration"`
	} `json:"descriptor"`
	Schema struct {
		Version string `json:"version"`
		URL     string `json:"url"`
	} `json:"schema"`
	Source struct {
		Type   string `json:"type"`
		Target string `json:"target"`
	} `json:"source"`
}

// NewSyftSBOMAnalyzer creates a new syft SBOM analyzer
func NewSyftSBOMAnalyzer() *SyftSBOMAnalyzer {
	return &SyftSBOMAnalyzer{
		cveDatabase:      NewNVDDatabase(),
		vulnerabilityDB:  nil, // OSV database integration placeholder
		licenseAnalyzer:  nil, // SPDX license analyzer placeholder
		riskCalculator:   nil, // CVSS risk calculator placeholder
	}
}

// AnalyzeSBOM analyzes an SBOM file for security issues
func (s *SyftSBOMAnalyzer) AnalyzeSBOM(sbomPath string) (*SBOMSecurityAnalysis, error) {
	// Read and parse SBOM file
	sbomData, err := s.readSBOMFile(sbomPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SBOM: %w", err)
	}
	
	// Extract dependencies
	dependencies, err := s.ExtractDependencies(sbomData)
	if err != nil {
		return nil, fmt.Errorf("failed to extract dependencies: %w", err)
	}
	
	// Find vulnerabilities
	vulnerabilities, err := s.CorrelateVulnerabilities(dependencies)
	if err != nil {
		return nil, fmt.Errorf("failed to correlate vulnerabilities: %w", err)
	}
	
	// Calculate security metrics
	metrics := s.calculateSecurityMetrics(dependencies, vulnerabilities)
	
	// Generate risk assessment
	riskAssessment := s.generateRiskAssessment(dependencies, vulnerabilities, metrics)
	
	// Generate recommendations
	recommendations := s.generateSecurityRecommendations(dependencies, vulnerabilities, riskAssessment)
	
	analysis := &SBOMSecurityAnalysis{
		Dependencies:    dependencies,
		Vulnerabilities: vulnerabilities,
		SecurityMetrics: metrics,
		RiskAssessment:  riskAssessment,
		Recommendations: recommendations,
		AnalyzedAt:      time.Now(),
	}
	
	return analysis, nil
}

// ExtractDependencies extracts dependency information from SBOM data
func (s *SyftSBOMAnalyzer) ExtractDependencies(sbomData map[string]interface{}) ([]Dependency, error) {
	var dependencies []Dependency
	
	// Handle different SBOM formats
	if artifacts, ok := sbomData["artifacts"].([]interface{}); ok {
		// Syft format
		return s.extractSyftDependencies(artifacts)
	}
	
	if components, ok := sbomData["components"].([]interface{}); ok {
		// CycloneDX format
		return s.extractCycloneDXDependencies(components)
	}
	
	if packages, ok := sbomData["packages"].([]interface{}); ok {
		// SPDX format
		return s.extractSPDXDependencies(packages)
	}
	
	return dependencies, fmt.Errorf("unsupported SBOM format")
}

// extractSyftDependencies extracts dependencies from syft format
func (s *SyftSBOMAnalyzer) extractSyftDependencies(artifacts []interface{}) ([]Dependency, error) {
	var dependencies []Dependency
	
	for _, artifact := range artifacts {
		art, ok := artifact.(map[string]interface{})
		if !ok {
			continue
		}
		
		dep := Dependency{
			Name:      s.getStringValue(art, "name"),
			Version:   s.getStringValue(art, "version"),
			Ecosystem: s.getStringValue(art, "language"),
			Type:      s.getStringValue(art, "type"),
			Metadata: map[string]interface{}{
				"id":      s.getStringValue(art, "id"),
				"foundBy": s.getStringValue(art, "foundBy"),
			},
		}
		
		// Extract location information
		if locations, ok := art["locations"].([]interface{}); ok && len(locations) > 0 {
			if loc, ok := locations[0].(map[string]interface{}); ok {
				dep.Path = s.getStringValue(loc, "path")
			}
		}
		
		// Extract license information
		if licenses, ok := art["licenses"].([]interface{}); ok {
			var licenseList []string
			for _, license := range licenses {
				if licStr, ok := license.(string); ok {
					licenseList = append(licenseList, licStr)
				}
			}
			if len(licenseList) > 0 {
				dep.Metadata["licenses"] = licenseList
			}
		}
		
		// Map ecosystem names to standard values
		dep.Ecosystem = s.normalizeEcosystem(dep.Ecosystem, dep.Type)
		
		dependencies = append(dependencies, dep)
	}
	
	return dependencies, nil
}

// extractCycloneDXDependencies extracts dependencies from CycloneDX format
func (s *SyftSBOMAnalyzer) extractCycloneDXDependencies(components []interface{}) ([]Dependency, error) {
	var dependencies []Dependency
	
	for _, component := range components {
		comp, ok := component.(map[string]interface{})
		if !ok {
			continue
		}
		
		dep := Dependency{
			Name:      s.getStringValue(comp, "name"),
			Version:   s.getStringValue(comp, "version"),
			Type:      s.getStringValue(comp, "type"),
			Metadata:  make(map[string]interface{}),
		}
		
		// Extract purl information for ecosystem
		if purl := s.getStringValue(comp, "purl"); purl != "" {
			dep.Ecosystem = s.extractEcosystemFromPURL(purl)
			dep.Metadata["purl"] = purl
		}
		
		// Extract license information
		if licenses, ok := comp["licenses"].([]interface{}); ok {
			var licenseList []string
			for _, license := range licenses {
				if lic, ok := license.(map[string]interface{}); ok {
					if id := s.getStringValue(lic, "id"); id != "" {
						licenseList = append(licenseList, id)
					} else if name := s.getStringValue(lic, "name"); name != "" {
						licenseList = append(licenseList, name)
					}
				}
			}
			if len(licenseList) > 0 {
				dep.Metadata["licenses"] = licenseList
			}
		}
		
		dependencies = append(dependencies, dep)
	}
	
	return dependencies, nil
}

// extractSPDXDependencies extracts dependencies from SPDX format
func (s *SyftSBOMAnalyzer) extractSPDXDependencies(packages []interface{}) ([]Dependency, error) {
	var dependencies []Dependency
	
	for _, pkg := range packages {
		p, ok := pkg.(map[string]interface{})
		if !ok {
			continue
		}
		
		dep := Dependency{
			Name:     s.getStringValue(p, "name"),
			Version:  s.getStringValue(p, "versionInfo"),
			Metadata: make(map[string]interface{}),
		}
		
		// Extract download location for ecosystem detection
		if downloadLocation := s.getStringValue(p, "downloadLocation"); downloadLocation != "" {
			dep.Ecosystem = s.inferEcosystemFromURL(downloadLocation)
			dep.Metadata["downloadLocation"] = downloadLocation
		}
		
		// Extract license information
		if licenseConcluded := s.getStringValue(p, "licenseConcluded"); licenseConcluded != "" {
			dep.Metadata["licenses"] = []string{licenseConcluded}
		}
		
		dependencies = append(dependencies, dep)
	}
	
	return dependencies, nil
}

// CorrelateVulnerabilities finds vulnerabilities for dependencies
func (s *SyftSBOMAnalyzer) CorrelateVulnerabilities(deps []Dependency) ([]VulnerabilityMatch, error) {
	var matches []VulnerabilityMatch
	
	for _, dep := range deps {
		vulns, err := s.vulnerabilityDB.FindVulnerabilities(dep)
		if err != nil {
			// Log error but continue processing
			continue
		}
		
		for _, vuln := range vulns {
			match := VulnerabilityMatch{
				Dependency:      dep,
				Vulnerability:   vuln,
				MatchConfidence: s.calculateMatchConfidence(dep, vuln),
				MatchReason:     s.determineMatchReason(dep, vuln),
			}
			matches = append(matches, match)
		}
	}
	
	return matches, nil
}

// readSBOMFile reads and parses an SBOM file
func (s *SyftSBOMAnalyzer) readSBOMFile(sbomPath string) (map[string]interface{}, error) {
	data, err := ioutil.ReadFile(sbomPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	
	var sbomData map[string]interface{}
	if err := json.Unmarshal(data, &sbomData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	
	return sbomData, nil
}

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
		OverallRisk:     overallRisk,
		RiskScore:       riskScore,
		CriticalCount:   criticalCount,
		HighCount:       highCount,
		MediumCount:     mediumCount,
		LowCount:        lowCount,
		Recommendations: recommendations,
		Timeline:        timeline,
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

// Helper functions

func (s *SyftSBOMAnalyzer) getStringValue(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func (s *SyftSBOMAnalyzer) normalizeEcosystem(language, pkgType string) string {
	// Map language/type to standard ecosystem names
	switch strings.ToLower(language) {
	case "java":
		return "maven"
	case "javascript", "typescript":
		return "npm"
	case "python":
		return "pypi"
	case "go":
		return "go"
	case "rust":
		return "cargo"
	case "ruby":
		return "gem"
	case "php":
		return "packagist"
	case "c#", "csharp":
		return "nuget"
	default:
		// Try to infer from package type
		switch strings.ToLower(pkgType) {
		case "jar":
			return "maven"
		case "wheel", "egg":
			return "pypi"
		case "gem":
			return "gem"
		default:
			return strings.ToLower(language)
		}
	}
}

func (s *SyftSBOMAnalyzer) extractEcosystemFromPURL(purl string) string {
	// Extract ecosystem from Package URL (purl)
	// Format: pkg:type/namespace/name@version
	parts := strings.Split(purl, ":")
	if len(parts) >= 2 {
		return parts[1]
	}
	return "unknown"
}

func (s *SyftSBOMAnalyzer) inferEcosystemFromURL(url string) string {
	url = strings.ToLower(url)
	if strings.Contains(url, "maven") {
		return "maven"
	}
	if strings.Contains(url, "npmjs") || strings.Contains(url, "npm") {
		return "npm"
	}
	if strings.Contains(url, "pypi") {
		return "pypi"
	}
	if strings.Contains(url, "github.com") {
		return "github"
	}
	return "unknown"
}

func (s *SyftSBOMAnalyzer) calculateMatchConfidence(dep Dependency, vuln VulnerabilityInfo) float64 {
	confidence := 0.5 // Base confidence
	
	// Exact name match
	if dep.Name == vuln.Package.Name {
		confidence += 0.3
	}
	
	// Exact version match
	if dep.Version == vuln.Package.Version {
		confidence += 0.2
	}
	
	// Ecosystem match
	if dep.Ecosystem == vuln.Package.Ecosystem {
		confidence += 0.2
	}
	
	if confidence > 1.0 {
		confidence = 1.0
	}
	
	return confidence
}

func (s *SyftSBOMAnalyzer) determineMatchReason(dep Dependency, vuln VulnerabilityInfo) string {
	reasons := []string{}
	
	if dep.Name == vuln.Package.Name {
		reasons = append(reasons, "exact_name_match")
	}
	
	if dep.Version == vuln.Package.Version {
		reasons = append(reasons, "exact_version_match")
	}
	
	if dep.Ecosystem == vuln.Package.Ecosystem {
		reasons = append(reasons, "ecosystem_match")
	}
	
	if len(reasons) == 0 {
		reasons = append(reasons, "fuzzy_match")
	}
	
	return strings.Join(reasons, ",")
}

func (s *SyftSBOMAnalyzer) isProblematicLicense(license string) bool {
	problematic := []string{
		"AGPL", "GPL-2.0", "GPL-3.0", "LGPL", 
		"SSPL", "OSL", "EPL", "MPL-2.0",
		"UNKNOWN", "NOASSERTION",
	}
	
	license = strings.ToUpper(license)
	for _, p := range problematic {
		if strings.Contains(license, p) {
			return true
		}
	}
	return false
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