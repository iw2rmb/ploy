package supply

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SBOMGenerator handles comprehensive SBOM generation for all build artifacts
type SBOMGenerator struct {
	SyftPath string
}

// SBOMMetadata contains metadata about the SBOM generation process
type SBOMMetadata struct {
	GeneratedAt time.Time `json:"generated_at"`
	Tool        string    `json:"tool"`
	Version     string    `json:"tool_version"`
	Format      string    `json:"format"`
	Lane        string    `json:"lane,omitempty"`
	AppName     string    `json:"app_name,omitempty"`
	SHA         string    `json:"sha,omitempty"`
}

// SBOMGenerationOptions configures SBOM generation parameters
type SBOMGenerationOptions struct {
	OutputFormat    string // json, spdx-json, cyclone-dx-json
	IncludeSecrets  bool   // scan for secrets in SBOM
	IncludeLicenses bool   // include license information
	Lane           string // deployment lane (A-F)
	AppName        string // application name
	SHA            string // build SHA
}

// NewSBOMGenerator creates a new SBOM generator instance
func NewSBOMGenerator() *SBOMGenerator {
	return &SBOMGenerator{
		SyftPath: "syft", // Default to PATH lookup
	}
}

// GenerateForFile generates comprehensive SBOM for file-based artifacts (Lanes A, B, C, D, F)
func (s *SBOMGenerator) GenerateForFile(artifactPath string, options SBOMGenerationOptions) error {
	outputPath := artifactPath + ".sbom.json"
	
	// Build syft command with modern syntax
	args := []string{
		"scan",
		artifactPath,
		"-o", options.OutputFormat,
		"--file", outputPath,
	}
	
	// Note: Modern syft uses automatic cataloger selection
	// Legacy --catalogers and --select-catalogers flags are deprecated
	
	cmd := exec.Command(s.SyftPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("syft execution failed: %w, output: %s", err, string(output))
	}
	
	// Enhance SBOM with additional metadata
	return s.enhanceSBOM(outputPath, options)
}

// GenerateForContainer generates comprehensive SBOM for container images (Lane E)
func (s *SBOMGenerator) GenerateForContainer(imageTag string, options SBOMGenerationOptions) error {
	// Generate unique output path for container SBOMs
	sanitizedTag := strings.ReplaceAll(strings.ReplaceAll(imageTag, "/", "-"), ":", "-")
	outputPath := filepath.Join("/tmp", fmt.Sprintf("%s.sbom.json", sanitizedTag))
	
	args := []string{
		"scan",
		imageTag,
		"-o", options.OutputFormat,
		"--file", outputPath,
	}
	
	// Note: Modern syft automatically selects appropriate catalogers
	
	cmd := exec.Command(s.SyftPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("syft container analysis failed: %w, output: %s", err, string(output))
	}
	
	return s.enhanceSBOM(outputPath, options)
}

// GenerateForSourceCode generates SBOM for source code dependencies
func (s *SBOMGenerator) GenerateForSourceCode(sourcePath string, options SBOMGenerationOptions) error {
	outputPath := filepath.Join(sourcePath, ".sbom.json")
	
	args := []string{
		"scan",
		sourcePath,
		"-o", options.OutputFormat,
		"--file", outputPath,
	}
	
	// Modern syft automatically detects and uses appropriate catalogers
	// based on the source code language and build files
	
	cmd := exec.Command(s.SyftPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("source code SBOM generation failed: %w, output: %s", err, string(output))
	}
	
	return s.enhanceSBOM(outputPath, options)
}

// enhanceSBOM adds additional metadata to generated SBOM files
func (s *SBOMGenerator) enhanceSBOM(sbomPath string, options SBOMGenerationOptions) error {
	// Read existing SBOM
	var sbomData map[string]interface{}
	content, err := exec.Command("cat", sbomPath).Output()
	if err != nil {
		return fmt.Errorf("failed to read SBOM file: %w", err)
	}
	
	if err := json.Unmarshal(content, &sbomData); err != nil {
		return fmt.Errorf("failed to parse SBOM JSON: %w", err)
	}
	
	// Add Ploy-specific metadata
	metadata := SBOMMetadata{
		GeneratedAt: time.Now(),
		Tool:        "syft",
		Format:      options.OutputFormat,
		Lane:        options.Lane,
		AppName:     options.AppName,
		SHA:         options.SHA,
	}
	
	// Get syft version
	if versionOut, err := exec.Command(s.SyftPath, "version").Output(); err == nil {
		lines := strings.Split(string(versionOut), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Version:") {
				metadata.Version = strings.TrimSpace(strings.Split(line, ":")[1])
				break
			}
		}
	}
	
	// Inject metadata into SBOM
	if sbomData["ploy_metadata"] == nil {
		sbomData["ploy_metadata"] = metadata
	}
	
	// Write enhanced SBOM back
	enhancedContent, err := json.MarshalIndent(sbomData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal enhanced SBOM: %w", err)
	}
	
	return exec.Command("sh", "-c", fmt.Sprintf("echo '%s' > %s", 
		strings.ReplaceAll(string(enhancedContent), "'", "'\"'\"'"), sbomPath)).Run()
}

// hasFile checks if a file exists in the given directory
func (s *SBOMGenerator) hasFile(dir, filename string) bool {
	_, err := exec.Command("test", "-f", filepath.Join(dir, filename)).Output()
	return err == nil
}

// DefaultSBOMOptions returns sensible defaults for SBOM generation
func DefaultSBOMOptions() SBOMGenerationOptions {
	return SBOMGenerationOptions{
		OutputFormat:    "spdx-json",
		IncludeSecrets:  false,
		IncludeLicenses: true,
	}
}

// GenerateSBOM is a convenience function for general SBOM generation
func GenerateSBOM(artifactPath string, lane string, appName string, sha string) error {
	generator := NewSBOMGenerator()
	options := DefaultSBOMOptions()
	options.Lane = lane
	options.AppName = appName
	options.SHA = sha
	
	if strings.Contains(artifactPath, ":") {
		// Container image
		return generator.GenerateForContainer(artifactPath, options)
	} else {
		// File-based artifact
		return generator.GenerateForFile(artifactPath, options)
	}
}