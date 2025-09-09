package arf

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
	License      string   `json:"license"`
	Type         string   `json:"type"` // permissive, copyleft, proprietary, unknown
	Restrictions []string `json:"restrictions"`
	Risks        []string `json:"risks"`
	Compliance   string   `json:"compliance"` // compliant, non_compliant, review_required
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
	Compliant    bool               `json:"compliant"`
	Violations   []LicenseViolation `json:"violations"`
	ReviewNeeded []string           `json:"review_needed"`
	Summary      string             `json:"summary"`
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
			Mode            int    `json:"mode"`
			Type            string `json:"type"`
			LinkDestination string `json:"linkDestination,omitempty"`
			UserID          int    `json:"userID"`
			GroupID         int    `json:"groupID"`
			Size            int64  `json:"size"`
			MIMEType        string `json:"mimeType"`
			Digests         []struct {
				Algorithm string `json:"algorithm"`
				Value     string `json:"value"`
			} `json:"digests"`
		} `json:"metadata"`
		Contents string `json:"contents,omitempty"`
	} `json:"files,omitempty"`
	Distro struct {
		Name             string   `json:"name"`
		Version          string   `json:"version"`
		IDLike           []string `json:"idLike,omitempty"`
		VersionCodename  string   `json:"versionCodename,omitempty"`
		VersionID        string   `json:"versionID,omitempty"`
		HomeURL          string   `json:"homeURL,omitempty"`
		SupportURL       string   `json:"supportURL,omitempty"`
		BugReportURL     string   `json:"bugReportURL,omitempty"`
		PrivacyPolicyURL string   `json:"privacyPolicyURL,omitempty"`
	} `json:"distro,omitempty"`
	Descriptor struct {
		Name          string `json:"name"`
		Version       string `json:"version"`
		Configuration struct {
			ConfigPath        string   `json:"configPath"`
			VerboseOutput     bool     `json:"verboseOutput"`
			QuietOutput       bool     `json:"quietOutput"`
			CheckForAppUpdate bool     `json:"checkForAppUpdate"`
			OnlyFixed         bool     `json:"onlyFixed"`
			OnlyNotFixed      bool     `json:"onlyNotFixed"`
			OutputFormat      []string `json:"outputFormat"`
			OutputFile        string   `json:"outputFile"`
			FileMetadata      struct {
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
