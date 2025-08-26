package arf

import (
	"context"
	"fmt"
	"testing"
)

func TestSecurityEngine_ScanForVulnerabilities(t *testing.T) {
	engine := NewSecurityEngine()
	ctx := context.Background()

	tests := []struct {
		name     string
		target   string
		scanType string
		wantErr  bool
	}{
		{
			name:     "Valid container scan",
			target:   "nginx:1.20",
			scanType: "container",
			wantErr:  false,
		},
		{
			name:     "Valid filesystem scan",
			target:   "/app",
			scanType: "filesystem",
			wantErr:  false,
		},
		{
			name:     "Invalid target",
			target:   "",
			scanType: "container",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := engine.ScanForVulnerabilities(ctx, tt.target, tt.scanType)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("ScanForVulnerabilities() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !tt.wantErr && report != nil {
				// Validate report structure
				if report.Summary.TotalVulnerabilities == 0 {
					t.Log("No vulnerabilities found - this may be expected")
				}
				
				if report.GeneratedAt.IsZero() {
					t.Error("Expected GeneratedAt to be set")
				}
				
				if report.Summary.Status == "" {
					t.Error("Expected status to be set")
				}
				
				if report.Summary.RiskScore < 0 {
					t.Error("Expected risk score to be non-negative")
				}
			}
		})
	}
}

func TestSecurityEngine_GenerateRemediationPlan(t *testing.T) {
	engine := NewSecurityEngine()
	ctx := context.Background()

	vulns := []VulnerabilityInfo{
		{
			CVE: CVEInfo{
				ID:          "CVE-2024-0001",
				Description: "Test vulnerability",
				Severity:    "high",
			},
			Package: Dependency{
				Name:      "test-package",
				Version:   "1.0.0",
				Ecosystem: "npm",
			},
			Severity:   "HIGH",
			CVSS:       7.5,
			FixVersion: "1.0.1",
			HasFix:     true,
		},
	}

	codebase := Codebase{
		Repository: "test-repo",
		Language:   "javascript",
		Branch:     "main",
	}

	plan, err := engine.GenerateRemediationPlan(ctx, vulns, codebase)
	if err != nil {
		t.Fatalf("GenerateRemediationPlan() error = %v", err)
	}

	if plan == nil {
		t.Fatal("Expected non-nil remediation plan")
	}

	if plan.ID == "" {
		t.Error("Expected plan ID to be set")
	}

	if len(plan.Vulnerabilities) != len(vulns) {
		t.Errorf("Expected %d vulnerabilities in plan, got %d", len(vulns), len(plan.Vulnerabilities))
	}

	if plan.EstimatedEffort.TimeMinutes <= 0 {
		t.Error("Expected positive estimated time")
	}

	if plan.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
}

func TestSecurityEngine_VulnerabilityPrioritization(t *testing.T) {
	engine := NewSecurityEngine()

	vulns := []VulnerabilityInfo{
		{
			CVE:      CVEInfo{ID: "CVE-2024-0001", Severity: "critical"},
			Severity: "CRITICAL",
			CVSS:     9.5,
			Package:  Dependency{Name: "critical-lib", Version: "1.0.0"},
		},
		{
			CVE:      CVEInfo{ID: "CVE-2024-0002", Severity: "high"},
			Severity: "HIGH",
			CVSS:     7.5,
			Package:  Dependency{Name: "high-lib", Version: "1.0.0"},
		},
		{
			CVE:      CVEInfo{ID: "CVE-2024-0003", Severity: "medium"},
			Severity: "MEDIUM",
			CVSS:     5.0,
			Package:  Dependency{Name: "medium-lib", Version: "1.0.0"},
		},
	}

	priorities := engine.prioritizeVulnerabilities(vulns)

	if len(priorities) != len(vulns) {
		t.Errorf("Expected %d priorities, got %d", len(vulns), len(priorities))
	}

	// Verify prioritization order (higher CVSS = higher priority = lower priority number)
	if len(priorities) >= 2 {
		if priorities[0].Priority > priorities[1].Priority {
			t.Error("Expected vulnerabilities to be sorted by priority")
		}
	}

	// Verify urgency mapping
	for _, priority := range priorities {
		switch {
		case priority.Vulnerability.CVSS >= 9.0:
			if priority.Urgency != "critical" {
				t.Errorf("Expected critical urgency for CVSS %.1f, got %s", priority.Vulnerability.CVSS, priority.Urgency)
			}
		case priority.Vulnerability.CVSS >= 7.0:
			if priority.Urgency != "high" {
				t.Errorf("Expected high urgency for CVSS %.1f, got %s", priority.Vulnerability.CVSS, priority.Urgency)
			}
		case priority.Vulnerability.CVSS >= 4.0:
			if priority.Urgency != "medium" {
				t.Errorf("Expected medium urgency for CVSS %.1f, got %s", priority.Vulnerability.CVSS, priority.Urgency)
			}
		default:
			if priority.Urgency != "low" {
				t.Errorf("Expected low urgency for CVSS %.1f, got %s", priority.Vulnerability.CVSS, priority.Urgency)
			}
		}
	}
}

