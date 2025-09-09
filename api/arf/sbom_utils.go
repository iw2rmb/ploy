package arf

import "strings"

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
