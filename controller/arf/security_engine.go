package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SecurityEngine handles vulnerability remediation and security analysis
type SecurityEngine struct {
	grypePath      string
	cveDatabase    CVEDatabase
	remediator     VulnerabilityRemediator
	riskAnalyzer   RiskAnalyzer
	sbomAnalyzer   SBOMSecurityAnalyzer
	httpClient     *http.Client
}

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

// CVEInfo represents detailed CVE information
type CVEInfo struct {
	ID              string                 `json:"id"`
	Description     string                 `json:"description"`
	CVSS            CVSSScore              `json:"cvss"`
	AffectedPackages []AffectedPackage      `json:"affected_packages"`
	References      []CVEReference         `json:"references"`
	PublishedDate   time.Time              `json:"published_date"`
	Severity        string                 `json:"severity"`
	Remediation     RemediationGuidance    `json:"remediation"`
	Exploitability  ExploitabilityInfo     `json:"exploitability"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// CVSSScore represents CVSS vulnerability scoring
type CVSSScore struct {
	Version     string  `json:"version"`
	BaseScore   float64 `json:"base_score"`
	Vector      string  `json:"vector"`
	Impact      float64 `json:"impact_score"`
	Exploitability float64 `json:"exploitability_score"`
}

// AffectedPackage represents a package affected by a CVE
type AffectedPackage struct {
	Name            string   `json:"name"`
	Ecosystem       string   `json:"ecosystem"`
	AffectedVersions []string `json:"affected_versions"`
	FixedVersions   []string `json:"fixed_versions"`
	PatchAvailable  bool     `json:"patch_available"`
}

// CVEReference represents external references for a CVE
type CVEReference struct {
	Type string `json:"type"`
	URL  string `json:"url"`
	Tags []string `json:"tags,omitempty"`
}

// RemediationGuidance provides guidance for fixing vulnerabilities
type RemediationGuidance struct {
	Type           string                 `json:"type"` // upgrade, patch, config, replace
	Instructions   string                 `json:"instructions"`
	AutoApplicable bool                   `json:"auto_applicable"`
	Recipe         *AutoRemediationRecipe `json:"recipe,omitempty"`
	Confidence     float64                `json:"confidence"`
	Effort         EstimatedEffort        `json:"effort"`
}

// AutoRemediationRecipe contains automatic remediation instructions
type AutoRemediationRecipe struct {
	Type           string                 `json:"type"`
	Operations     []RemediationOperation `json:"operations"`
	Validation     ValidationCriteria     `json:"validation"`
	Rollback       RollbackStrategy       `json:"rollback"`
	Dependencies   []string               `json:"dependencies"`
}

// RemediationOperation represents a specific remediation action
type RemediationOperation struct {
	Action      string                 `json:"action"` // replace, upgrade, remove, configure
	Target      OperationTarget        `json:"target"`
	Parameters  map[string]interface{} `json:"parameters"`
	Conditions  []OperationCondition   `json:"conditions"`
}

// OperationTarget specifies what the operation targets
type OperationTarget struct {
	Type       string   `json:"type"` // dependency, file, configuration
	Identifier string   `json:"identifier"`
	Path       string   `json:"path,omitempty"`
	Pattern    string   `json:"pattern,omitempty"`
	Files      []string `json:"files,omitempty"`
}

// OperationCondition represents conditions for applying operations
type OperationCondition struct {
	Type      string      `json:"type"`
	Field     string      `json:"field"`
	Operator  string      `json:"operator"`
	Value     interface{} `json:"value"`
	Required  bool        `json:"required"`
}

// ValidationCriteria defines how to validate remediation success
type ValidationCriteria struct {
	Tests       []ValidationTest `json:"tests"`
	Metrics     []string         `json:"metrics"`
	Timeout     time.Duration    `json:"timeout"`
	SuccessRate float64          `json:"required_success_rate"`
}

// ValidationTest represents a single validation test
type ValidationTest struct {
	Type        string                 `json:"type"`
	Command     string                 `json:"command,omitempty"`
	Expected    interface{}            `json:"expected"`
	Description string                 `json:"description"`
	Critical    bool                   `json:"critical"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// RollbackStrategy defines how to roll back failed remediations
type RollbackStrategy struct {
	Type       string            `json:"type"`
	Steps      []RollbackStep    `json:"steps"`
	Timeout    time.Duration     `json:"timeout"`
	Validation ValidationCriteria `json:"validation"`
}

// RollbackStep represents a single rollback operation
type RollbackStep struct {
	Action      string                 `json:"action"`
	Target      OperationTarget        `json:"target"`
	Parameters  map[string]interface{} `json:"parameters"`
	Order       int                    `json:"order"`
}

// EstimatedEffort represents the effort required for remediation
type EstimatedEffort struct {
	Level       string        `json:"level"` // low, medium, high, critical
	TimeMinutes int           `json:"estimated_minutes"`
	Complexity  int           `json:"complexity_score"` // 1-10
	Risk        string        `json:"risk_level"`
	Resources   []string      `json:"required_resources"`
}

// ExploitabilityInfo provides information about exploit potential
type ExploitabilityInfo struct {
	HasExploit       bool     `json:"has_public_exploit"`
	ExploitMaturity  string   `json:"exploit_maturity"`
	AttackVector     string   `json:"attack_vector"`
	AttackComplexity string   `json:"attack_complexity"`
	References       []string `json:"exploit_references"`
}

// VulnerabilityQuery defines criteria for vulnerability searches
type VulnerabilityQuery struct {
	PackageName string    `json:"package_name"`
	Ecosystem   string    `json:"ecosystem"`
	Version     string    `json:"version"`
	Severity    []string  `json:"severity_filter"`
	DateRange   DateRange `json:"date_range"`
	HasExploit  *bool     `json:"has_exploit"`
	CVSS        CVSSRange `json:"cvss_range"`
}

// DateRange represents a date range filter
type DateRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// CVSSRange represents CVSS score filtering
type CVSSRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// VulnerabilityInfo represents comprehensive vulnerability information
type VulnerabilityInfo struct {
	CVE             CVEInfo               `json:"cve"`
	Package         Dependency            `json:"package"`
	Severity        string                `json:"severity"`
	CVSS            float64               `json:"cvss_score"`
	Exploitable     bool                  `json:"exploitable"`
	HasFix          bool                  `json:"has_fix"`
	FixVersion      string                `json:"fix_version,omitempty"`
	Remediation     RemediationGuidance   `json:"remediation"`
	Context         SecurityContext       `json:"context"`
	Discovery       VulnerabilityDiscovery `json:"discovery"`
}

// Dependency represents a software dependency
type Dependency struct {
	Name      string            `json:"name"`
	Version   string            `json:"version"`
	Ecosystem string            `json:"ecosystem"`
	Type      string            `json:"type"`
	Path      string            `json:"path,omitempty"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// SecurityContext provides context for security analysis
type SecurityContext struct {
	Environment   string   `json:"environment"` // development, staging, production
	ExposureLevel string   `json:"exposure_level"` // internal, external, public
	DataSensitivity string `json:"data_sensitivity"` // low, medium, high
	ComplianceReqs []string `json:"compliance_requirements"`
	BusinessImpact string   `json:"business_impact"`
}

// VulnerabilityDiscovery tracks how a vulnerability was discovered
type VulnerabilityDiscovery struct {
	Method      string    `json:"method"` // scan, manual, automated
	Tool        string    `json:"tool"`
	Timestamp   time.Time `json:"timestamp"`
	Confidence  float64   `json:"confidence"`
	Source      string    `json:"source"`
}

// RiskAssessment represents overall security risk assessment
type RiskAssessment struct {
	OverallRisk      string                    `json:"overall_risk"`
	RiskScore        float64                   `json:"risk_score"`
	CriticalCount    int                       `json:"critical_count"`
	HighCount        int                       `json:"high_count"`
	MediumCount      int                       `json:"medium_count"`
	LowCount         int                       `json:"low_count"`
	Recommendations  []SecurityRecommendation  `json:"recommendations"`
	Timeline         RemediationTimeline       `json:"timeline"`
	ComplianceStatus ComplianceStatus          `json:"compliance"`
}

// SecurityRecommendation represents an actionable security recommendation
type SecurityRecommendation struct {
	Priority    int             `json:"priority"`
	Category    string          `json:"category"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Action      string          `json:"action"`
	Impact      string          `json:"impact"`
	Effort      EstimatedEffort `json:"effort"`
	Urgency     string          `json:"urgency"`
}

// RemediationTimeline provides timeline for addressing vulnerabilities
type RemediationTimeline struct {
	Immediate []string `json:"immediate"` // < 24 hours
	Short     []string `json:"short_term"` // < 1 week
	Medium    []string `json:"medium_term"` // < 1 month
	Long      []string `json:"long_term"` // > 1 month
}

// ComplianceStatus tracks compliance with security frameworks
type ComplianceStatus struct {
	Frameworks map[string]FrameworkCompliance `json:"frameworks"`
	Overall    string                         `json:"overall_status"`
	Issues     []ComplianceIssue              `json:"issues"`
}

// FrameworkCompliance represents compliance with a specific framework
type FrameworkCompliance struct {
	Name        string  `json:"name"`
	Version     string  `json:"version"`
	Score       float64 `json:"score"`
	Status      string  `json:"status"`
	Requirements map[string]bool `json:"requirements"`
}

// ComplianceIssue represents a compliance-related issue
type ComplianceIssue struct {
	Framework   string   `json:"framework"`
	Requirement string   `json:"requirement"`
	Description string   `json:"description"`
	Severity    string   `json:"severity"`
	Actions     []string `json:"required_actions"`
}

// VulnerabilityPriority represents prioritized vulnerability information
type VulnerabilityPriority struct {
	Vulnerability VulnerabilityInfo `json:"vulnerability"`
	Priority      int               `json:"priority"`
	Urgency       string            `json:"urgency"`
	Justification string            `json:"justification"`
	EstimatedFix  time.Duration     `json:"estimated_fix_time"`
}

// SecurityReport represents a comprehensive security analysis report
type SecurityReport struct {
	Summary         SecuritySummary        `json:"summary"`
	Vulnerabilities []VulnerabilityInfo    `json:"vulnerabilities"`
	RiskAssessment  RiskAssessment         `json:"risk_assessment"`
	Recommendations []SecurityRecommendation `json:"recommendations"`
	ComplianceReport ComplianceStatus       `json:"compliance"`
	GeneratedAt     time.Time              `json:"generated_at"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// SecuritySummary provides high-level security status
type SecuritySummary struct {
	TotalVulnerabilities int     `json:"total_vulnerabilities"`
	RiskScore            float64 `json:"risk_score"`
	FixableCount         int     `json:"fixable_count"`
	ExploitableCount     int     `json:"exploitable_count"`
	Status               string  `json:"status"`
}

// RemediationRecipe represents a complete remediation solution
type RemediationRecipe struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Vulnerabilities []string               `json:"target_vulnerabilities"`
	Recipe          AutoRemediationRecipe  `json:"recipe"`
	Metadata        map[string]interface{} `json:"metadata"`
	CreatedAt       time.Time              `json:"created_at"`
	Version         string                 `json:"version"`
}

// RemediationResult represents the outcome of applying a remediation
type RemediationResult struct {
	Success          bool                   `json:"success"`
	VulnerabilitiesFixed []string           `json:"vulnerabilities_fixed"`
	Errors           []string               `json:"errors"`
	Warnings         []string               `json:"warnings"`
	ValidationResults []ValidationResult    `json:"validation_results"`
	Duration         time.Duration          `json:"duration"`
	ChangeSummary    ChangeSummary          `json:"changes"`
	RollbackRequired bool                   `json:"rollback_required"`
}

// ValidationResult represents the outcome of a validation test
type ValidationResult struct {
	Test        ValidationTest `json:"test"`
	Success     bool           `json:"success"`
	Output      string         `json:"output"`
	Error       string         `json:"error,omitempty"`
	Duration    time.Duration  `json:"duration"`
	Critical    bool           `json:"critical"`
}

// ChangeSummary summarizes changes made during remediation
type ChangeSummary struct {
	FilesModified   []string `json:"files_modified"`
	DependenciesChanged []DependencyChange `json:"dependencies_changed"`
	ConfigurationChanges []ConfigChange `json:"configuration_changes"`
	Statistics      ChangeStatistics `json:"statistics"`
}

// DependencyChange represents a change to a dependency
type DependencyChange struct {
	Name        string `json:"name"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
	Action      string `json:"action"` // upgrade, add, remove
	Reason      string `json:"reason"`
}

// ConfigChange represents a configuration change
type ConfigChange struct {
	File        string      `json:"file"`
	Path        string      `json:"path"`
	OldValue    interface{} `json:"old_value"`
	NewValue    interface{} `json:"new_value"`
	Description string      `json:"description"`
}

// ChangeStatistics provides statistics about remediation changes
type ChangeStatistics struct {
	TotalFiles      int `json:"total_files_changed"`
	LinesAdded      int `json:"lines_added"`
	LinesRemoved    int `json:"lines_removed"`
	DependencyCount int `json:"dependencies_changed"`
	ConfigCount     int `json:"configurations_changed"`
}

// SBOMSecurityAnalysis represents security analysis of an SBOM
type SBOMSecurityAnalysis struct {
	Dependencies      []Dependency          `json:"dependencies"`
	Vulnerabilities   []VulnerabilityMatch  `json:"vulnerabilities"`
	SecurityMetrics   SBOMSecurityMetrics   `json:"metrics"`
	RiskAssessment    RiskAssessment        `json:"risk_assessment"`
	Recommendations   []SecurityRecommendation `json:"recommendations"`
	AnalyzedAt        time.Time             `json:"analyzed_at"`
}

// VulnerabilityMatch represents a vulnerability matched to a dependency
type VulnerabilityMatch struct {
	Dependency      Dependency        `json:"dependency"`
	Vulnerability   VulnerabilityInfo `json:"vulnerability"`
	MatchConfidence float64           `json:"match_confidence"`
	MatchReason     string            `json:"match_reason"`
}

// SBOMSecurityMetrics provides security metrics for an SBOM
type SBOMSecurityMetrics struct {
	TotalDependencies    int     `json:"total_dependencies"`
	VulnerableDependencies int   `json:"vulnerable_dependencies"`
	SecurityScore        float64 `json:"security_score"`
	LicenseIssues        int     `json:"license_issues"`
	OutdatedDependencies int     `json:"outdated_dependencies"`
}

// NewSecurityEngine creates a new security engine instance
func NewSecurityEngine() *SecurityEngine {
	return &SecurityEngine{
		grypePath:    "grype",
		cveDatabase:  NewNVDDatabase(),
		remediator:   NewOpenRewriteRemediator(),
		riskAnalyzer: NewCVSSRiskAnalyzer(),
		sbomAnalyzer: NewSyftSBOMAnalyzer(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ScanForVulnerabilities performs comprehensive vulnerability scanning
func (s *SecurityEngine) ScanForVulnerabilities(ctx context.Context, target string, scanType string) (*SecurityReport, error) {
	var vulns []VulnerabilityInfo
	var err error
	
	switch scanType {
	case "sbom":
		vulns, err = s.scanSBOM(ctx, target)
	case "container":
		vulns, err = s.scanContainer(ctx, target)
	case "source":
		vulns, err = s.scanSource(ctx, target)
	default:
		return nil, fmt.Errorf("unsupported scan type: %s", scanType)
	}
	
	if err != nil {
		return nil, fmt.Errorf("vulnerability scan failed: %w", err)
	}
	
	// Enrich vulnerabilities with CVE data
	enrichedVulns := make([]VulnerabilityInfo, len(vulns))
	for i, vuln := range vulns {
		enriched, err := s.enrichVulnerability(ctx, vuln)
		if err != nil {
			// Log but don't fail on enrichment errors
			enriched = vuln
		}
		enrichedVulns[i] = enriched
	}
	
	// Generate risk assessment
	context := SecurityContext{
		Environment:     "production", // TODO: make configurable
		ExposureLevel:   "external",
		DataSensitivity: "medium",
	}
	
	riskAssessment := s.riskAnalyzer.AnalyzeRisk(enrichedVulns, context)
	recommendations := s.generateRecommendations(enrichedVulns, riskAssessment)
	
	report := &SecurityReport{
		Summary: SecuritySummary{
			TotalVulnerabilities: len(enrichedVulns),
			RiskScore:            riskAssessment.RiskScore,
			FixableCount:         s.countFixable(enrichedVulns),
			ExploitableCount:     s.countExploitable(enrichedVulns),
			Status:               s.determineStatus(riskAssessment.RiskScore),
		},
		Vulnerabilities:  enrichedVulns,
		RiskAssessment:   riskAssessment,
		Recommendations:  recommendations,
		ComplianceReport: s.assessCompliance(enrichedVulns),
		GeneratedAt:      time.Now(),
		Metadata: map[string]interface{}{
			"scan_type": scanType,
			"target":    target,
			"engine":    "ploy-arf-security",
		},
	}
	
	return report, nil
}

// scanSBOM scans an SBOM file for vulnerabilities
func (s *SecurityEngine) scanSBOM(ctx context.Context, sbomPath string) ([]VulnerabilityInfo, error) {
	// Use grype to scan SBOM
	cmd := exec.CommandContext(ctx, s.grypePath, sbomPath, "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("grype scan failed: %w", err)
	}
	
	// Parse grype output
	return s.parseGrypeOutput(output)
}

// scanContainer scans a container image for vulnerabilities
func (s *SecurityEngine) scanContainer(ctx context.Context, image string) ([]VulnerabilityInfo, error) {
	cmd := exec.CommandContext(ctx, s.grypePath, image, "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("container scan failed: %w", err)
	}
	
	return s.parseGrypeOutput(output)
}

// scanSource scans source code for vulnerabilities
func (s *SecurityEngine) scanSource(ctx context.Context, sourcePath string) ([]VulnerabilityInfo, error) {
	cmd := exec.CommandContext(ctx, s.grypePath, "dir:"+sourcePath, "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("source scan failed: %w", err)
	}
	
	return s.parseGrypeOutput(output)
}

// parseGrypeOutput parses grype JSON output into VulnerabilityInfo
func (s *SecurityEngine) parseGrypeOutput(output []byte) ([]VulnerabilityInfo, error) {
	var grypeReport struct {
		Matches []struct {
			Vulnerability struct {
				ID          string `json:"id"`
				DataSource  string `json:"dataSource"`
				Severity    string `json:"severity"`
				URLs        []string `json:"urls"`
				Description string `json:"description"`
				Cvss        []struct {
					Version string  `json:"version"`
					Vector  string  `json:"vector"`
					Metrics struct {
						BaseScore           float64 `json:"baseScore"`
						ExploitabilityScore float64 `json:"exploitabilityScore"`
						ImpactScore         float64 `json:"impactScore"`
					} `json:"metrics"`
				} `json:"cvss"`
			} `json:"vulnerability"`
			RelatedVulnerabilities []struct {
				ID        string `json:"id"`
				DataSource string `json:"dataSource"`
			} `json:"relatedVulnerabilities"`
			Artifact struct {
				Name      string `json:"name"`
				Version   string `json:"version"`
				Type      string `json:"type"`
				Language  string `json:"language"`
				Locations []struct {
					Path string `json:"path"`
				} `json:"locations"`
			} `json:"artifact"`
		} `json:"matches"`
	}
	
	if err := json.Unmarshal(output, &grypeReport); err != nil {
		return nil, fmt.Errorf("failed to parse grype output: %w", err)
	}
	
	vulns := make([]VulnerabilityInfo, len(grypeReport.Matches))
	for i, match := range grypeReport.Matches {
		vuln := VulnerabilityInfo{
			CVE: CVEInfo{
				ID:          match.Vulnerability.ID,
				Description: match.Vulnerability.Description,
				Severity:    match.Vulnerability.Severity,
				References:  s.parseReferences(match.Vulnerability.URLs),
			},
			Package: Dependency{
				Name:      match.Artifact.Name,
				Version:   match.Artifact.Version,
				Ecosystem: match.Artifact.Language,
				Type:      match.Artifact.Type,
			},
			Severity:    match.Vulnerability.Severity,
			Discovery: VulnerabilityDiscovery{
				Method:    "scan",
				Tool:      "grype",
				Timestamp: time.Now(),
				Confidence: 0.9,
				Source:    match.Vulnerability.DataSource,
			},
		}
		
		// Parse CVSS if available
		if len(match.Vulnerability.Cvss) > 0 {
			cvss := match.Vulnerability.Cvss[0]
			vuln.CVE.CVSS = CVSSScore{
				Version:        cvss.Version,
				BaseScore:      cvss.Metrics.BaseScore,
				Vector:         cvss.Vector,
				Impact:         cvss.Metrics.ImpactScore,
				Exploitability: cvss.Metrics.ExploitabilityScore,
			}
			vuln.CVSS = cvss.Metrics.BaseScore
		}
		
		// Set package path if available
		if len(match.Artifact.Locations) > 0 {
			vuln.Package.Path = match.Artifact.Locations[0].Path
		}
		
		vulns[i] = vuln
	}
	
	return vulns, nil
}

// parseReferences converts URL list to CVE references
func (s *SecurityEngine) parseReferences(urls []string) []CVEReference {
	refs := make([]CVEReference, len(urls))
	for i, url := range urls {
		refs[i] = CVEReference{
			Type: "advisory",
			URL:  url,
		}
	}
	return refs
}

// enrichVulnerability enriches vulnerability data with additional CVE information
func (s *SecurityEngine) enrichVulnerability(ctx context.Context, vuln VulnerabilityInfo) (VulnerabilityInfo, error) {
	// Look up detailed CVE information
	if cveInfo, err := s.cveDatabase.LookupCVE(vuln.CVE.ID); err == nil {
		vuln.CVE = *cveInfo
		vuln.CVSS = cveInfo.CVSS.BaseScore
		vuln.Exploitable = cveInfo.Exploitability.HasExploit
		
		// Check if fix is available
		for _, pkg := range cveInfo.AffectedPackages {
			if pkg.Name == vuln.Package.Name && pkg.PatchAvailable {
				vuln.HasFix = true
				if len(pkg.FixedVersions) > 0 {
					vuln.FixVersion = pkg.FixedVersions[0]
				}
				break
			}
		}
		
		vuln.Remediation = cveInfo.Remediation
	}
	
	return vuln, nil
}

// generateRecommendations generates security recommendations based on vulnerabilities
func (s *SecurityEngine) generateRecommendations(vulns []VulnerabilityInfo, assessment RiskAssessment) []SecurityRecommendation {
	var recommendations []SecurityRecommendation
	
	// Prioritize critical vulnerabilities
	criticalVulns := s.filterBySeverity(vulns, "Critical")
	if len(criticalVulns) > 0 {
		recommendations = append(recommendations, SecurityRecommendation{
			Priority:    1,
			Category:    "immediate",
			Title:       "Address Critical Vulnerabilities",
			Description: fmt.Sprintf("Found %d critical vulnerabilities requiring immediate attention", len(criticalVulns)),
			Action:      "Apply security patches and updates immediately",
			Impact:      "High - Prevents potential system compromise",
			Urgency:     "immediate",
			Effort: EstimatedEffort{
				Level:       "high",
				TimeMinutes: len(criticalVulns) * 30,
				Complexity:  8,
				Risk:        "high",
			},
		})
	}
	
	// Recommend dependency updates
	outdatedDeps := s.findOutdatedDependencies(vulns)
	if len(outdatedDeps) > 0 {
		recommendations = append(recommendations, SecurityRecommendation{
			Priority:    2,
			Category:    "maintenance",
			Title:       "Update Outdated Dependencies",
			Description: fmt.Sprintf("Update %d outdated dependencies to latest secure versions", len(outdatedDeps)),
			Action:      "Run dependency update process",
			Impact:      "Medium - Reduces attack surface",
			Urgency:     "short_term",
			Effort: EstimatedEffort{
				Level:       "medium",
				TimeMinutes: len(outdatedDeps) * 15,
				Complexity:  5,
				Risk:        "medium",
			},
		})
	}
	
	return recommendations
}

// filterBySeverity filters vulnerabilities by severity level
func (s *SecurityEngine) filterBySeverity(vulns []VulnerabilityInfo, severity string) []VulnerabilityInfo {
	var filtered []VulnerabilityInfo
	for _, vuln := range vulns {
		if strings.EqualFold(vuln.Severity, severity) {
			filtered = append(filtered, vuln)
		}
	}
	return filtered
}

// findOutdatedDependencies identifies dependencies that need updates
func (s *SecurityEngine) findOutdatedDependencies(vulns []VulnerabilityInfo) []Dependency {
	depMap := make(map[string]Dependency)
	
	for _, vuln := range vulns {
		if vuln.HasFix {
			key := vuln.Package.Name + ":" + vuln.Package.Ecosystem
			if _, exists := depMap[key]; !exists {
				depMap[key] = vuln.Package
			}
		}
	}
	
	deps := make([]Dependency, 0, len(depMap))
	for _, dep := range depMap {
		deps = append(deps, dep)
	}
	
	return deps
}

// countFixable counts vulnerabilities that have available fixes
func (s *SecurityEngine) countFixable(vulns []VulnerabilityInfo) int {
	count := 0
	for _, vuln := range vulns {
		if vuln.HasFix {
			count++
		}
	}
	return count
}

// countExploitable counts vulnerabilities with known exploits
func (s *SecurityEngine) countExploitable(vulns []VulnerabilityInfo) int {
	count := 0
	for _, vuln := range vulns {
		if vuln.Exploitable {
			count++
		}
	}
	return count
}

// determineStatus determines overall security status based on risk score
func (s *SecurityEngine) determineStatus(riskScore float64) string {
	switch {
	case riskScore >= 9.0:
		return "critical"
	case riskScore >= 7.0:
		return "high"
	case riskScore >= 4.0:
		return "medium"
	case riskScore >= 1.0:
		return "low"
	default:
		return "secure"
	}
}

// assessCompliance performs compliance assessment
func (s *SecurityEngine) assessCompliance(vulns []VulnerabilityInfo) ComplianceStatus {
	frameworks := map[string]FrameworkCompliance{
		"OWASP": {
			Name:    "OWASP Top 10",
			Version: "2021",
			Score:   s.calculateOWASPScore(vulns),
			Status:  "partial",
		},
		"NIST": {
			Name:    "NIST Cybersecurity Framework",
			Version: "1.1",
			Score:   s.calculateNISTScore(vulns),
			Status:  "partial",
		},
	}
	
	return ComplianceStatus{
		Frameworks: frameworks,
		Overall:    "partial_compliance",
		Issues:     s.identifyComplianceIssues(vulns, frameworks),
	}
}

// calculateOWASPScore calculates OWASP compliance score
func (s *SecurityEngine) calculateOWASPScore(vulns []VulnerabilityInfo) float64 {
	if len(vulns) == 0 {
		return 10.0
	}
	
	criticalCount := len(s.filterBySeverity(vulns, "Critical"))
	highCount := len(s.filterBySeverity(vulns, "High"))
	
	score := 10.0 - float64(criticalCount*3+highCount*2)
	if score < 0 {
		score = 0
	}
	
	return score
}

// calculateNISTScore calculates NIST compliance score
func (s *SecurityEngine) calculateNISTScore(vulns []VulnerabilityInfo) float64 {
	return s.calculateOWASPScore(vulns) // Simplified implementation
}

// identifyComplianceIssues identifies specific compliance issues
func (s *SecurityEngine) identifyComplianceIssues(vulns []VulnerabilityInfo, frameworks map[string]FrameworkCompliance) []ComplianceIssue {
	var issues []ComplianceIssue
	
	criticalVulns := s.filterBySeverity(vulns, "Critical")
	if len(criticalVulns) > 0 {
		issues = append(issues, ComplianceIssue{
			Framework:   "OWASP",
			Requirement: "A06:2021 – Vulnerable and Outdated Components",
			Description: fmt.Sprintf("%d critical vulnerabilities found in dependencies", len(criticalVulns)),
			Severity:    "high",
			Actions:     []string{"Update dependencies", "Apply security patches", "Implement vulnerability scanning"},
		})
	}
	
	return issues
}

// GenerateRemediationPlan creates a comprehensive remediation plan
func (s *SecurityEngine) GenerateRemediationPlan(ctx context.Context, vulns []VulnerabilityInfo, codebase Codebase) (*RemediationPlan, error) {
	// Prioritize vulnerabilities
	priorities := s.riskAnalyzer.PrioritizeVulnerabilities(vulns)
	
	// Sort by priority
	sort.Slice(priorities, func(i, j int) bool {
		return priorities[i].Priority < priorities[j].Priority
	})
	
	var recipes []RemediationRecipe
	for _, priority := range priorities {
		if recipe, err := s.remediator.GenerateRemediation(priority.Vulnerability, codebase); err == nil {
			recipes = append(recipes, *recipe)
		}
	}
	
	plan := &RemediationPlan{
		ID:          generateID(),
		Vulnerabilities: vulns,
		Recipes:     recipes,
		Timeline:    s.createRemediationTimeline(priorities),
		CreatedAt:   time.Now(),
		Metadata: map[string]interface{}{
			"total_vulnerabilities": len(vulns),
			"total_recipes":         len(recipes),
		},
	}
	
	return plan, nil
}

// RemediationPlan represents a comprehensive plan for addressing vulnerabilities
type RemediationPlan struct {
	ID              string                 `json:"id"`
	Vulnerabilities []VulnerabilityInfo    `json:"vulnerabilities"`
	Recipes         []RemediationRecipe    `json:"recipes"`
	Timeline        RemediationTimeline    `json:"timeline"`
	EstimatedEffort EstimatedEffort        `json:"estimated_effort"`
	CreatedAt       time.Time              `json:"created_at"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// createRemediationTimeline creates a timeline for remediation activities
func (s *SecurityEngine) createRemediationTimeline(priorities []VulnerabilityPriority) RemediationTimeline {
	timeline := RemediationTimeline{
		Immediate: []string{},
		Short:     []string{},
		Medium:    []string{},
		Long:      []string{},
	}
	
	for _, priority := range priorities {
		cveID := priority.Vulnerability.CVE.ID
		
		switch priority.Urgency {
		case "immediate":
			timeline.Immediate = append(timeline.Immediate, cveID)
		case "short_term":
			timeline.Short = append(timeline.Short, cveID)
		case "medium_term":
			timeline.Medium = append(timeline.Medium, cveID)
		default:
			timeline.Long = append(timeline.Long, cveID)
		}
	}
	
	return timeline
}

// generateID generates a unique ID for plans and recipes
func generateID() string {
	return fmt.Sprintf("arf-sec-%d", time.Now().Unix())
}