func TestSecurityEngine_RemediationTimeline(t *testing.T) {
	engine := NewSecurityEngine()

	priorities := []VulnerabilityPriority{
		{
			Vulnerability: VulnerabilityInfo{CVE: CVEInfo{ID: "CVE-2024-0001"}},
			Priority:      1,
			Urgency:       "critical",
		},
		{
			Vulnerability: VulnerabilityInfo{CVE: CVEInfo{ID: "CVE-2024-0002"}},
			Priority:      2,
			Urgency:       "high",
		},
		{
			Vulnerability: VulnerabilityInfo{CVE: CVEInfo{ID: "CVE-2024-0003"}},
			Priority:      3,
			Urgency:       "medium",
		},
		{
			Vulnerability: VulnerabilityInfo{CVE: CVEInfo{ID: "CVE-2024-0004"}},
			Priority:      4,
			Urgency:       "low",
		},
	}

	timeline := engine.createRemediationTimeline(priorities)

	// Verify critical vulnerabilities are in immediate timeline
	if len(timeline.Immediate) == 0 {
		t.Error("Expected critical vulnerabilities in immediate timeline")
	}

	// Verify timeline structure
	totalVulns := len(timeline.Immediate) + len(timeline.Short) + len(timeline.Medium) + len(timeline.Long)
	if totalVulns != len(priorities) {
		t.Errorf("Expected %d vulnerabilities in timeline, got %d", len(priorities), totalVulns)
	}

	// Verify CVE-2024-0001 is in immediate
	found := false
	for _, cveID := range timeline.Immediate {
		if cveID == "CVE-2024-0001" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected CVE-2024-0001 to be in immediate timeline")
	}
}

func TestSecurityEngine_EffortCalculation(t *testing.T) {
	engine := NewSecurityEngine()

	vulns := []VulnerabilityInfo{
		{
			CVE:     CVEInfo{ID: "CVE-2024-0001"},
			HasFix:  true,
			Package: Dependency{Name: "fixable-lib"},
		},
		{
			CVE:     CVEInfo{ID: "CVE-2024-0002"},
			HasFix:  false,
			Package: Dependency{Name: "manual-lib"},
		},
	}

	effort := engine.calculateEffort(vulns)

	// Verify effort calculation
	if effort.TimeMinutes <= 0 {
		t.Error("Expected positive time estimate")
	}

	if effort.Level == "" {
		t.Error("Expected effort level to be set")
	}

	if effort.Complexity <= 0 {
		t.Error("Expected positive complexity score")
	}

	if len(effort.Resources) == 0 {
		t.Error("Expected resources to be specified")
	}

	// Verify manual fixes increase complexity
	expectedTime := 30 + 120 // 30 min for fixable + 120 min for manual
	if effort.TimeMinutes != expectedTime {
		t.Errorf("Expected %d minutes, got %d", expectedTime, effort.TimeMinutes)
	}
}

func BenchmarkSecurityEngine_ScanForVulnerabilities(b *testing.B) {
	engine := NewSecurityEngine()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.ScanForVulnerabilities(ctx, "nginx:latest", "container")
	}
}

func BenchmarkSecurityEngine_PrioritizeVulnerabilities(b *testing.B) {
	engine := NewSecurityEngine()

	vulns := make([]VulnerabilityInfo, 100)
	for i := 0; i < 100; i++ {
		vulns[i] = VulnerabilityInfo{
			CVE:      CVEInfo{ID: fmt.Sprintf("CVE-2024-%04d", i), Severity: "medium"},
			Severity: "MEDIUM",
			CVSS:     float64(4.0 + (i%5)),
			Package:  Dependency{Name: fmt.Sprintf("lib-%d", i), Version: "1.0.0"},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.prioritizeVulnerabilities(vulns)
	}
}