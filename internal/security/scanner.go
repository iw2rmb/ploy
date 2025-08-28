package security

import (
	"fmt"
	"time"

	"github.com/iw2rmb/ploy/internal/harbor"
)

// VulnerabilityScanner manages vulnerability scanning operations
type VulnerabilityScanner struct {
	client            *harbor.Client
	severityThreshold string
}

// ScanResult represents the result of a vulnerability scan
type ScanResult struct {
	Passed          bool                    `json:"passed"`
	Severity        string                  `json:"severity"`
	VulnCount       int                     `json:"vulnerability_count"`
	FixableCount    int                     `json:"fixable_count"`
	HighSeverity    bool                    `json:"high_severity"`
	CriticalCount   int                     `json:"critical_count"`
	HighCount       int                     `json:"high_count"`
	MediumCount     int                     `json:"medium_count"`
	LowCount        int                     `json:"low_count"`
	Vulnerabilities []harbor.Vulnerability  `json:"vulnerabilities"`
	Report          *harbor.ScanReport      `json:"report,omitempty"`
}

// NewVulnerabilityScanner creates a new vulnerability scanner
func NewVulnerabilityScanner(client *harbor.Client) *VulnerabilityScanner {
	return &VulnerabilityScanner{
		client:            client,
		severityThreshold: "HIGH", // Default: block HIGH and CRITICAL
	}
}

// SetSeverityThreshold sets the severity threshold for blocking deployments
func (v *VulnerabilityScanner) SetSeverityThreshold(threshold string) {
	v.severityThreshold = threshold
}

// ScanAndValidate performs vulnerability scan and validates against policy
func (v *VulnerabilityScanner) ScanAndValidate(projectName, repository, tag string) (*ScanResult, error) {
	// Trigger vulnerability scan
	if err := v.client.TriggerScan(projectName, repository, tag); err != nil {
		return nil, fmt.Errorf("failed to trigger vulnerability scan: %w", err)
	}
	
	// Wait for scan to complete with timeout
	report, err := v.waitForScanComplete(projectName, repository, tag, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("vulnerability scan failed: %w", err)
	}
	
	// Analyze scan results
	result := v.analyzeScanReport(report)
	
	// Apply severity threshold policy
	if v.exceedsSeverityThreshold(result) {
		result.Passed = false
		return result, fmt.Errorf("image contains vulnerabilities exceeding %s severity threshold", v.severityThreshold)
	}
	
	result.Passed = true
	return result, nil
}

// waitForScanComplete waits for vulnerability scan to complete
func (v *VulnerabilityScanner) waitForScanComplete(projectName, repository, tag string, timeout time.Duration) (*harbor.ScanReport, error) {
	return v.client.WaitForScanComplete(projectName, repository, tag, timeout)
}

// analyzeScanReport analyzes the vulnerability scan report
func (v *VulnerabilityScanner) analyzeScanReport(report *harbor.ScanReport) *ScanResult {
	if report == nil {
		return &ScanResult{
			Passed:       true,
			VulnCount:    0,
			FixableCount: 0,
		}
	}
	
	result := &ScanResult{
		Severity:        report.Severity,
		VulnCount:       len(report.Vulnerabilities),
		FixableCount:    report.Summary.Fixable,
		Vulnerabilities: report.Vulnerabilities,
		Report:          report,
	}
	
	// Count vulnerabilities by severity
	severityCounts := make(map[string]int)
	for _, vuln := range report.Vulnerabilities {
		severityCounts[vuln.Severity]++
	}
	
	result.CriticalCount = severityCounts["CRITICAL"]
	result.HighCount = severityCounts["HIGH"]
	result.MediumCount = severityCounts["MEDIUM"]
	result.LowCount = severityCounts["LOW"]
	
	// Check if has high severity vulnerabilities
	result.HighSeverity = result.CriticalCount > 0 || result.HighCount > 0
	
	return result
}

// exceedsSeverityThreshold checks if vulnerabilities exceed the configured threshold
func (v *VulnerabilityScanner) exceedsSeverityThreshold(result *ScanResult) bool {
	switch v.severityThreshold {
	case "CRITICAL":
		return result.CriticalCount > 0
	case "HIGH":
		return result.CriticalCount > 0 || result.HighCount > 0
	case "MEDIUM":
		return result.CriticalCount > 0 || result.HighCount > 0 || result.MediumCount > 0
	case "LOW":
		return result.VulnCount > 0
	default:
		// Default to HIGH threshold
		return result.CriticalCount > 0 || result.HighCount > 0
	}
}

// GetSeverityThreshold returns the current severity threshold
func (v *VulnerabilityScanner) GetSeverityThreshold() string {
	return v.severityThreshold
}

// ValidateForPlatform validates vulnerability scan results for platform services
// Platform services have strict security requirements (HIGH threshold)
func (v *VulnerabilityScanner) ValidateForPlatform(projectName, repository, tag string) (*ScanResult, error) {
	v.SetSeverityThreshold("HIGH")
	return v.ScanAndValidate(projectName, repository, tag)
}

// ValidateForUserApps validates vulnerability scan results for user applications
// User applications have relaxed security requirements (CRITICAL threshold)
func (v *VulnerabilityScanner) ValidateForUserApps(projectName, repository, tag string) (*ScanResult, error) {
	v.SetSeverityThreshold("CRITICAL")
	return v.ScanAndValidate(projectName, repository, tag)
}

// GetVulnerabilitySummary returns a human-readable summary of vulnerabilities
func (v *VulnerabilityScanner) GetVulnerabilitySummary(result *ScanResult) string {
	if result == nil || result.VulnCount == 0 {
		return "No vulnerabilities found"
	}
	
	summary := fmt.Sprintf("Found %d vulnerabilities (%d fixable)", 
		result.VulnCount, result.FixableCount)
	
	if result.CriticalCount > 0 || result.HighCount > 0 || result.MediumCount > 0 || result.LowCount > 0 {
		summary += fmt.Sprintf(" - Critical: %d, High: %d, Medium: %d, Low: %d",
			result.CriticalCount, result.HighCount, result.MediumCount, result.LowCount)
	}
	
	return summary
}

// HasBlockingVulnerabilities returns true if the scan has vulnerabilities that should block deployment
func (v *VulnerabilityScanner) HasBlockingVulnerabilities(result *ScanResult) bool {
	if result == nil {
		return false
	}
	return v.exceedsSeverityThreshold(result)
}