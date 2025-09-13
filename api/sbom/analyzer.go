package sbom

import (
	"encoding/json"
	"fmt"
	"os"
)

// SyftSBOMAnalyzer implements SBOMSecurityAnalyzer using syft-generated SBOMs
type SyftSBOMAnalyzer struct{}

func NewSyftSBOMAnalyzer() *SyftSBOMAnalyzer { return &SyftSBOMAnalyzer{} }

func (s *SyftSBOMAnalyzer) AnalyzeSBOM(sbomPath string) (*SBOMSecurityAnalysis, error) {
	sbomData, err := s.readSBOMFile(sbomPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SBOM: %w", err)
	}
	deps, err := s.ExtractDependencies(sbomData)
	if err != nil {
		return nil, fmt.Errorf("failed to extract dependencies: %w", err)
	}
	vulns, _ := s.CorrelateVulnerabilities(deps)
	metrics := s.CalculateSecurityMetrics(deps, vulns)
	assessment := s.AssessRisk(&SBOMSecurityAnalysis{Dependencies: deps, Vulnerabilities: vulns, SecurityMetrics: metrics})
	return &SBOMSecurityAnalysis{
		Dependencies:    deps,
		Vulnerabilities: vulns,
		SecurityMetrics: metrics,
		RiskAssessment:  assessment,
	}, nil
}

func (s *SyftSBOMAnalyzer) readSBOMFile(path string) (map[string]interface{}, error) {
	if path == "" {
		return nil, fmt.Errorf("invalid SBOM path")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}
