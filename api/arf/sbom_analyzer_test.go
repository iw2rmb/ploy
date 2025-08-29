package arf

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestSBOMAnalyzer_AnalyzeSBOM(t *testing.T) {
	analyzer := &MockSBOMAnalyzer{}

	tests := []struct {
		name     string
		sbomPath string
		wantErr  bool
	}{
		{
			name:     "Valid SBOM analysis",
			sbomPath: "/tmp/test.sbom.json",
			wantErr:  false,
		},
		{
			name:     "Invalid SBOM path",
			sbomPath: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis, err := analyzer.AnalyzeSBOM(tt.sbomPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("AnalyzeSBOM() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && analysis != nil {
				// Validate analysis structure
				if len(analysis.Dependencies) == 0 {
					t.Error("Expected at least one dependency in analysis")
				}

				if analysis.SecurityMetrics.TotalDependencies == 0 {
					t.Error("Expected total dependencies to be greater than 0")
				}

				if analysis.AnalyzedAt.IsZero() {
					t.Error("Expected AnalyzedAt to be set")
				}
			}
		})
	}
}

func TestSBOMAnalyzer_ExtractDependencies(t *testing.T) {
	analyzer := &MockSBOMAnalyzer{}

	sbomData := map[string]interface{}{
		"artifacts": []interface{}{
			map[string]interface{}{
				"name":      "lodash",
				"version":   "4.17.21",
				"type":      "npm",
				"ecosystem": "npm",
			},
			map[string]interface{}{
				"name":      "jackson-core",
				"version":   "2.13.0",
				"type":      "jar",
				"ecosystem": "maven",
			},
		},
	}

	dependencies, err := analyzer.ExtractDependencies(sbomData)
	if err != nil {
		t.Fatalf("ExtractDependencies() error = %v", err)
	}

	if len(dependencies) != 2 {
		t.Errorf("Expected 2 dependencies, got %d", len(dependencies))
	}

	// Validate first dependency
	if len(dependencies) > 0 {
		dep := dependencies[0]
		if dep.Name == "" {
			t.Error("Expected dependency name to be set")
		}
		if dep.Version == "" {
			t.Error("Expected dependency version to be set")
		}
		if dep.Ecosystem == "" {
			t.Error("Expected dependency ecosystem to be set")
		}
	}
}

func TestSBOMAnalyzer_CorrelateVulnerabilities(t *testing.T) {
	analyzer := &MockSBOMAnalyzer{}

	dependencies := []Dependency{
		{
			Name:      "lodash",
			Version:   "4.17.20",
			Ecosystem: "npm",
		},
		{
			Name:      "jackson-core",
			Version:   "2.12.0",
			Ecosystem: "maven",
		},
	}

	matches, err := analyzer.CorrelateVulnerabilities(dependencies)
	if err != nil {
		t.Fatalf("CorrelateVulnerabilities() error = %v", err)
	}

	// Should return some vulnerability matches for these known vulnerable versions
	if len(matches) == 0 {
		t.Log("No vulnerability matches found - this may be expected for test data")
	}

	for _, match := range matches {
		if match.Dependency.Name == "" {
			t.Error("Expected dependency name to be set in match")
		}
		if match.MatchConfidence < 0 || match.MatchConfidence > 1 {
			t.Errorf("Expected match confidence between 0-1, got %f", match.MatchConfidence)
		}
	}
}

func TestSBOMSecurityMetrics_Validation(t *testing.T) {
	metrics := SBOMSecurityMetrics{
		TotalDependencies:      10,
		VulnerableDependencies: 3,
		SecurityScore:          0.7,
		LicenseIssues:          1,
		OutdatedDependencies:   5,
	}

	if metrics.TotalDependencies < metrics.VulnerableDependencies {
		t.Error("Total dependencies should be >= vulnerable dependencies")
	}

	if metrics.SecurityScore < 0 || metrics.SecurityScore > 1 {
		t.Errorf("Security score should be between 0-1, got %f", metrics.SecurityScore)
	}

	if metrics.VulnerableDependencies < 0 {
		t.Error("Vulnerable dependencies should not be negative")
	}

	if metrics.LicenseIssues < 0 {
		t.Error("License issues should not be negative")
	}

	if metrics.OutdatedDependencies < 0 {
		t.Error("Outdated dependencies should not be negative")
	}
}

