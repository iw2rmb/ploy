package arf

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
)

// SBOMParser handles SBOM file parsing and dependency extraction
type SBOMParser struct{}

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
