package security

import "time"

// CVEInfo represents detailed CVE information
type CVEInfo struct {
	ID               string                 `json:"id"`
	Description      string                 `json:"description"`
	CVSS             CVSSScore              `json:"cvss"`
	AffectedPackages []AffectedPackage      `json:"affected_packages"`
	References       []CVEReference         `json:"references"`
	PublishedDate    time.Time              `json:"published_date"`
	Severity         string                 `json:"severity"`
	Modification     ModificationGuidance   `json:"modification"`
	Exploitability   ExploitabilityInfo     `json:"exploitability"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// CVSSScore represents CVSS vulnerability scoring
type CVSSScore struct {
	Version        string  `json:"version"`
	BaseScore      float64 `json:"base_score"`
	Vector         string  `json:"vector"`
	Impact         float64 `json:"impact_score"`
	Exploitability float64 `json:"exploitability_score"`
}

// AffectedPackage represents a package affected by a CVE
type AffectedPackage struct {
	Name             string   `json:"name"`
	Ecosystem        string   `json:"ecosystem"`
	AffectedVersions []string `json:"affected_versions"`
	FixedVersions    []string `json:"fixed_versions"`
	PatchAvailable   bool     `json:"patch_available"`
}

// CVEReference represents external references for a CVE
type CVEReference struct {
	Type string   `json:"type"`
	URL  string   `json:"url"`
	Tags []string `json:"tags,omitempty"`
}

// ModificationGuidance provides guidance for fixing vulnerabilities
type ModificationGuidance struct {
	Type           string          `json:"type"` // upgrade, patch, config, replace
	Instructions   string          `json:"instructions"`
	AutoApplicable bool            `json:"auto_applicable"`
	Recipe         *AutoModRecipe  `json:"recipe,omitempty"`
	Confidence     float64         `json:"confidence"`
	Effort         EstimatedEffort `json:"effort"`
}

// AutoModRecipe contains automatic modification instructions
type AutoModRecipe struct {
	Type         string             `json:"type"`
	Operations   []ModOperation     `json:"operations"`
	Validation   ValidationCriteria `json:"validation"`
	Rollback     RollbackStrategy   `json:"rollback"`
	Dependencies []string           `json:"dependencies"`
}

// ModOperation represents a specific modification action
type ModOperation struct {
	Action     string                 `json:"action"` // replace, upgrade, remove, configure
	Target     OperationTarget        `json:"target"`
	Parameters map[string]interface{} `json:"parameters"`
	Conditions []OperationCondition   `json:"conditions"`
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
	Type     string      `json:"type"`
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
	Required bool        `json:"required"`
}

// ValidationCriteria defines how to validate modification success
type ValidationCriteria struct {
	Tests       []ValidationTest `json:"tests"`
	Metrics     []string         `json:"metrics"`
	Timeout     time.Duration    `json:"timeout"`
	SuccessRate float64          `json:"required_success_rate"`
}

// RollbackStrategy defines how to roll back failed modifications
type RollbackStrategy struct {
	Type       string             `json:"type"`
	Steps      []RollbackStep     `json:"steps"`
	Timeout    time.Duration      `json:"timeout"`
	Validation ValidationCriteria `json:"validation"`
}

// RollbackStep represents a single rollback operation
type RollbackStep struct {
	Action     string                 `json:"action"`
	Target     OperationTarget        `json:"target"`
	Parameters map[string]interface{} `json:"parameters"`
	Order      int                    `json:"order"`
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
	CVE          CVEInfo                `json:"cve"`
	Package      Dependency             `json:"package"`
	Severity     string                 `json:"severity"`
	CVSS         float64                `json:"cvss_score"`
	Exploitable  bool                   `json:"exploitable"`
	HasFix       bool                   `json:"has_fix"`
	FixVersion   string                 `json:"fix_version,omitempty"`
	Modification ModificationGuidance   `json:"modification"`
	Context      SecurityContext        `json:"context"`
	Discovery    VulnerabilityDiscovery `json:"discovery"`
}

// SecurityContext provides context for security analysis
type SecurityContext struct {
	Environment     string   `json:"environment"`      // development, staging, production
	ExposureLevel   string   `json:"exposure_level"`   // internal, external, public
	DataSensitivity string   `json:"data_sensitivity"` // low, medium, high
	ComplianceReqs  []string `json:"compliance_requirements"`
	BusinessImpact  string   `json:"business_impact"`
}

// VulnerabilityDiscovery tracks how a vulnerability was discovered
type VulnerabilityDiscovery struct {
	Method     string    `json:"method"` // scan, manual, automated
	Tool       string    `json:"tool"`
	Timestamp  time.Time `json:"timestamp"`
	Confidence float64   `json:"confidence"`
	Source     string    `json:"source"`
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
	Summary          SecuritySummary          `json:"summary"`
	Vulnerabilities  []VulnerabilityInfo      `json:"vulnerabilities"`
	RiskAssessment   RiskAssessment           `json:"risk_assessment"`
	Recommendations  []SecurityRecommendation `json:"recommendations"`
	ComplianceReport ComplianceStatus         `json:"compliance"`
	GeneratedAt      time.Time                `json:"generated_at"`
	Metadata         map[string]interface{}   `json:"metadata"`
}

// SecuritySummary provides high-level security status
type SecuritySummary struct {
	TotalVulnerabilities int     `json:"total_vulnerabilities"`
	RiskScore            float64 `json:"risk_score"`
	FixableCount         int     `json:"fixable_count"`
	ExploitableCount     int     `json:"exploitable_count"`
	Status               string  `json:"status"`
}

// ModRecipe represents a complete modification solution
type ModRecipe struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Vulnerabilities []string               `json:"target_vulnerabilities"`
	Recipe          AutoModRecipe          `json:"recipe"`
	Metadata        map[string]interface{} `json:"metadata"`
	CreatedAt       time.Time              `json:"created_at"`
	Version         string                 `json:"version"`
}

// ModResult represents the outcome of applying a modification
type ModResult struct {
	Success              bool               `json:"success"`
	VulnerabilitiesFixed []string           `json:"vulnerabilities_fixed"`
	Errors               []string           `json:"errors"`
	Warnings             []string           `json:"warnings"`
	ValidationResults    []ValidationResult `json:"validation_results"`
	Duration             time.Duration      `json:"duration"`
	ChangeSummary        ChangeSummary      `json:"changes"`
	RollbackRequired     bool               `json:"rollback_required"`
}

// ChangeSummary summarizes changes made during modification
type ChangeSummary struct {
	FilesModified        []string           `json:"files_modified"`
	DependenciesChanged  []DependencyChange `json:"dependencies_changed"`
	ConfigurationChanges []ConfigChange     `json:"configuration_changes"`
	Statistics           ChangeStatistics   `json:"statistics"`
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

// ChangeStatistics provides statistics about modification changes
type ChangeStatistics struct {
	TotalFiles      int `json:"total_files_changed"`
	LinesAdded      int `json:"lines_added"`
	LinesRemoved    int `json:"lines_removed"`
	DependencyCount int `json:"dependencies_changed"`
	ConfigCount     int `json:"configurations_changed"`
}

// SBOMSecurityAnalysis represents security analysis of an SBOM
type SBOMSecurityAnalysis struct {
	Dependencies    []Dependency             `json:"dependencies"`
	Vulnerabilities []VulnerabilityMatch     `json:"vulnerabilities"`
	SecurityMetrics SBOMSecurityMetrics      `json:"metrics"`
	RiskAssessment  RiskAssessment           `json:"risk_assessment"`
	Recommendations []SecurityRecommendation `json:"recommendations"`
	AnalyzedAt      time.Time                `json:"analyzed_at"`
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
	TotalDependencies      int     `json:"total_dependencies"`
	VulnerableDependencies int     `json:"vulnerable_dependencies"`
	SecurityScore          float64 `json:"security_score"`
	LicenseIssues          int     `json:"license_issues"`
	OutdatedDependencies   int     `json:"outdated_dependencies"`
}

// ModPlan represents a comprehensive plan for addressing vulnerabilities
type ModPlan struct {
	ID              string                 `json:"id"`
	Vulnerabilities []VulnerabilityInfo    `json:"vulnerabilities"`
	Recipes         []ModRecipe            `json:"recipes"`
	Timeline        ModTimeline            `json:"timeline"`
	EstimatedEffort EstimatedEffort        `json:"estimated_effort"`
	CreatedAt       time.Time              `json:"created_at"`
	Metadata        map[string]interface{} `json:"metadata"`
}