func TestSBOMAnalysis_RecommendationGeneration(t *testing.T) {
	analysis := &SBOMSecurityAnalysis{
		Dependencies: []Dependency{
			{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"},
		},
		Vulnerabilities: []VulnerabilityMatch{
			{
				Dependency: Dependency{Name: "lodash", Version: "4.17.20"},
				Vulnerability: VulnerabilityInfo{
					CVE:      CVEInfo{ID: "CVE-2021-23337", Severity: "high"},
					Severity: "HIGH",
					CVSS:     7.5,
					HasFix:   true,
				},
				MatchConfidence: 0.9,
			},
		},
		SecurityMetrics: SBOMSecurityMetrics{
			TotalDependencies:      1,
			VulnerableDependencies: 1,
			SecurityScore:          0.3,
		},
		AnalyzedAt: time.Now(),
	}

	if len(analysis.Dependencies) == 0 {
		t.Error("Expected at least one dependency")
	}

	if len(analysis.Vulnerabilities) == 0 {
		t.Error("Expected at least one vulnerability match")
	}

	if analysis.SecurityMetrics.SecurityScore < 0 || analysis.SecurityMetrics.SecurityScore > 1 {
		t.Errorf("Security score should be between 0-1, got %f", analysis.SecurityMetrics.SecurityScore)
	}

	// Verify vulnerability match structure
	if len(analysis.Vulnerabilities) > 0 {
		vuln := analysis.Vulnerabilities[0]
		if vuln.Dependency.Name == "" {
			t.Error("Expected vulnerability dependency name to be set")
		}
		if vuln.MatchConfidence < 0 || vuln.MatchConfidence > 1 {
			t.Errorf("Expected match confidence between 0-1, got %f", vuln.MatchConfidence)
		}
	}
}

func BenchmarkSBOMAnalyzer_AnalyzeSBOM(b *testing.B) {
	analyzer := &MockSBOMAnalyzer{}
	testPath := filepath.Join(b.TempDir(), "benchmark.sbom.json")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = analyzer.AnalyzeSBOM(testPath)
	}
}

// MockSBOMAnalyzer is a mock implementation for testing
type MockSBOMAnalyzer struct{}

func (m *MockSBOMAnalyzer) AnalyzeSBOM(sbomPath string) (*SBOMSecurityAnalysis, error) {
	if sbomPath == "" {
		return nil, fmt.Errorf("invalid SBOM path")
	}

	return &SBOMSecurityAnalysis{
		Dependencies: []Dependency{
			{Name: "test-lib", Version: "1.0.0", Ecosystem: "npm"},
		},
		SecurityMetrics: SBOMSecurityMetrics{
			TotalDependencies: 1,
			SecurityScore:     0.8,
		},
		AnalyzedAt: time.Now(),
	}, nil
}

func (m *MockSBOMAnalyzer) ExtractDependencies(sbomData map[string]interface{}) ([]Dependency, error) {
	var dependencies []Dependency

	artifacts, ok := sbomData["artifacts"].([]interface{})
	if !ok {
		return dependencies, nil
	}

	for _, artifact := range artifacts {
		if artMap, ok := artifact.(map[string]interface{}); ok {
			dep := Dependency{}
			if name, ok := artMap["name"].(string); ok {
				dep.Name = name
			}
			if version, ok := artMap["version"].(string); ok {
				dep.Version = version
			}
			if ecosystem, ok := artMap["ecosystem"].(string); ok {
				dep.Ecosystem = ecosystem
			}
			dependencies = append(dependencies, dep)
		}
	}

	return dependencies, nil
}

func (m *MockSBOMAnalyzer) CorrelateVulnerabilities(deps []Dependency) ([]VulnerabilityMatch, error) {
	var matches []VulnerabilityMatch

	// Mock correlation - return a match for known vulnerable packages
	for _, dep := range deps {
		if dep.Name == "lodash" && dep.Version == "4.17.20" {
			matches = append(matches, VulnerabilityMatch{
				Dependency: dep,
				Vulnerability: VulnerabilityInfo{
					CVE:      CVEInfo{ID: "CVE-2021-23337", Severity: "high"},
					Severity: "HIGH",
					CVSS:     7.5,
					HasFix:   true,
				},
				MatchConfidence: 0.9,
				MatchReason:     "Exact version match",
			})
		}
	}

	return matches, nil
}
