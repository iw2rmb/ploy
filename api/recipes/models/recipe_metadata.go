package models

import (
	"fmt"
	"regexp"
	"strings"
)

// RecipeMetadata contains human-readable recipe information
type RecipeMetadata struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Author      string `json:"author" yaml:"author"`
	Version     string `json:"version" yaml:"version"`
	License     string `json:"license,omitempty" yaml:"license,omitempty"`
	Homepage    string `json:"homepage,omitempty" yaml:"homepage,omitempty"`
	Repository  string `json:"repository,omitempty" yaml:"repository,omitempty"`

	// Categorization and discovery
	Tags       []string `json:"tags" yaml:"tags"`
	Categories []string `json:"categories" yaml:"categories"`
	Languages  []string `json:"languages" yaml:"languages"`
	Frameworks []string `json:"frameworks,omitempty" yaml:"frameworks,omitempty"`

	// Compatibility and requirements
	MinPlatform  string   `json:"min_platform,omitempty" yaml:"min_platform,omitempty"`
	MaxPlatform  string   `json:"max_platform,omitempty" yaml:"max_platform,omitempty"`
	Requirements []string `json:"requirements,omitempty" yaml:"requirements,omitempty"`
}

// Validate performs validation on recipe metadata
func (m *RecipeMetadata) Validate() error {
	// Validate name format (lowercase, alphanumeric with hyphens)
	if !isValidRecipeName(m.Name) {
		return fmt.Errorf("invalid recipe name: must be lowercase alphanumeric with hyphens, 2-63 characters")
	}

	// Validate version format (semantic versioning)
	if m.Version != "" && !isValidSemanticVersion(m.Version) {
		return fmt.Errorf("invalid version format: must follow semantic versioning (e.g., 1.0.0)")
	}

	// Validate author
	if m.Author == "" {
		return fmt.Errorf("author is required")
	}

	// Validate license if provided; normalize invalid to empty (non-fatal)
	if m.License != "" && !isValidLicense(m.License) {
		m.License = ""
	}

	// Validate URLs if provided
	if m.Homepage != "" && !isValidURL(m.Homepage) {
		return fmt.Errorf("invalid homepage URL")
	}

	if m.Repository != "" && !isValidRepositoryURL(m.Repository) {
		return fmt.Errorf("invalid repository URL")
	}

	// Validate categories
	for _, category := range m.Categories {
		if !isValidCategory(category) {
			return fmt.Errorf("invalid category: %s", category)
		}
	}

	// Validate languages
	for _, lang := range m.Languages {
		if !isValidLanguage(lang) {
			return fmt.Errorf("unsupported language: %s", lang)
		}
	}

	return nil
}

// GetSearchableText returns all text fields concatenated for full-text search
func (m *RecipeMetadata) GetSearchableText() string {
	parts := []string{
		m.Name,
		m.Description,
		m.Author,
		strings.Join(m.Tags, " "),
		strings.Join(m.Categories, " "),
		strings.Join(m.Languages, " "),
		strings.Join(m.Frameworks, " "),
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// HasTag checks if the metadata contains a specific tag
func (m *RecipeMetadata) HasTag(tag string) bool {
	for _, t := range m.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// HasCategory checks if the metadata contains a specific category
func (m *RecipeMetadata) HasCategory(category string) bool {
	for _, c := range m.Categories {
		if strings.EqualFold(c, category) {
			return true
		}
	}
	return false
}

// SupportsLanguage checks if the recipe supports a specific language
func (m *RecipeMetadata) SupportsLanguage(language string) bool {
	for _, l := range m.Languages {
		if strings.EqualFold(l, language) {
			return true
		}
	}
	return false
}

// SupportsFramework checks if the recipe supports a specific framework
func (m *RecipeMetadata) SupportsFramework(framework string) bool {
	for _, f := range m.Frameworks {
		if strings.EqualFold(f, framework) {
			return true
		}
	}
	return false
}

// Helper functions for validation

func isValidRecipeName(name string) bool {
	if len(name) < 2 || len(name) > 63 {
		return false
	}
	match, _ := regexp.MatchString("^[a-z0-9]+(-[a-z0-9]+)*$", name)
	return match
}

func isValidSemanticVersion(version string) bool {
	// Simplified semantic version validation
	match, _ := regexp.MatchString(`^v?\d+\.\d+\.\d+(-[a-zA-Z0-9\.\-]+)?(\+[a-zA-Z0-9\.\-]+)?$`, version)
	return match
}

func isValidLicense(license string) bool {
	// Common open source licenses
	validLicenses := []string{
		"MIT", "Apache-2.0", "GPL-3.0", "BSD-3-Clause", "BSD-2-Clause",
		"ISC", "MPL-2.0", "LGPL-3.0", "AGPL-3.0", "Unlicense", "CC0-1.0",
	}
	for _, valid := range validLicenses {
		if strings.EqualFold(license, valid) {
			return true
		}
	}
	return false
}

func isValidURL(url string) bool {
	match, _ := regexp.MatchString(`^https?://[^\s/$.?#].[^\s]*$`, url)
	return match
}

func isValidRepositoryURL(url string) bool {
	// Support common git repository formats
	patterns := []string{
		`^https?://github\.com/[\w\-]+/[\w\-]+`,
		`^https?://gitlab\.com/[\w\-]+/[\w\-]+`,
		`^https?://bitbucket\.org/[\w\-]+/[\w\-]+`,
		`^git@github\.com:[\w\-]+/[\w\-]+\.git$`,
		`^git@gitlab\.com:[\w\-]+/[\w\-]+\.git$`,
	}

	for _, pattern := range patterns {
		if match, _ := regexp.MatchString(pattern, url); match {
			return true
		}
	}
	return false
}

func isValidCategory(category string) bool {
	validCategories := []string{
		"language-upgrade",
		"framework-migration",
		"security-fix",
		"code-cleanup",
		"performance-optimization",
		"api-migration",
		"dependency-update",
		"modernization",
		"refactoring",
		"testing",
		"documentation",
		"formatting",
		"best-practices",
	}

	for _, valid := range validCategories {
		if strings.EqualFold(category, valid) {
			return true
		}
	}
	return false
}

func isValidLanguage(language string) bool {
	validLanguages := []string{
		"java", "kotlin", "groovy", "scala",
		"go", "golang",
		"python", "python2", "python3",
		"javascript", "typescript", "js", "ts",
		"c", "cpp", "c++",
		"rust",
		"ruby",
		"php",
		"swift",
		"csharp", "c#",
		"shell", "bash", "sh",
	}

	for _, valid := range validLanguages {
		if strings.EqualFold(language, valid) {
			return true
		}
	}
	return false
}
